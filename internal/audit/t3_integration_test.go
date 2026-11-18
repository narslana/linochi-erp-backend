//go:build integration

package audit_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/audit"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/boards"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/calendar"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/customers"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/ids"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/inventory"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/kiosk"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/products"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/purchasing"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/reminders"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/suppliers"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/workflow"
)

// ─── helpers ────────────────────────────────────────────────────────────────

type auditRow struct {
	Module  string
	Action  string
	ActorID string
	Reason  sql.NullString
}

func queryAuditRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, module, action string) []auditRow {
	t.Helper()
	rows, err := pool.Query(ctx,
		"SELECT module, action, actor_id, reason FROM audit_log WHERE module = $1 AND action = $2 ORDER BY id",
		module, action)
	if err != nil {
		t.Fatalf("query audit rows: %v", err)
	}
	defer rows.Close()

	var result []auditRow
	for rows.Next() {
		var r auditRow
		if err := rows.Scan(&r.Module, &r.Action, &r.ActorID, &r.Reason); err != nil {
			t.Fatalf("scan audit row: %v", err)
		}
		result = append(result, r)
	}
	return result
}

func assertAuditRow(t *testing.T, ctx context.Context, pool *pgxpool.Pool, module, action, actorID string, reason string) {
	t.Helper()
	rows := queryAuditRows(t, ctx, pool, module, action)
	if len(rows) != 1 {
		t.Fatalf("expected 1 audit row for %s/%s, got %d", module, action, len(rows))
	}
	r := rows[0]
	if r.Module != module {
		t.Errorf("module = %q, want %q", r.Module, module)
	}
	if r.Action != action {
		t.Errorf("action = %q, want %q", r.Action, action)
	}
	if r.ActorID != actorID {
		t.Errorf("actor_id = %q, want %q", r.ActorID, actorID)
	}
	if reason != "" {
		if !r.Reason.Valid || r.Reason.String != reason {
			t.Errorf("reason = %v, want %q", r.Reason, reason)
		}
	} else {
		if r.Reason.Valid {
			t.Errorf("reason = %q, want nil", r.Reason.String)
		}
	}
}

// ─── products ───────────────────────────────────────────────────────────────

func TestINS05_ProductsWriteAndAuditCommitTogether(t *testing.T) {
	ctx := actorCtx()
	e := newEnv(t)

	store := products.NewStore(e.pool)
	svc := products.NewService(store, e.rec, e.pool)

	// Create a product type.
	pt, err := svc.CreateProductType(ctx, "Audit Test Type", "Audit Test EN", "AUD")
	if err != nil {
		t.Fatal(err)
	}
	assertAuditRow(t, ctx, e.pool, "product_types", "Create", userID.String(), "")

	// Create a field definition.
	_, err = svc.CreateFieldDefinition(ctx, "Audit Field", "Audit EN", "short_text", "test", "", true, json.RawMessage(`[]`), "", false)
	if err != nil {
		t.Fatal(err)
	}
	assertAuditRow(t, ctx, e.pool, "field_definitions", "Create", userID.String(), "")

	// Create a product.
	attrs := json.RawMessage(`{}`)
	p, err := svc.CreateProduct(ctx, "Audit Product", "LIN-AUD-500", "1234567890128", pt.ID, "Short", 10000, 15000, "SEK", "SEK", "active", attrs)
	if err != nil {
		t.Fatal(err)
	}
	assertAuditRow(t, ctx, e.pool, "products", "Create", userID.String(), "")

	// Update the product.
	_, err = svc.UpdateProduct(ctx, p.ID, "Updated", "LIN-AUD-500", "1234567890128", pt.ID, "Short", 10000, 15000, "SEK", "SEK", "active", attrs, p.Version)
	if err != nil {
		t.Fatal(err)
	}
	assertAuditRow(t, ctx, e.pool, "products", "Update", userID.String(), "")

	// Archive the product.
	_, err = svc.ArchiveProduct(ctx, p.ID, p.Version+1)
	if err != nil {
		t.Fatal(err)
	}
	assertAuditRow(t, ctx, e.pool, "products", "Archive", userID.String(), "")
}

// ─── ids ────────────────────────────────────────────────────────────────────

