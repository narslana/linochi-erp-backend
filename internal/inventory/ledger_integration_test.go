//go:build integration

package inventory_test

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/audit"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/inventory"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/httpx"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/obs"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/testdb"
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

type env struct {
	pool *pgxpool.Pool
	svc  *inventory.Service
	ctx  context.Context
}

var actorID = uuid.MustParse("0194fa5e-0000-7000-8000-0000000000a1")

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
	return &env{
		pool: pool,
		svc:  inventory.NewService(inventory.NewStore(pool), rec, pool),
		ctx:  audit.WithActor(context.Background(), audit.UserActor(actorID)),
	}
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

func (e *env) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := e.pool.Exec(e.ctx, sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func (e *env) newProduct(t *testing.T) uuid.UUID {
	t.Helper()
	var typeID, id uuid.UUID
	err := e.pool.QueryRow(e.ctx,
		`INSERT INTO product_types (name, type_code) VALUES ('Type', $1) RETURNING id`,
		"T"+uuid.NewString()[:8]).Scan(&typeID)
	if err != nil {
		t.Fatalf("product type: %v", err)
	}
	err = e.pool.QueryRow(e.ctx,
		`INSERT INTO products (name, sku, product_type_id) VALUES ('P', $1, $2) RETURNING id`,
		"SKU-"+uuid.NewString()[:8], typeID).Scan(&id)
	if err != nil {
		t.Fatalf("product: %v", err)
	}
	return id
}

func (e *env) newLocation(t *testing.T) uuid.UUID {
	t.Helper()
	loc, err := e.svc.CreateLocation(e.ctx, "Warehouse", "Warehouse", "", "", "SE")
	if err != nil {
		t.Fatal(err)
	}
	return loc.ID
}

// newOrderLine inserts a customer, an order and one line directly: the order
// module is out of scope for T1.
func (e *env) newOrderLine(t *testing.T, productID uuid.UUID, qty int) uuid.UUID {
	t.Helper()
	var custID, orderID, lineID uuid.UUID
	if err := e.pool.QueryRow(e.ctx,
		`INSERT INTO customers (name, type_code) VALUES ('C', 'retail') RETURNING id`).Scan(&custID); err != nil {
		t.Fatalf("customer: %v", err)
	}
	if err := e.pool.QueryRow(e.ctx,
		`INSERT INTO orders (channel, customer_id) VALUES ('direct', $1) RETURNING id`, custID).Scan(&orderID); err != nil {
		t.Fatalf("order: %v", err)
	}
	if err := e.pool.QueryRow(e.ctx,
		`INSERT INTO order_lines (order_id, product_id, quantity, unit_price_sek) VALUES ($1, $2, $3, 1000) RETURNING id`,
		orderID, productID, qty).Scan(&lineID); err != nil {
		t.Fatalf("order line: %v", err)
	}
	return lineID
}

func (e *env) receive(t *testing.T, product, loc uuid.UUID, batch string, qty int, bestBefore string) {
	t.Helper()
	in := inventory.MovementInput{
		ProductID: product, LocationID: loc, BatchNumber: batch,
		Type: "receipt", Quantity: qty, Reason: "goods received",
	}
	if bestBefore != "" {
		in.BestBeforeDate = &bestBefore
	}
	if _, err := e.svc.CreateStockMovement(e.ctx, in); err != nil {
		t.Fatalf("receive: %v", err)
	}
}

func (e *env) inTx(t *testing.T, fn func(tx *db.Tx) error) error {
	t.Helper()
	return db.InTx(e.ctx, e.pool, fn)
}

func (e *env) reserve(t *testing.T, product, loc uuid.UUID, qty int, line uuid.UUID) error {
	t.Helper()
	return e.inTx(t, func(tx *db.Tx) error {
		return e.svc.Reserve(e.ctx, tx, inventory.StockRef{
			ProductID: product, LocationID: loc, Quantity: qty,
			ReferenceType: "order_line", ReferenceID: line,
		})
	})
}

// total returns the product's balance summed over all locations.
func (e *env) total(t *testing.T, product uuid.UUID) (inStock, reserved, available int) {
	t.Helper()
	rows, err := e.svc.GetBalance(e.ctx, product)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		inStock += r.InStock
		reserved += r.Reserved
		available += r.Available
	}
	return
}

func assertBalance(t *testing.T, e *env, product uuid.UUID, inStock, reserved, available int) {
	t.Helper()
	i, r, a := e.total(t, product)
	if i != inStock || r != reserved || a != available {
		t.Fatalf("balance in_stock/reserved/available = %d/%d/%d, want %d/%d/%d", i, r, a, inStock, reserved, available)
	}
}

