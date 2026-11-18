//go:build integration

package workflow_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/audit"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/inventory"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/httpx"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/obs"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/testdb"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/workflow"
)

var server *testdb.Server

func TestMain(m *testing.M) {
	os.Exit(func() int {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		s, err := testdb.StartServer(ctx)
		if err != nil {
			_, _ = os.Stderr.WriteString("start postgres: " + err.Error() + "\n")
			return 1
		}
		server = s
		defer func() { _ = s.Terminate(context.Background()) }()
		return m.Run()
	}())
}

// ─── helpers ────────────────────────────────────────────────────────────────

var userID = uuid.MustParse("0194fa5e-0000-7000-8000-0000000000b1")

var (
	admin  = workflow.Actor{UserID: userID, Admin: true}
	nonAdm = workflow.Actor{UserID: userID, Admin: false}
)

type env struct {
	pool *pgxpool.Pool
	inv  *inventory.Service
	svc  *workflow.Service
	ctx  context.Context
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

func newEnv(t *testing.T) *env {
	t.Helper()
	dsn := server.NewMigratedDatabase(t)
	metrics := obs.NewMetrics("test", "abc1234")
	tp := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	pool, err := db.Open(context.Background(), db.Options{
		URL: testdb.AppURL(dsn), MaxConns: 12, Metrics: metrics, Tracer: tp,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	rec, err := audit.NewRecorder(obs.NewLogger(&discard{}, slog.LevelError), metrics)
	if err != nil {
		t.Fatal(err)
	}
	inv := inventory.NewService(inventory.NewStore(pool), rec, pool)
	return &env{
		pool: pool,
		inv:  inv,
		svc:  workflow.NewService(workflow.NewStore(pool), rec, pool, inv),
		ctx:  audit.WithActor(context.Background(), audit.UserActor(userID)),
	}
}

func (e *env) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := e.pool.Exec(e.ctx, sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func (e *env) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(e.ctx, sql, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", sql, err)
	}
	return n
}

func (e *env) newProduct(t *testing.T) uuid.UUID {
	t.Helper()
	var typeID, id uuid.UUID
	if err := e.pool.QueryRow(e.ctx,
		`INSERT INTO product_types (name, type_code) VALUES ('Type', $1) RETURNING id`,
		"T"+uuid.NewString()[:8]).Scan(&typeID); err != nil {
		t.Fatalf("product type: %v", err)
	}
	if err := e.pool.QueryRow(e.ctx,
		`INSERT INTO products (name, sku, product_type_id) VALUES ('P', $1, $2) RETURNING id`,
		"SKU-"+uuid.NewString()[:8], typeID).Scan(&id); err != nil {
		t.Fatalf("product: %v", err)
	}
	return id
}

// stocked returns a product with qty units received at a new location.
func (e *env) stocked(t *testing.T, qty int) uuid.UUID {
	t.Helper()
	product := e.newProduct(t)
	loc, err := e.inv.CreateLocation(e.ctx, "Warehouse", "Warehouse", "", "", "SE")
	if err != nil {
		t.Fatal(err)
	}
	if qty > 0 {
		if _, err := e.inv.CreateStockMovement(e.ctx, inventory.MovementInput{
			ProductID: product, LocationID: loc.ID, BatchNumber: "B1",
			Type: "receipt", Quantity: qty, Reason: "goods received",
		}); err != nil {
			t.Fatal(err)
		}
	}
	return product
}

// newOrder inserts an order with one line per product (quantity qty). It relies
// on the column defaults: status is the start step and the workflow version is
// the active version of the default flow.
func (e *env) newOrder(t *testing.T, qty int, products ...uuid.UUID) uuid.UUID {
	t.Helper()
	var custID, orderID uuid.UUID
	if err := e.pool.QueryRow(e.ctx,
		`INSERT INTO customers (name, type_code) VALUES ('C', 'retail') RETURNING id`).Scan(&custID); err != nil {
		t.Fatalf("customer: %v", err)
	}
	if err := e.pool.QueryRow(e.ctx,
		`INSERT INTO orders (channel, customer_id) VALUES ('direct', $1) RETURNING id`, custID).Scan(&orderID); err != nil {
		t.Fatalf("order: %v", err)
	}
	for _, p := range products {
		e.exec(t, `INSERT INTO order_lines (order_id, product_id, quantity, unit_price_sek) VALUES ($1, $2, $3, 1000)`,
			orderID, p, qty)
	}
	return orderID
}

func (e *env) status(t *testing.T, order uuid.UUID) string {
	t.Helper()
	var s string
	if err := e.pool.QueryRow(e.ctx, `SELECT status FROM orders WHERE id = $1`, order).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func (e *env) move(t *testing.T, order uuid.UUID, to string) workflow.Result {
	t.Helper()
	res, err := e.svc.ChangeStatus(e.ctx, order, to, admin, "", false)
	if err != nil {
		t.Fatalf("move %s -> %s: %v", e.status(t, order), to, err)
	}
	return res
}

func (e *env) walk(t *testing.T, order uuid.UUID, keys ...string) {
	t.Helper()
	for _, k := range keys {
		e.move(t, order, k)
	}
}

// balance returns in_stock and reserved for the product over all locations.
func (e *env) balance(t *testing.T, product uuid.UUID) (inStock, reserved int) {
	t.Helper()
	rows, err := e.inv.GetBalance(e.ctx, product)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		inStock += r.InStock
		reserved += r.Reserved
	}
	return
}

func problemStatus(t *testing.T, err error) int {
	t.Helper()
	var pe httpx.ProblemError
	if !errors.As(err, &pe) {
		t.Fatalf("error is not a problem: %v", err)
	}
	return pe.Problem().Status
}

func wantStatus(t *testing.T, err error, want int) {
	t.Helper()
	if err == nil {
		t.Fatalf("no error, want HTTP %d", want)
	}
	if got := problemStatus(t, err); got != want {
		t.Fatalf("HTTP %d (%v), want %d", got, err, want)
	}
}

// ─── tests ──────────────────────────────────────────────────────────────────

func TestWF02_TransitionNotInFlowIsRefused(t *testing.T) {
	e := newEnv(t)
	order := e.newOrder(t, 1, e.stocked(t, 5))

	_, err := e.svc.ChangeStatus(e.ctx, order, "packing", admin, "", false) // new -> packing is not in the flow
	wantStatus(t, err, http.StatusConflict)
	_, err = e.svc.ChangeStatus(e.ctx, order, "no_such_step", admin, "", false)
	wantStatus(t, err, http.StatusConflict)

	if got := e.status(t, order); got != "new" {
		t.Fatalf("status = %q, want new", got)
	}
	if n := e.count(t, `SELECT count(*) FROM order_status_history WHERE order_id = $1`, order); n != 0 {
		t.Fatalf("%d history rows, want 0", n)
	}
}

func TestWF03_EffectsRunInSameTransaction(t *testing.T) {
	e := newEnv(t)
	withStock := e.stocked(t, 5)
	noStock := e.stocked(t, 0)
	// Two lines: the first can be reserved, the second cannot.
	order := e.newOrder(t, 2, withStock, noStock)
	movementsBefore := e.count(t, `SELECT count(*) FROM stockmovements`)

	_, err := e.svc.ChangeStatus(e.ctx, order, "accepted", admin, "", false)
	wantStatus(t, err, http.StatusConflict) // insufficient stock

	if got := e.status(t, order); got != "new" {
		t.Fatalf("status = %q, want new", got)
	}
	if n := e.count(t, `SELECT count(*) FROM order_status_history WHERE order_id = $1`, order); n != 0 {
		t.Fatalf("%d history rows, want 0", n)
	}
	if n := e.count(t, `SELECT count(*) FROM stockmovements`); n != movementsBefore {
		t.Fatalf("%d movements, want %d: the first line's reservation was not rolled back", n, movementsBefore)
	}
	if n := e.count(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1`, order); n != 0 {
		t.Fatalf("%d audit rows for the failed change, want 0", n)
	}
}

func TestWF05_OverrideRequiresAdminAndReason(t *testing.T) {
	e := newEnv(t)
	order := e.newOrder(t, 1, e.stocked(t, 5))
	e.walk(t, order, "accepted", "picking")

	_, err := e.svc.ChangeStatus(e.ctx, order, "new", nonAdm, "customer asked", true)
	wantStatus(t, err, http.StatusForbidden)
	_, err = e.svc.ChangeStatus(e.ctx, order, "new", admin, "", true)
	wantStatus(t, err, http.StatusUnprocessableEntity)
	_, err = e.svc.ChangeStatus(e.ctx, order, "new", admin, "   ", true)
	wantStatus(t, err, http.StatusUnprocessableEntity)
	if got := e.status(t, order); got != "picking" {
		t.Fatalf("status = %q after refused overrides, want picking", got)
	}

	if _, err := e.svc.ChangeStatus(e.ctx, order, "new", admin, "wrong order", true); err != nil {
		t.Fatalf("valid override: %v", err)
	}
	if got := e.status(t, order); got != "new" {
		t.Fatalf("status = %q, want new", got)
	}
}

func TestWF05_OverrideBackwardsRestoresStock(t *testing.T) {
	e := newEnv(t)
	product := e.stocked(t, 10)
	order := e.newOrder(t, 3, product)

	e.walk(t, order, "accepted", "picking", "packing", "label_printed", "ready_to_ship", "posted")
	if in, res := e.balance(t, product); in != 7 || res != 0 {
		t.Fatalf("after posted in_stock/reserved = %d/%d, want 7/0", in, res)
	}

	// Back to packing: the stock comes back and is reserved again.
	if _, err := e.svc.ChangeStatus(e.ctx, order, "packing", admin, "posted by mistake", true); err != nil {
		t.Fatal(err)
	}
	if in, res := e.balance(t, product); in != 10 || res != 3 {
		t.Fatalf("after override to packing in_stock/reserved = %d/%d, want 10/3", in, res)
	}
	if n := e.count(t, `SELECT count(*) FROM stockmovements WHERE movement_type = 'return' AND product_id = $1`, product); n == 0 {
		t.Fatal("no return movement was written")
	}
	var isOverride bool
	var reason string
	if err := e.pool.QueryRow(e.ctx,
		`SELECT is_override, reason FROM order_status_history WHERE order_id = $1 ORDER BY created_at DESC, id DESC LIMIT 1`,
		order).Scan(&isOverride, &reason); err != nil {
		t.Fatal(err)
	}
	if !isOverride || reason != "posted by mistake" {
		t.Fatalf("history is_override/reason = %v/%q", isOverride, reason)
	}

	// Back before accepted: the reservation is released, stock stays.
	if _, err := e.svc.ChangeStatus(e.ctx, order, "new", admin, "start over", true); err != nil {
		t.Fatal(err)
	}
	if in, res := e.balance(t, product); in != 10 || res != 0 {
		t.Fatalf("after override to new in_stock/reserved = %d/%d, want 10/0", in, res)
	}

	// Forward again by override from new straight to posted: reserved and shipped in one go.
	if _, err := e.svc.ChangeStatus(e.ctx, order, "posted", admin, "shipped outside the app", true); err != nil {
		t.Fatal(err)
	}
	if in, res := e.balance(t, product); in != 7 || res != 0 {
		t.Fatalf("after override to posted in_stock/reserved = %d/%d, want 7/0", in, res)
	}
	// And from posted straight back to new: stock returns, nothing is reserved.
	if _, err := e.svc.ChangeStatus(e.ctx, order, "new", admin, "returned", true); err != nil {
		t.Fatal(err)
	}
	if in, res := e.balance(t, product); in != 10 || res != 0 {
		t.Fatalf("after override posted->new in_stock/reserved = %d/%d, want 10/0", in, res)
	}
}

func TestWF05_PreviewShowsEffectsWithoutRunningThem(t *testing.T) {
	e := newEnv(t)
	product := e.stocked(t, 10)
	order := e.newOrder(t, 4, product)

	effects, err := e.svc.Preview(e.ctx, order, "accepted", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(effects) != 1 || effects[0].Kind != "reserve" || effects[0].ProductID != product || effects[0].Quantity != 4 {
		t.Fatalf("preview = %+v, want one reserve of 4", effects)
	}
	if got := e.status(t, order); got != "new" {
		t.Fatalf("preview changed the status to %q", got)
	}
	if in, res := e.balance(t, product); in != 10 || res != 0 {
		t.Fatalf("preview changed stock: %d/%d", in, res)
	}
	if n := e.count(t, `SELECT count(*) FROM order_status_history WHERE order_id = $1`, order); n != 0 {
		t.Fatalf("preview wrote %d history rows", n)
	}

	_, err = e.svc.Preview(e.ctx, order, "packing", false) // not in the flow
	wantStatus(t, err, http.StatusConflict)
}

func TestWF05_DeliveredIsFinal(t *testing.T) {
	e := newEnv(t)
	product := e.stocked(t, 10)
	order := e.newOrder(t, 1, product)
	e.walk(t, order, "accepted", "picking", "packing", "label_printed", "ready_to_ship", "posted", "in_transit", "delivered")

	for _, to := range []string{"packing", "new", "cancelled", "in_transit"} {
		_, err := e.svc.ChangeStatus(e.ctx, order, to, admin, "try to leave delivered", true)
		wantStatus(t, err, http.StatusConflict)
	}
	_, err := e.svc.ChangeStatus(e.ctx, order, "cancelled", admin, "", false)
	wantStatus(t, err, http.StatusConflict)

	if got := e.status(t, order); got != "delivered" {
		t.Fatalf("status = %q, want delivered", got)
	}
	if in, res := e.balance(t, product); in != 9 || res != 0 {
		t.Fatalf("stock changed after refused moves: %d/%d, want 9/0", in, res)
	}
}

func TestWF05_ReopenCancelledReservesAgain(t *testing.T) {
	t.Run("stock available", func(t *testing.T) {
		e := newEnv(t)
		product := e.stocked(t, 10)
		order := e.newOrder(t, 3, product)
		e.walk(t, order, "accepted", "cancelled")
		if in, res := e.balance(t, product); in != 10 || res != 0 {
			t.Fatalf("after cancel in_stock/reserved = %d/%d, want 10/0", in, res)
		}

		_, err := e.svc.ChangeStatus(e.ctx, order, "new", nonAdm, "customer changed mind", true)
		wantStatus(t, err, http.StatusForbidden)
		_, err = e.svc.ChangeStatus(e.ctx, order, "new", admin, "", true)
		wantStatus(t, err, http.StatusUnprocessableEntity)

		res, err := e.svc.ChangeStatus(e.ctx, order, "new", admin, "customer changed mind", true)
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Warnings) != 0 {
			t.Fatalf("warnings = %v, want none", res.Warnings)
		}
		if got := e.status(t, order); got != "new" {
			t.Fatalf("status = %q, want new", got)
		}
		if in, reserved := e.balance(t, product); in != 10 || reserved != 3 {
			t.Fatalf("after reopen in_stock/reserved = %d/%d, want 10/3", in, reserved)
		}
		// Accepting again must not reserve a second time.
		e.move(t, order, "accepted")
		if _, reserved := e.balance(t, product); reserved != 3 {
			t.Fatalf("reserved = %d after accepting a reopened order, want 3", reserved)
		}
	})

	t.Run("stock not available", func(t *testing.T) {
		e := newEnv(t)
		product := e.stocked(t, 5)
		first := e.newOrder(t, 5, product)
		e.walk(t, first, "accepted", "cancelled")
		other := e.newOrder(t, 5, product)
		e.move(t, other, "accepted") // takes all the stock

		res, err := e.svc.ChangeStatus(e.ctx, first, "new", admin, "reopen", true)
		if err != nil {
			t.Fatalf("reopen must succeed without stock: %v", err)
		}
		if !slices.Contains(res.Warnings, workflow.WarningStockNotAvailable) {
			t.Fatalf("warnings = %v, want %q", res.Warnings, workflow.WarningStockNotAvailable)
		}
		if got := e.status(t, first); got != "new" {
			t.Fatalf("status = %q, want new", got)
		}
		if _, reserved := e.balance(t, product); reserved != 5 {
			t.Fatalf("reserved = %d, want 5 (only the other order)", reserved)
		}
	})

	t.Run("rejected behaves like cancelled", func(t *testing.T) {
		e := newEnv(t)
		product := e.stocked(t, 5)
		order := e.newOrder(t, 2, product)
		e.move(t, order, "rejected")
		if _, err := e.svc.ChangeStatus(e.ctx, order, "new", admin, "reconsidered", true); err != nil {
			t.Fatal(err)
		}
		if got := e.status(t, order); got != "new" {
			t.Fatalf("status = %q, want new", got)
		}
	})
}

func TestWF06_InvalidFlowIsRejectedOnSave(t *testing.T) {
	e := newEnv(t)
	base := func() workflow.VersionDefinition {
		v, err := e.svc.ActiveVersion(e.ctx)
		if err != nil {
			t.Fatal(err)
		}
		return v.Definition()
	}
	versions := func() int { return e.count(t, `SELECT count(*) FROM workflow_versions`) }

	cases := map[string]func(d *workflow.VersionDefinition){
		"no start step": func(d *workflow.VersionDefinition) {
			for i := range d.Steps {
				d.Steps[i].IsStart = false
			}
		},
		"two start steps": func(d *workflow.VersionDefinition) {
			for i := range d.Steps {
				if d.Steps[i].Key == "in_review" {
					d.Steps[i].IsStart = true
				}
			}
		},
		"non-final step cannot reach delivered": func(d *workflow.VersionDefinition) {
			d.Steps = append(d.Steps, workflow.StepDef{Key: "dead_end", SortOrder: 99})
			d.Transitions = append(d.Transitions, workflow.TransitionDef{From: "new", To: "dead_end"})
		},
		"transition into the start step": func(d *workflow.VersionDefinition) {
			d.Transitions = append(d.Transitions, workflow.TransitionDef{From: "packing", To: "new"})
		},
		"transition to a step that does not exist": func(d *workflow.VersionDefinition) {
			d.Transitions = append(d.Transitions, workflow.TransitionDef{From: "new", To: "ghost"})
		},
		"duplicate step key": func(d *workflow.VersionDefinition) {
			d.Steps = append(d.Steps, workflow.StepDef{Key: "packing", SortOrder: 98})
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			before := versions()
			d := base()
			mutate(&d)
			_, err := e.svc.PublishVersion(e.ctx, d)
			wantStatus(t, err, http.StatusUnprocessableEntity)
			if after := versions(); after != before {
				t.Fatalf("%d versions after a refused publish, want %d", after, before)
			}
		})
	}

	t.Run("the default flow is itself valid", func(t *testing.T) {
		if _, err := e.svc.PublishVersion(e.ctx, base()); err != nil {
			t.Fatalf("republishing the default flow: %v", err)
		}
	})
}

func TestWF07_OrderKeepsItsWorkflowVersion(t *testing.T) {
	e := newEnv(t)
	product := e.stocked(t, 20)
	oldOrder := e.newOrder(t, 1, product)
	e.walk(t, oldOrder, "accepted", "picking", "packing")
	v1, err := e.svc.ActiveVersion(e.ctx)
	if err != nil {
		t.Fatal(err)
	}

	// v2 puts a quality_check step between packing and label_printed.
	d := v1.Definition()
	d.Steps = append(d.Steps, workflow.StepDef{Key: "quality_check", SortOrder: 55, StockPhase: "reserved"})
	d.Transitions = slices.DeleteFunc(d.Transitions, func(tr workflow.TransitionDef) bool {
		return tr.From == "packing" && tr.To == "label_printed"
	})
	d.Transitions = append(d.Transitions,
		workflow.TransitionDef{From: "packing", To: "quality_check"},
		workflow.TransitionDef{From: "quality_check", To: "label_printed"},
		workflow.TransitionDef{From: "quality_check", To: "cancelled", Effect: "release"},
	)
	v2, err := e.svc.PublishVersion(e.ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	if v2.Version != v1.Version+1 || v2.ID == v1.ID {
		t.Fatalf("v2 = %+v, v1 = %+v", v2, v1)
	}
	if active, _ := e.svc.ActiveVersion(e.ctx); active.ID != v2.ID {
		t.Fatal("the new version is not the active one")
	}

	// The old order follows v1: the direct move still works, quality_check does not exist.
	_, err = e.svc.ChangeStatus(e.ctx, oldOrder, "quality_check", admin, "", false)
	wantStatus(t, err, http.StatusConflict)
	e.move(t, oldOrder, "label_printed")
	var oldVersion uuid.UUID
	if err := e.pool.QueryRow(e.ctx, `SELECT workflow_version_id FROM orders WHERE id = $1`, oldOrder).Scan(&oldVersion); err != nil {
		t.Fatal(err)
	}
	if oldVersion != v1.ID {
		t.Fatalf("old order moved to version %s, want %s", oldVersion, v1.ID)
	}

	// A new order gets v2: the direct move is refused, the detour works.
	newOrder := e.newOrder(t, 1, product)
	var newVersion uuid.UUID
	if err := e.pool.QueryRow(e.ctx, `SELECT workflow_version_id FROM orders WHERE id = $1`, newOrder).Scan(&newVersion); err != nil {
		t.Fatal(err)
	}
	if newVersion != v2.ID {
		t.Fatalf("new order has version %s, want %s", newVersion, v2.ID)
	}
	e.walk(t, newOrder, "accepted", "picking", "packing")
	_, err = e.svc.ChangeStatus(e.ctx, newOrder, "label_printed", admin, "", false)
	wantStatus(t, err, http.StatusConflict)
	e.walk(t, newOrder, "quality_check", "label_printed")
}

func TestWF10_HistoryRecordsEveryChange(t *testing.T) {
	e := newEnv(t)
	product := e.stocked(t, 10)
	order := e.newOrder(t, 1, product)

	e.walk(t, order, "accepted", "picking")
	if _, err := e.svc.ChangeStatus(e.ctx, order, "new", admin, "mistake", true); err != nil {
		t.Fatal(err)
	}
	e.move(t, order, "in_review")

	hist, err := e.svc.History(e.ctx, order)
	if err != nil {
		t.Fatal(err)
	}
	type step struct {
		from, to string
		override bool
	}
	want := []step{{"new", "accepted", false}, {"accepted", "picking", false}, {"picking", "new", true}, {"new", "in_review", false}}
	if len(hist) != len(want) {
		t.Fatalf("%d history rows, want %d: %+v", len(hist), len(want), hist)
	}
	for i, w := range want {
		h := hist[i]
		if h.FromKey != w.from || h.ToKey != w.to || h.IsOverride != w.override || h.ActorID != userID {
			t.Fatalf("history[%d] = %+v, want %+v by %s", i, h, w, userID)
		}
	}
	if hist[2].Reason != "mistake" {
		t.Fatalf("override reason = %q", hist[2].Reason)
	}

	// Time in step is computed from the history: the order has now been in in_review for 3 hours.
	times, err := e.svc.TimeInSteps(e.ctx, order, time.Now().Add(3*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	var total time.Duration
	for _, st := range times {
		if st.Duration < 0 {
			t.Fatalf("negative time in step %q", st.Key)
		}
		total += st.Duration
	}
	if len(times) == 0 || times[len(times)-1].Key != "in_review" || times[len(times)-1].Duration < 3*time.Hour {
		t.Fatalf("times = %+v, want the last interval in in_review for at least 3h", times)
	}
	if total < 3*time.Hour {
		t.Fatalf("total = %s", total)
	}
}

// TestNFK27_StockNeverNegativeUnderRandomTransitions runs random sequences of
// normal moves and admin overrides over several orders that share one product.
// Whatever is accepted or refused, in stock and reserved must never go negative
// and reserved must never exceed in stock. At the end, cancelling every order
// that is not delivered must leave no reservation at all.
func TestNFK27_StockNeverNegativeUnderRandomTransitions(t *testing.T) {
	keys := []string{"new", "in_review", "accepted", "picking", "packing", "label_printed",
		"ready_to_ship", "posted", "in_transit", "delivered", "rejected", "cancelled"}

	for seed := int64(1); seed <= 6; seed++ {
		t.Run(fmt.Sprintf("seed %d", seed), func(t *testing.T) {
			e := newEnv(t)
			rng := rand.New(rand.NewSource(seed)) //nolint:gosec // deterministic test data
			product := e.stocked(t, 12)
			var orders []uuid.UUID
			for range 4 {
				orders = append(orders, e.newOrder(t, 1+rng.Intn(4), product))
			}

			check := func(step int, op string) {
				in, res := e.balance(t, product)
				if in < 0 || res < 0 || res > in {
					t.Fatalf("after op %d (%s): in_stock/reserved = %d/%d", step, op, in, res)
				}
			}
			for step := range 60 {
				order := orders[rng.Intn(len(orders))]
				to := keys[rng.Intn(len(keys))]
				override := rng.Intn(3) == 0
				_, err := e.svc.ChangeStatus(e.ctx, order, to, admin, "random", override)
				if err != nil {
					var pe httpx.ProblemError
					if !errors.As(err, &pe) {
						t.Fatalf("op %d: unexpected non-problem error: %v", step, err)
					}
				}
				check(step, fmt.Sprintf("%s -> %s override=%v err=%v", order, to, override, err))
			}

			// Reservations must balance out: cancel everything that can be cancelled.
			for _, o := range orders {
				_, _ = e.svc.ChangeStatus(e.ctx, o, "cancelled", admin, "", false)
			}
			var shipped int
			for _, o := range orders {
				if e.status(t, o) == "delivered" {
					shipped += e.count(t, `SELECT COALESCE(sum(quantity), 0)::int FROM order_lines WHERE order_id = $1`, o)
				}
			}
			var posted int
			for _, o := range orders {
				switch e.status(t, o) {
				case "posted", "in_transit":
					posted += e.count(t, `SELECT COALESCE(sum(quantity), 0)::int FROM order_lines WHERE order_id = $1`, o)
				}
			}
			in, res := e.balance(t, product)
			// Orders left in cancelled hold nothing. Orders that could not be cancelled (shipped) hold no reservation either.
			if res != 0 {
				t.Fatalf("reserved = %d after cleanup, want 0 (orders: %v)", res, orders)
			}
			if in != 12-shipped-posted {
				t.Fatalf("in_stock = %d, want %d (12 minus %d delivered and %d shipped)", in, 12-shipped-posted, shipped, posted)
			}
		})
	}
}
