package purchasing

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/audit"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
)

// Service holds the business logic for purchase orders and lines.
type Service struct {
	store *Store
	rec   *audit.Recorder
	pool  *pgxpool.Pool
}

func NewService(store *Store, rec *audit.Recorder, pool *pgxpool.Pool) *Service {
	return &Service{store: store, rec: rec, pool: pool}
}

// ListPurchaseOrders returns all active purchase orders.
func (s *Service) ListPurchaseOrders(ctx context.Context, status *string, supplierID *uuid.UUID) ([]PurchaseOrder, error) {
	switch {
	case status != nil:
		return s.store.ListPurchaseOrdersByStatus(ctx, *status)
	case supplierID != nil:
		return s.store.ListPurchaseOrdersBySupplier(ctx, *supplierID)
	default:
		return s.store.ListPurchaseOrders(ctx)
	}
}

// GetPurchaseOrder returns a purchase order by ID.
func (s *Service) GetPurchaseOrder(ctx context.Context, id uuid.UUID) (PurchaseOrder, error) {
	po, err := s.store.GetPurchaseOrder(ctx, id)
	if err != nil {
		return po, ProblemError(err)
	}
	return po, nil
}

// CreatePurchaseOrder creates a new purchase order (INK-01).
func (s *Service) CreatePurchaseOrder(ctx context.Context, supplierID uuid.UUID, orderNumber, currency string, orderDate, expectedDate *time.Time, notes string) (PurchaseOrder, error) {
	var po PurchaseOrder
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		id := uuid.New()
		var err error
		po, err = st.CreatePurchaseOrder(ctx, id, supplierID, orderNumber, currency, orderDate, expectedDate, "draft", 0, notes)
		if err != nil {
			return err
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "purchasing",
			EntityType: "purchase_order",
			EntityID:   po.ID,
			Action:     "Create",
			After:      map[string]any{"supplier_id": supplierID, "status": "draft"},
		})
	})
	return po, err
}

// UpdatePurchaseOrder updates a purchase order (INK-01).
func (s *Service) UpdatePurchaseOrder(ctx context.Context, id uuid.UUID, status *string, expectedDate *time.Time, totalAmount *int64, notes *string, version int) (PurchaseOrder, error) {
	var updated PurchaseOrder
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		po, err := st.GetPurchaseOrder(ctx, id)
		if err != nil {
			return ProblemError(err)
		}
		updated, err = st.UpdatePurchaseOrder(ctx, status, expectedDate, totalAmount, notes, id, version)
		if err != nil {
			return ProblemError(err)
		}

		after := map[string]any{}
		if status != nil {
			after["status"] = *status
		}
		if notes != nil {
			after["notes"] = *notes
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "purchasing",
			EntityType: "purchase_order",
			EntityID:   id,
			Action:     "Update",
			Before:     map[string]any{"status": po.Status, "notes": po.Notes},
			After:      after,
		})
	})
	return updated, err
}

// ArchivePurchaseOrder archives a purchase order (NFK-09).
func (s *Service) ArchivePurchaseOrder(ctx context.Context, id uuid.UUID, version int) (PurchaseOrder, error) {
	var updated PurchaseOrder
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		po, err := st.GetPurchaseOrder(ctx, id)
		if err != nil {
			return ProblemError(err)
		}
		updated, err = st.ArchivePurchaseOrder(ctx, id, version)
		if err != nil {
			return ProblemError(err)
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "purchasing",
			EntityType: "purchase_order",
			EntityID:   id,
			Action:     "Archive",
			Before:     map[string]any{"order_number": po.OrderNumber},
		})
	})
	return updated, err
}

// ListLines returns the lines for a purchase order.
func (s *Service) ListLines(ctx context.Context, poID uuid.UUID) ([]PurchaseOrderLine, error) {
	return s.store.ListPurchaseOrderLines(ctx, poID)
}

// CreateLine adds a line to a purchase order (INK-01).
func (s *Service) CreateLine(ctx context.Context, poID, productID uuid.UUID, quantity int, unitPrice, lineTotal int64, unitCurrency string) (PurchaseOrderLine, error) {
	var line PurchaseOrderLine
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		id := uuid.New()
		var err error
		line, err = st.CreatePurchaseOrderLine(ctx, id, poID, productID, quantity, unitPrice, lineTotal, unitCurrency)
		if err != nil {
			return err
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "purchasing",
			EntityType: "purchase_order_line",
			EntityID:   line.ID,
			Action:     "Create",
			After:      map[string]any{"po_id": poID, "product_id": productID, "quantity": quantity},
		})
	})
	return line, err
}

// UpdateLine updates a purchase order line (INK-01).
func (s *Service) UpdateLine(ctx context.Context, id uuid.UUID, quantity, unitPrice, lineTotal *int, version int) (PurchaseOrderLine, error) {
	var updated PurchaseOrderLine
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		line, err := st.GetPurchaseOrderLine(ctx, id)
		if err != nil {
			return ProblemError(err)
		}
		updated, err = st.UpdatePurchaseOrderLine(ctx, quantity, unitPrice, lineTotal, id, version)
		if err != nil {
			return ProblemError(err)
		}

		after := map[string]any{}
		if quantity != nil {
			after["quantity"] = *quantity
		}
		if lineTotal != nil {
			after["line_total"] = *lineTotal
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "purchasing",
			EntityType: "purchase_order_line",
			EntityID:   id,
			Action:     "Update",
			Before:     map[string]any{"quantity": line.Quantity, "line_total": line.LineTotal},
			After:      after,
		})
	})
	return updated, err
}

// ArchiveLine archives a purchase order line (NFK-09).
func (s *Service) ArchiveLine(ctx context.Context, id uuid.UUID, version int) (PurchaseOrderLine, error) {
	var updated PurchaseOrderLine
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		line, err := st.GetPurchaseOrderLine(ctx, id)
		if err != nil {
			return ProblemError(err)
		}
		updated, err = st.ArchivePurchaseOrderLine(ctx, id, version)
		if err != nil {
			return ProblemError(err)
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "purchasing",
			EntityType: "purchase_order_line",
			EntityID:   id,
			Action:     "Archive",
			Before:     map[string]any{"product_id": line.ProductID, "quantity": line.Quantity},
		})
	})
	return updated, err
}