func (e *env) movementCount(t *testing.T) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(e.ctx, `SELECT count(*) FROM stockmovements`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func problemStatus(err error) int {
	var pe httpx.ProblemError
	if errors.As(err, &pe) {
		return pe.Problem().Status
	}
	return 0
}

// ─── LAG-02 ─────────────────────────────────────────────────────────────────

func TestLAG02_ReceiptIncreasesInStockAndAvailable(t *testing.T) {
	e := newEnv(t)
	p, loc := e.newProduct(t), e.newLocation(t)
	e.receive(t, p, loc, "B1", 10, "")
	assertBalance(t, e, p, 10, 0, 10)
}

func TestLAG02_ReserveLowersAvailableNotInStock(t *testing.T) {
	e := newEnv(t)
	p, loc := e.newProduct(t), e.newLocation(t)
	line := e.newOrderLine(t, p, 3)
	e.receive(t, p, loc, "B1", 10, "")
	if err := e.reserve(t, p, loc, 3, line); err != nil {
		t.Fatal(err)
	}
	assertBalance(t, e, p, 10, 3, 7)
}

func TestLAG02_ShipReservedLineDeductsOnce(t *testing.T) {
	e := newEnv(t)
	p, loc := e.newProduct(t), e.newLocation(t)
	line := e.newOrderLine(t, p, 3)
	e.receive(t, p, loc, "B1", 10, "")
	if err := e.reserve(t, p, loc, 3, line); err != nil {
		t.Fatal(err)
	}
	err := e.inTx(t, func(tx *db.Tx) error {
		_, err := e.svc.Ship(e.ctx, tx, inventory.ShipInput{
			StockRef: inventory.StockRef{
				ProductID: p, LocationID: loc, Quantity: 3,
				ReferenceType: "order_line", ReferenceID: line,
			},
			OrderLineID: line,
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	assertBalance(t, e, p, 7, 0, 7)
}

func TestLAG02_CancelReleasesReservation(t *testing.T) {
	e := newEnv(t)
	p, loc := e.newProduct(t), e.newLocation(t)
	line := e.newOrderLine(t, p, 3)
	e.receive(t, p, loc, "B1", 10, "")
	if err := e.reserve(t, p, loc, 3, line); err != nil {
		t.Fatal(err)
	}
	err := e.inTx(t, func(tx *db.Tx) error {
		return e.svc.Release(e.ctx, tx, inventory.StockRef{
			ProductID: p, LocationID: loc, Quantity: 3,
			ReferenceType: "order_line", ReferenceID: line,
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	assertBalance(t, e, p, 10, 0, 10)
}

func TestLAG02_ReserveMoreThanAvailableIsRefused(t *testing.T) {
	e := newEnv(t)
	p, loc := e.newProduct(t), e.newLocation(t)
	line := e.newOrderLine(t, p, 4)
	e.receive(t, p, loc, "B1", 3, "")
	before := e.movementCount(t)

	err := e.reserve(t, p, loc, 4, line)
	if got := problemStatus(err); got != http.StatusConflict {
		t.Fatalf("status = %d (err %v), want 409", got, err)
	}
	if after := e.movementCount(t); after != before {
		t.Fatalf("movements %d -> %d, a refused reservation must write nothing", before, after)
	}
	assertBalance(t, e, p, 3, 0, 3)
}

func TestLAG02_ConcurrentReservationsCannotOversell(t *testing.T) {
	e := newEnv(t)
	p, loc := e.newProduct(t), e.newLocation(t)
	e.receive(t, p, loc, "B1", 5, "")
	lines := make([]uuid.UUID, 10)
	for i := range lines {
		lines[i] = e.newOrderLine(t, p, 1)
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	ok, conflict, other := 0, 0, 0
	for _, line := range lines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := e.reserve(t, p, loc, 1, line)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ok++
			case problemStatus(err) == http.StatusConflict:
				conflict++
			default:
				other++
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()
	if ok != 5 || conflict != 5 || other != 0 {
		t.Fatalf("ok/conflict/other = %d/%d/%d, want 5/5/0", ok, conflict, other)
	}
	assertBalance(t, e, p, 5, 5, 0)
}

// ─── LAG-03 ─────────────────────────────────────────────────────────────────

func (e *env) shipFrom(t *testing.T, p, loc, line uuid.UUID, qty int, batch string) {
	t.Helper()
	err := e.inTx(t, func(tx *db.Tx) error {
		_, err := e.svc.Ship(e.ctx, tx, inventory.ShipInput{
			StockRef: inventory.StockRef{
				ProductID: p, LocationID: loc, Quantity: qty,
				ReferenceType: "order_line", ReferenceID: line,
			},
			OrderLineID: line,
			BatchNumber: batch,
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (e *env) lineBatches(t *testing.T, line uuid.UUID) map[string]int {
	t.Helper()
	rows, err := e.pool.Query(e.ctx,
		`SELECT batch_number, quantity FROM order_line_batches WHERE order_line_id = $1`, line)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var b string
		var q int
		if err := rows.Scan(&b, &q); err != nil {
			t.Fatal(err)
		}
		out[b] += q
	}
	return out
}

func TestLAG03_ShipPicksEarliestBestBeforeBatch(t *testing.T) {
	e := newEnv(t)
	p, loc := e.newProduct(t), e.newLocation(t)
	line := e.newOrderLine(t, p, 3)
	e.receive(t, p, loc, "LATE", 5, "2027-06-01")
	e.receive(t, p, loc, "EARLY", 5, "2027-03-01")
	if err := e.reserve(t, p, loc, 3, line); err != nil {
		t.Fatal(err)
	}
	e.shipFrom(t, p, loc, line, 3, "")

	got := e.lineBatches(t, line)
	if len(got) != 1 || got["EARLY"] != 3 {
		t.Fatalf("order line batches = %v, want EARLY:3", got)
	}
}

func TestLAG03_ShipWithManualBatch(t *testing.T) {
	e := newEnv(t)
	p, loc := e.newProduct(t), e.newLocation(t)
	line := e.newOrderLine(t, p, 3)
	e.receive(t, p, loc, "LATE", 5, "2027-06-01")
	e.receive(t, p, loc, "EARLY", 5, "2027-03-01")
	if err := e.reserve(t, p, loc, 3, line); err != nil {
		t.Fatal(err)
	}
	e.shipFrom(t, p, loc, line, 3, "LATE")

	got := e.lineBatches(t, line)
	if len(got) != 1 || got["LATE"] != 3 {
		t.Fatalf("order line batches = %v, want LATE:3", got)
	}
	assertBalance(t, e, p, 7, 0, 7)
}

// ─── LAG-04 ─────────────────────────────────────────────────────────────────

func TestLAG04_AdjustmentWithoutReasonIsRefused(t *testing.T) {
	e := newEnv(t)
	p, loc := e.newProduct(t), e.newLocation(t)
	e.receive(t, p, loc, "B1", 10, "")
	before := e.movementCount(t)

	for _, typ := range []string{"adjustment_in", "adjustment_out"} {
		for _, reason := range []string{"", "   "} {
			_, err := e.svc.CreateStockMovement(e.ctx, inventory.MovementInput{
				ProductID: p, LocationID: loc, BatchNumber: "B1",
				Type: typ, Quantity: 1, Reason: reason,
			})
			if err == nil {
				t.Fatalf("%s with reason %q was accepted", typ, reason)
			}
			if got := problemStatus(err); got != http.StatusUnprocessableEntity {
				t.Fatalf("%s status = %d (err %v), want 422", typ, got, err)
			}
		}
	}
	if after := e.movementCount(t); after != before {
		t.Fatalf("movements %d -> %d, refused adjustments must write nothing", before, after)
	}

	// With a reason it works and the reason is stored on the movement.
	m, err := e.svc.CreateStockMovement(e.ctx, inventory.MovementInput{
		ProductID: p, LocationID: loc, BatchNumber: "B1",
		Type: "adjustment_out", Quantity: 2, Reason: "damaged",
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.Reason != "damaged" || m.Quantity != -2 {
		t.Fatalf("movement reason/quantity = %q/%d, want damaged/-2", m.Reason, m.Quantity)
	}
}

func TestLAG04_SignFollowsMovementType(t *testing.T) {
	e := newEnv(t)
	p, loc := e.newProduct(t), e.newLocation(t)
	e.receive(t, p, loc, "B1", 10, "")

	post := func(typ string, qty int) error {
		_, err := e.svc.CreateStockMovement(e.ctx, inventory.MovementInput{
			ProductID: p, LocationID: loc, BatchNumber: "B1", Type: typ, Quantity: qty,
			Reason: "test",
		})
		return err
	}

	// ASSUMPTION: the caller sends the magnitude. Any explicit sign (a negative
	// number) and zero are refused, whatever the type.
	for _, tc := range []struct {
		typ string
		qty int
	}{{"sale", -5}, {"receipt", -5}, {"sale", 0}, {"adjustment_in", -1}} {
		if err := post(tc.typ, tc.qty); err == nil {
			t.Fatalf("%s with quantity %d was accepted", tc.typ, tc.qty)
		}
	}
	assertBalance(t, e, p, 10, 0, 10)

	if err := post("sale", 5); err != nil {
		t.Fatal(err)
	}
	assertBalance(t, e, p, 5, 0, 5)

	// The stored sign is decided by the type, in one place.
	rows, err := e.pool.Query(e.ctx, `SELECT movement_type, quantity FROM stockmovements WHERE product_id = $1`, p)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var typ string
		var q int
		if err := rows.Scan(&typ, &q); err != nil {
			t.Fatal(err)
		}
		switch typ {
		case "receipt", "adjustment_in", "transfer_in", "release", "return":
			if q <= 0 {
				t.Errorf("%s stored with quantity %d, want positive", typ, q)
			}
		default:
			if q >= 0 {
				t.Errorf("%s stored with quantity %d, want negative", typ, q)
			}
		}
	}

	// The database refuses a row whose sign contradicts the type, even when it
	// bypasses the service.
	_, err = e.pool.Exec(e.ctx,
		`INSERT INTO stockmovements (product_id, location_id, movement_type, quantity) VALUES ($1, $2, 'sale', 5)`, p, loc)
	if err == nil {
		t.Fatal("database accepted a positive sale")
	}
}

func TestLAG04_TransferWritesPairedMovements(t *testing.T) {
	e := newEnv(t)
	p, from, to := e.newProduct(t), e.newLocation(t), e.newLocation(t)
	e.receive(t, p, from, "B1", 10, "")

	if err := e.svc.Transfer(e.ctx, inventory.TransferInput{
		ProductID: p, FromLocationID: from, ToLocationID: to, BatchNumber: "B1", Quantity: 4, Reason: "restock",
	}); err != nil {
		t.Fatal(err)
	}
	assertBalance(t, e, p, 10, 0, 10)
	rows, err := e.svc.GetBalance(e.ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	got := map[uuid.UUID]int{}
	for _, r := range rows {
		got[r.LocationID] = r.InStock
	}
	if got[from] != 6 || got[to] != 4 {
		t.Fatalf("per location = %v, want from:6 to:4", got)
	}
}

// ─── LAG-01 ─────────────────────────────────────────────────────────────────

func TestLAG01_LedgerIsAppendOnly(t *testing.T) {
	e := newEnv(t)
	p, loc := e.newProduct(t), e.newLocation(t)
	e.receive(t, p, loc, "B1", 10, "")

	for _, stmt := range []string{
		`UPDATE stockmovements SET quantity = 999`,
		`UPDATE stockmovements SET reason = 'tampered'`,
		`DELETE FROM stockmovements`,
	} {
		_, err := e.pool.Exec(e.ctx, stmt)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
			t.Fatalf("%q: err = %v, want permission denied (42501)", stmt, err)
		}
	}
	assertBalance(t, e, p, 10, 0, 10)
}

// ─── LAG-05 ─────────────────────────────────────────────────────────────────

func TestLAG05_ValueUsesManufacturingCost(t *testing.T) {
	e := newEnv(t)
	p, loc := e.newProduct(t), e.newLocation(t)
	e.exec(t, `UPDATE products SET manufacturing_cost = 12.50 WHERE id = $1`, p)

	// Two suppliers with different purchase prices must not change or double the value.
	var s1, s2 uuid.UUID
	for i, id := range []*uuid.UUID{&s1, &s2} {
		if err := e.pool.QueryRow(e.ctx,
			`INSERT INTO suppliers (name) VALUES ($1) RETURNING id`, "Supplier "+string(rune('A'+i))).Scan(id); err != nil {
			t.Fatalf("supplier: %v", err)
		}
		e.exec(t, `INSERT INTO supplier_products (supplier_id, product_id, purchase_price) VALUES ($1, $2, $3)`,
			*id, p, 1000*(i+1))
	}

	e.receive(t, p, loc, "B1", 10, "")
	line := e.newOrderLine(t, p, 2)
	if err := e.reserve(t, p, loc, 2, line); err != nil { // reserved stock is still in stock
		t.Fatal(err)
	}

	values, err := e.svc.GetInventoryValue(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 {
		t.Fatalf("got %d value rows, want 1: %+v", len(values), values)
	}
	// 10 x 12.50 SEK = 125.00 SEK = 12500 öre.
	if values[0].InStock != 10 || values[0].Value != 12500 {
		t.Fatalf("in_stock/value = %d/%d, want 10/12500", values[0].InStock, values[0].Value)
	}
}

// ─── LAG-07 ─────────────────────────────────────────────────────────────────

func TestLAG07_MarketplaceQuantityRespectsBuffer(t *testing.T) {
	e := newEnv(t)
	p, loc := e.newProduct(t), e.newLocation(t)
	e.exec(t, `UPDATE products SET channel_buffer = 5 WHERE id = $1`, p)

	e.receive(t, p, loc, "B1", 100, "")
	got, err := e.svc.MarketplaceQuantity(e.ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if got != 95 {
		t.Fatalf("available 100, buffer 5: marketplace quantity = %d, want 95", got)
	}

	q := e.newProduct(t)
	e.exec(t, `UPDATE products SET channel_buffer = 5 WHERE id = $1`, q)
	e.receive(t, q, loc, "B1", 3, "")
	got, err = e.svc.MarketplaceQuantity(e.ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Fatalf("available 3, buffer 5: marketplace quantity = %d, want 0", got)
	}
}
