//go:build integration

package orders_test

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/audit"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/inventory"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/orders"
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

var userID = uuid.MustParse("0194fa5e-0000-7000-8000-0000000000c1")

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

type env struct {
	pool *pgxpool.Pool
	inv  *inventory.Service
	svc  *orders.Service
	ctx  context.Context
}

func newEnv(t *testing.T) *env {
	t.Helper()
	dsn := server.NewMigratedDatabase(t)
	metrics := obs.NewMetrics("test", "abc1234")
	tp := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	pool, err := db.Open(context.Background(), db.Options{URL: testdb.AppURL(dsn), MaxConns: 8, Metrics: metrics, Tracer: tp})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	rec, err := audit.NewRecorder(obs.NewLogger(&discard{}, slog.LevelError), metrics)
	if err != nil {
		t.Fatal(err)
	}
	inv := inventory.NewService(inventory.NewStore(pool), rec, pool)
	wf := workflow.NewService(workflow.NewStore(pool), rec, pool, inv)
	return &env{
		pool: pool, inv: inv,
		svc: orders.NewService(orders.NewStore(pool), rec, pool, wf),
		ctx: audit.WithActor(context.Background(), audit.UserActor(userID)),
	}
}

var admin = workflow.Actor{UserID: userID, Admin: true}

// order inserts a product with stock units in stock and an order for qty of it.
func (e *env) order(t *testing.T, stock, qty int) (orderID, productID uuid.UUID) {
	t.Helper()
	var typeID, custID uuid.UUID
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(e.pool.QueryRow(e.ctx, `INSERT INTO product_types (name, type_code) VALUES ('Type', $1) RETURNING id`, "T"+uuid.NewString()[:8]).Scan(&typeID))
	must(e.pool.QueryRow(e.ctx, `INSERT INTO products (name, sku, product_type_id) VALUES ('P', $1, $2) RETURNING id`, "SKU-"+uuid.NewString()[:8], typeID).Scan(&productID))
	loc, err := e.inv.CreateLocation(e.ctx, "Warehouse", "Warehouse", "", "", "SE")
	must(err)
	_, err = e.inv.CreateStockMovement(e.ctx, inventory.MovementInput{
		ProductID: productID, LocationID: loc.ID, BatchNumber: "B1", Type: "receipt", Quantity: stock, Reason: "goods received",
	})
	must(err)
	must(e.pool.QueryRow(e.ctx, `INSERT INTO customers (name, type_code) VALUES ('C', 'retail') RETURNING id`).Scan(&custID))
	must(e.pool.QueryRow(e.ctx, `INSERT INTO orders (channel, customer_id) VALUES ('direct', $1) RETURNING id`, custID).Scan(&orderID))
	_, err = e.pool.Exec(e.ctx, `INSERT INTO order_lines (order_id, product_id, quantity, unit_price_sek) VALUES ($1, $2, $3, 1000)`, orderID, productID, qty)
	must(err)
	return orderID, productID
}

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

// TestORD04_AcceptOrderSucceeds is the regression for the version=0 bug: accepting
// an order passed version 0 to the optimistic lock, so every accept failed.
func TestORD04_AcceptOrderSucceeds(t *testing.T) {
	e := newEnv(t)
	id, product := e.order(t, 10, 3)
	if _, _, err := e.svc.ChangeOrderStatus(e.ctx, id, "in_review", admin, "", false); err != nil {
		t.Fatal(err)
	}
	before, err := e.svc.GetOrder(e.ctx, id)
	if err != nil {
		t.Fatal(err)
	}

	got, _, err := e.svc.AcceptOrder(e.ctx, id, admin)
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if got.Status != "accepted" {
		t.Fatalf("status = %q, want accepted", got.Status)
	}
	if got.Version <= before.Version {
		t.Fatalf("version %d did not increase from %d", got.Version, before.Version)
	}
	if in, res := e.balance(t, product); in != 10 || res != 3 {
		t.Fatalf("in_stock/reserved = %d/%d, want 10/3", in, res)
	}
}

func TestORD04_AcceptWithoutStockIsRefused(t *testing.T) {
	e := newEnv(t)
	id, product := e.order(t, 2, 3)
	if _, _, err := e.svc.ChangeOrderStatus(e.ctx, id, "in_review", admin, "", false); err != nil {
		t.Fatal(err)
	}

	_, _, err := e.svc.AcceptOrder(e.ctx, id, admin)
	var pe httpx.ProblemError
	if !errors.As(err, &pe) || pe.Problem().Status != http.StatusConflict {
		t.Fatalf("err = %v, want a 409 problem", err)
	}
	if o, _ := e.svc.GetOrder(e.ctx, id); o.Status != "in_review" {
		t.Fatalf("status = %q, want in_review", o.Status)
	}
	if _, res := e.balance(t, product); res != 0 {
		t.Fatalf("reserved = %d, want 0", res)
	}
}

func TestORD06_ShipOrderWithdrawsStock(t *testing.T) {
	e := newEnv(t)
	id, product := e.order(t, 10, 3)
	for _, to := range []string{"in_review", "accepted", "picking", "packing", "label_printed", "ready_to_ship"} {
		if _, _, err := e.svc.ChangeOrderStatus(e.ctx, id, to, admin, "", false); err != nil {
			t.Fatalf("-> %s: %v", to, err)
		}
	}
	got, _, err := e.svc.ShipOrder(e.ctx, id, admin)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "posted" {
		t.Fatalf("status = %q, want posted", got.Status)
	}
	if in, res := e.balance(t, product); in != 7 || res != 0 {
		t.Fatalf("in_stock/reserved = %d/%d, want 7/0", in, res)
	}
}

func TestORD07_CancelOrderReleasesReservation(t *testing.T) {
	e := newEnv(t)
	id, product := e.order(t, 10, 3)
	if _, _, err := e.svc.ChangeOrderStatus(e.ctx, id, "in_review", admin, "", false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.AcceptOrder(e.ctx, id, admin); err != nil {
		t.Fatal(err)
	}
	got, _, err := e.svc.CancelOrder(e.ctx, id, admin)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "cancelled" {
		t.Fatalf("status = %q, want cancelled", got.Status)
	}
	if in, res := e.balance(t, product); in != 10 || res != 0 {
		t.Fatalf("in_stock/reserved = %d/%d, want 10/0", in, res)
	}
}

// TestORD03_StatusOnlyChangesThroughTheEngine: the generic update must not be
// a way around the workflow.
func TestORD03_StatusOnlyChangesThroughTheEngine(t *testing.T) {
	e := newEnv(t)
	id, _ := e.order(t, 10, 1)
	_, _, err := e.svc.ChangeOrderStatus(e.ctx, id, "delivered", admin, "", false)
	var pe httpx.ProblemError
	if !errors.As(err, &pe) || pe.Problem().Status != http.StatusConflict {
		t.Fatalf("new -> delivered: err = %v, want 409", err)
	}
}