func TestINS05_IdsWriteAndAuditCommitTogether(t *testing.T) {
	ctx := actorCtx()
	e := newEnv(t)

	store := ids.NewStore(e.pool)
	svc := ids.NewService(store, e.rec, e.pool)

	entry, err := svc.CreateIdsEntry(ctx, "EPR", "Test Entry", "EPR-DE-12345", "LUCID", "Linochi", "Test note", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertAuditRow(t, ctx, e.pool, "ids_entries", "Create", userID.String(), "")

	// Update.
	_, err = svc.UpdateIdsEntry(ctx, entry.ID, "EPR", "Updated", "EPR-DE-12345", "LUCID", "Linochi", "Updated note", "", nil, nil, entry.Version)
	if err != nil {
		t.Fatal(err)
	}
	assertAuditRow(t, ctx, e.pool, "ids_entries", "Update", userID.String(), "")

	// Archive.
	_, err = svc.ArchiveIdsEntry(ctx, entry.ID, entry.Version+1)
	if err != nil {
		t.Fatal(err)
	}
	assertAuditRow(t, ctx, e.pool, "ids_entries", "Archive", userID.String(), "")
}

// ─── reminders ──────────────────────────────────────────────────────────────

func TestINS05_RemindersWriteAndAuditCommitTogether(t *testing.T) {
	ctx := actorCtx()
	e := newEnv(t)

	store := reminders.NewStore(e.pool)
	svc := reminders.NewService(store, e.rec, e.pool)

	// Create a valid ids_entry first.
	idsStore := ids.NewStore(e.pool)
	idsSvc := ids.NewService(idsStore, e.rec, e.pool)
	entry, err := idsSvc.CreateIdsEntry(ctx, "EPR", "Reminder Entry", "EPR-REM-001", "LUCID", "Linochi", "For reminders test", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	reminder, err := svc.CreateReminder(ctx, entry.ID, uuid.Nil, "ids_entry", "ext", "Test Reminder", "Test EN", "Desc", "2027-01-01")
	if err != nil {
		t.Fatal(err)
	}
	assertAuditRow(t, ctx, e.pool, "reminders", "Create", userID.String(), "")

	// Update.
	_, err = svc.UpdateReminder(ctx, ptrStr("Updated"), nil, nil, ptrStr("2027-02-01"), reminder.ID, reminder.Version)
	if err != nil {
		t.Fatal(err)
	}
	assertAuditRow(t, ctx, e.pool, "reminders", "Update", userID.String(), "")

	// Archive.
	_, err = svc.ArchiveReminder(ctx, reminder.ID, reminder.Version+1)
	if err != nil {
		t.Fatal(err)
	}
	assertAuditRow(t, ctx, e.pool, "reminders", "Archive", userID.String(), "")
}

// ─── suppliers ──────────────────────────────────────────────────────────────

func TestINS05_SuppliersWriteAndAuditCommitTogether(t *testing.T) {
	ctx := actorCtx()
	e := newEnv(t)

	store := suppliers.NewStore(e.pool)
	svc := suppliers.NewService(store, e.rec, e.pool)

	supplier, err := svc.CreateSupplier(ctx, "Audit Supplier", "Audit EN", "1234567890", "", "SE", "Street 1", "Stockholm", "11122", "", "test@test.se", "123456", "manufacturer", 30, 100, "SEK", "NET30", "DEL", "Notes")
	if err != nil {
		t.Fatal(err)
	}
	assertAuditRow(t, ctx, e.pool, "suppliers", "Create", userID.String(), "")

	// Update.
	_, err = svc.UpdateSupplier(ctx, supplier.ID, "Updated", "Updated EN", "1234567890", "", "SE", "Street 1", "Stockholm", "11122", "", "test@test.se", "123456", "manufacturer", "NET30", "DEL", "Updated", nil, nil, supplier.Version)
	if err != nil {
		t.Fatal(err)
	}
	assertAuditRow(t, ctx, e.pool, "suppliers", "Update", userID.String(), "")

	// Archive.
	_, err = svc.ArchiveSupplier(ctx, supplier.ID, supplier.Version+1)
	if err != nil {
		t.Fatal(err)
	}
	assertAuditRow(t, ctx, e.pool, "suppliers", "Archive", userID.String(), "")
}

// ─── purchasing ─────────────────────────────────────────────────────────────

func TestINS05_PurchasingWriteAndAuditCommitTogether(t *testing.T) {
	ctx := actorCtx()
	e := newEnv(t)

	// Create a supplier first.
	supStore := suppliers.NewStore(e.pool)
	supSvc := suppliers.NewService(supStore, e.rec, e.pool)
	supplier, err := supSvc.CreateSupplier(ctx, "PO Supplier", "", "111", "", "SE", "", "", "", "", "", "", "manufacturer", 0, 0, "SEK", "", "", "")
	if err != nil {
		t.Fatal(err)
	}

	store := purchasing.NewStore(e.pool)
	svc := purchasing.NewService(store, e.rec, e.pool)

	po, err := svc.CreatePurchaseOrder(ctx, supplier.ID, "PO-001", "SEK", ptrTime(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	assertAuditRow(t, ctx, e.pool, "purchasing", "Create", userID.String(), "")

	// Update.
	_, err = svc.UpdatePurchaseOrder(ctx, po.ID, ptrStr("ordered"), nil, nil, nil, po.Version)
	if err != nil {
		t.Fatal(err)
	}
	assertAuditRow(t, ctx, e.pool, "purchasing", "Update", userID.String(), "")

	// Archive.
	_, err = svc.ArchivePurchaseOrder(ctx, po.ID, po.Version+1)
	if err != nil {
		t.Fatal(err)
	}
	assertAuditRow(t, ctx, e.pool, "purchasing", "Archive", userID.String(), "")
}

// ─── inventory ──────────────────────────────────────────────────────────────

func TestINS05_InventoryWriteAndAuditCommitTogether(t *testing.T) {
	ctx := actorCtx()
	e := newEnv(t)

	store := inventory.NewStore(e.pool)
	svc := inventory.NewService(store, e.rec, e.pool)

	// Create location.
	loc, err := svc.CreateLocation(ctx, "Test Location", "Test EN", "Street 1", "Stockholm", "SE")
	if err != nil {
		t.Fatal(err)
	}
	assertAuditRow(t, ctx, e.pool, "inventory", "Create", userID.String(), "")

	// Update the location.
	_, err = svc.UpdateLocation(ctx, loc.ID, ptrStr("Updated Location"), nil, nil, nil, nil, loc.Version)
	if err != nil {
		t.Fatal(err)
	}
	assertAuditRow(t, ctx, e.pool, "inventory", "Update", userID.String(), "")

	// Archive the location.
	_, err = svc.ArchiveLocation(ctx, loc.ID, loc.Version+1)
	if err != nil {
		t.Fatal(err)
	}
	assertAuditRow(t, ctx, e.pool, "inventory", "Archive", userID.String(), "")
}

// ─── customers ──────────────────────────────────────────────────────────────

func TestINS05_CustomersWriteAndAuditCommitTogether(t *testing.T) {
	ctx := actorCtx()
	e := newEnv(t)

	store := customers.NewStore(e.pool)
	svc := customers.NewService(store, e.rec, e.pool)

	cust, err := svc.CreateCustomer(ctx, "Audit Customer", "shop", "Street 1", "Stockholm", "SE", "", "", "SEK", "")
	if err != nil {
		t.Fatal(err)
	}
	assertAuditRow(t, ctx, e.pool, "customers", "Create", userID.String(), "")

	// Update.
	_, err = svc.UpdateCustomer(ctx, ptrStr("Updated Customer"), nil, nil, nil, nil, nil, nil, nil, nil, cust.ID, cust.Version)
	if err != nil {
		t.Fatal(err)
	}
	assertAuditRow(t, ctx, e.pool, "customers", "Update", userID.String(), "")

	// Archive.
	_, err = svc.ArchiveCustomer(ctx, cust.ID, cust.Version+1)
	if err != nil {
		t.Fatal(err)
	}
	assertAuditRow(t, ctx, e.pool, "customers", "Archive", userID.String(), "")
}

// ─── workflow ───────────────────────────────────────────────────────────────

func TestINS05_WorkflowWriteAndAuditCommitTogether(t *testing.T) {
	ctx := actorCtx()
	e := newEnv(t)

	inv := inventory.NewService(inventory.NewStore(e.pool), e.rec, e.pool)
	svc := workflow.NewService(workflow.NewStore(e.pool), e.rec, e.pool, inv)

	// Publishing a version (T2: the old step/transition CRUD is gone) writes
	// the version and its audit row together.
	active, err := svc.ActiveVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PublishVersion(ctx, active.Definition()); err != nil {
		t.Fatal(err)
	}
	assertAuditRow(t, ctx, e.pool, "workflow", "create", userID.String(), "")
}

// ─── calendar ───────────────────────────────────────────────────────────────

func TestINS05_CalendarWriteAndAuditCommitTogether(t *testing.T) {
	ctx := actorCtx()
	e := newEnv(t)

	store := calendar.NewStore(e.pool)
	svc := calendar.NewService(store, e.rec, e.pool)

	// Create an iCal source.
	source, err := svc.CreateICalSource(ctx, "Test Calendar", "Test EN", "https://example.com/calendar.ics", "#FF0000", 60)
	if err != nil {
		t.Fatal(err)
	}
	assertAuditRow(t, ctx, e.pool, "calendar", "Create", userID.String(), "")

	// Update.
	_, err = svc.UpdateICalSource(ctx, source.ID, ptrStr("Updated"), nil, nil, nil, nil, nil, source.Version)
	if err != nil {
		t.Fatal(err)
	}
	assertAuditRow(t, ctx, e.pool, "calendar", "Update", userID.String(), "")

	// Archive.
	_, err = svc.ArchiveICalSource(ctx, source.ID, source.Version+1)
	if err != nil {
		t.Fatal(err)
	}
	assertAuditRow(t, ctx, e.pool, "calendar", "Archive", userID.String(), "")
}

// ─── kiosk ──────────────────────────────────────────────────────────────────

func TestINS05_KioskWriteAndAuditCommitTogether(t *testing.T) {
	ctx := actorCtx()
	e := newEnv(t)

	store := kiosk.NewStore(e.pool)
	svc := kiosk.NewService(store, e.rec, e.pool)

	// Create a playlist.
	playlist, err := svc.CreateKioskPlaylist(ctx, "Kiosk Playlist", "Kiosk EN")
	if err != nil {
		t.Fatal(err)
	}
	assertAuditRow(t, ctx, e.pool, "kiosk", "Create", userID.String(), "")

	// Update the playlist.
	_, err = svc.UpdateKioskPlaylist(ctx, playlist.ID, ptrStr("Updated Playlist"), nil, playlist.Version)
	if err != nil {
		t.Fatal(err)
	}
	assertAuditRow(t, ctx, e.pool, "kiosk", "Update", userID.String(), "")

	// Archive the playlist.
	_, err = svc.ArchiveKioskPlaylist(ctx, playlist.ID, playlist.Version+1)
	if err != nil {
		t.Fatal(err)
	}
	assertAuditRow(t, ctx, e.pool, "kiosk", "Archive", userID.String(), "")
}

// ─── boards ─────────────────────────────────────────────────────────────────

func TestINS05_BoardsWriteAndAuditCommitTogether(t *testing.T) {
	ctx := actorCtx()
	e := newEnv(t)

	store := boards.NewStore(e.pool)
	svc := boards.NewService(store, e.rec, e.pool)

	board, err := svc.CreateBoard(ctx, "Audit Board")
	if err != nil {
		t.Fatal(err)
	}
	assertAuditRow(t, ctx, e.pool, "boards", "Create", userID.String(), "")

	// Update.
	_, err = svc.UpdateBoard(ctx, board.ID, "Updated Board", board.Version)
	if err != nil {
		t.Fatal(err)
	}
	assertAuditRow(t, ctx, e.pool, "boards", "Update", userID.String(), "")

	// Archive.
	_, err = svc.ArchiveBoard(ctx, board.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertAuditRow(t, ctx, e.pool, "boards", "Archive", userID.String(), "")
}

// ─── audit failure rolls back the write ─────────────────────────────────────

func TestINS05_AuditFailureRollsBackTheWrite(t *testing.T) {
	ctx := context.Background() // no actor → Record fails with ErrNoActor
	e := newEnv(t)

	// Create a product type first (needed as FK).
	ptStore := products.NewStore(e.pool)
	ptSvc := products.NewService(ptStore, e.rec, e.pool)
	ptCtx := audit.WithActor(ctx, audit.UserActor(uuid.MustParse("00000000-0000-0000-0000-000000000001")))
	pt, err := ptSvc.CreateProductType(ptCtx, "Fail Rollback Type", "Fail EN", "FAIL")
	if err != nil {
		t.Fatal(err)
	}

	// Try to create a product without an actor.
	pStore := products.NewStore(e.pool)
	pSvc := products.NewService(pStore, e.rec, e.pool)
	_, err = pSvc.CreateProduct(ctx, "Fail Rollback", "LIN-FAIL-RB", "", pt.ID, "Short", 10000, 15000, "SEK", "SEK", "draft", json.RawMessage("{}"))
	if err == nil {
		t.Fatal("expected error when audit fails, got nil")
	}

	// Assert that no product was written.
	var count int
	err = e.pool.QueryRow(ctx, "SELECT count(*) FROM products WHERE sku = $1", "LIN-FAIL-RB").Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected 0 products, got %d (write was not rolled back)", count)
	}
}

// ─── helpers ────────────────────────────────────────────────────────────────

func ptrStr(s string) *string        { return &s }
func ptrTime(t time.Time) *time.Time { return &t }
