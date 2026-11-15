package purchasing

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db/dbgen"
)

// Store wraps sqlc queries for the purchasing module.
type Store struct {
	q *dbgen.Queries
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{q: dbgen.New(pool)}
}

// ─── Purchase Orders ──────────────────────────────────────────────────────────

func (s *Store) ListPurchaseOrders(ctx context.Context) ([]PurchaseOrder, error) {
	rows, err := s.q.ListPurchaseOrders(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]PurchaseOrder, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToPurchaseOrder(r))
	}
	return out, nil
}

func (s *Store) ListPurchaseOrdersByStatus(ctx context.Context, status string) ([]PurchaseOrder, error) {
	rows, err := s.q.ListPurchaseOrdersByStatus(ctx, status)
	if err != nil {
		return nil, err
	}
	out := make([]PurchaseOrder, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToPurchaseOrder(r))
	}
	return out, nil
}

func (s *Store) ListPurchaseOrdersBySupplier(ctx context.Context, supplierID uuid.UUID) ([]PurchaseOrder, error) {
	rows, err := s.q.ListPurchaseOrdersBySupplier(ctx, supplierID)
	if err != nil {
		return nil, err
	}
	out := make([]PurchaseOrder, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToPurchaseOrder(r))
	}
	return out, nil
}

func (s *Store) GetPurchaseOrder(ctx context.Context, id uuid.UUID) (PurchaseOrder, error) {
	row, err := s.q.GetPurchaseOrder(ctx, id)
	if err != nil {
		return PurchaseOrder{}, err
	}
	return dbRowToPurchaseOrder(row), nil
}

func (s *Store) CreatePurchaseOrder(ctx context.Context, id, supplierID uuid.UUID, orderNumber, currency string, orderDate, expectedDate *time.Time, status string, totalAmount int64, notes string) (PurchaseOrder, error) {
	var orderDatePg, expectedDatePg pgtype.Date
	if orderDate != nil {
		orderDatePg = pgtype.Date{Time: *orderDate, Valid: true}
	}
	var expectedDatePgVal pgtype.Date
	if expectedDate != nil {
		expectedDatePgVal = pgtype.Date{Time: *expectedDate, Valid: true}
		expectedDatePg = expectedDatePgVal
	}
	row, err := s.q.CreatePurchaseOrder(ctx, dbgen.CreatePurchaseOrderParams{
		ID:           id,
		SupplierID:   supplierID,
		OrderNumber:  orderNumber,
		Currency:     currency,
		OrderDate:    orderDatePg,
		ExpectedDate: expectedDatePg,
		Status:       status,
		TotalAmount:  totalAmount,
		Notes:        notes,
	})
	if err != nil {
		return PurchaseOrder{}, err
	}
	return dbRowToPurchaseOrder(row), nil
}

func (s *Store) UpdatePurchaseOrder(ctx context.Context, status *string, expectedDate *time.Time, totalAmount *int64, notes *string, id uuid.UUID, version int) (PurchaseOrder, error) {
	var params dbgen.UpdatePurchaseOrderParams
	params.ID = id
	params.Version = int32(version) //nolint:gosec // version is bounded by optimistic locking
	if status != nil {
		params.Status = *status
	}
	if totalAmount != nil {
		params.TotalAmount = *totalAmount
	}
	if notes != nil {
		params.Notes = *notes
	}
	if expectedDate != nil {
		params.ExpectedDate = pgtype.Date{Time: *expectedDate, Valid: true}
	}
	row, err := s.q.UpdatePurchaseOrder(ctx, params)
	if err != nil {
		return PurchaseOrder{}, err
	}
	return dbRowToPurchaseOrder(row), nil
}

func (s *Store) ArchivePurchaseOrder(ctx context.Context, id uuid.UUID, version int) (PurchaseOrder, error) {
	row, err := s.q.ArchivePurchaseOrder(ctx, dbgen.ArchivePurchaseOrderParams{ID: id, Version: int32(version)}) //nolint:gosec // version is bounded by optimistic locking
	if err != nil {
		return PurchaseOrder{}, err
	}
	return dbRowToPurchaseOrder(row), nil
}

// ─── Purchase Order Lines ─────────────────────────────────────────────────────

func (s *Store) ListPurchaseOrderLines(ctx context.Context, poID uuid.UUID) ([]PurchaseOrderLine, error) {
	rows, err := s.q.ListPurchaseOrderLines(ctx, poID)
	if err != nil {
		return nil, err
	}
	out := make([]PurchaseOrderLine, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToPurchaseOrderLine(r))
	}
	return out, nil
}

func (s *Store) CreatePurchaseOrderLine(ctx context.Context, id, poID, productID uuid.UUID, quantity int, unitPrice, lineTotal int64, unitCurrency string) (PurchaseOrderLine, error) {
	row, err := s.q.CreatePurchaseOrderLine(ctx, dbgen.CreatePurchaseOrderLineParams{
		ID:              id,
		PurchaseOrderID: poID,
		ProductID:       productID,
		Quantity:        int32(quantity), //nolint:gosec // quantity is bounded by business logic
		UnitPrice:       unitPrice,
		UnitCurrency:    unitCurrency,
		LineTotal:       lineTotal,
	})
	if err != nil {
		return PurchaseOrderLine{}, err
	}
	return dbRowToPurchaseOrderLine(row), nil
}

func (s *Store) UpdatePurchaseOrderLine(ctx context.Context, quantity, unitPrice, lineTotal *int, id uuid.UUID, version int) (PurchaseOrderLine, error) {
	var params dbgen.UpdatePurchaseOrderLineParams
	params.ID = id
	params.Version = int32(version) //nolint:gosec // version is bounded by optimistic locking
	if quantity != nil {
		params.Quantity = int32(*quantity) //nolint:gosec // quantity is bounded by business logic
	}
	if unitPrice != nil {
		params.UnitPrice = int64(*unitPrice)
	}
	if lineTotal != nil {
		params.LineTotal = int64(*lineTotal)
	}
	row, err := s.q.UpdatePurchaseOrderLine(ctx, params)
	if err != nil {
		return PurchaseOrderLine{}, err
	}
	return dbRowToPurchaseOrderLine(row), nil
}

func (s *Store) ArchivePurchaseOrderLine(ctx context.Context, id uuid.UUID, version int) (PurchaseOrderLine, error) {
	row, err := s.q.ArchivePurchaseOrderLine(ctx, dbgen.ArchivePurchaseOrderLineParams{ID: id, Version: int32(version)}) //nolint:gosec // version is bounded by optimistic locking
	if err != nil {
		return PurchaseOrderLine{}, err
	}
	return dbRowToPurchaseOrderLine(row), nil
}

// ─── Row helpers ──────────────────────────────────────────────────────────────

func dbRowToPurchaseOrder(r dbgen.PurchaseOrder) PurchaseOrder {
	po := PurchaseOrder{
		ID:          r.ID,
		SupplierID:  r.SupplierID,
		OrderNumber: r.OrderNumber,
		Currency:    r.Currency,
		Status:      r.Status,
		TotalAmount: r.TotalAmount,
		Notes:       r.Notes,
		Version:     int(r.Version),
		CreatedAt:   r.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   r.UpdatedAt.Format(time.RFC3339),
	}
	if r.OrderDate.Valid {
		po.OrderDate = r.OrderDate.Time.Format("2006-01-02")
	}
	if r.ExpectedDate.Valid {
		s := r.ExpectedDate.Time.Format("2006-01-02")
		po.ExpectedDate = &s
	}
	if r.ArchivedAt != nil {
		po.Archived = true
		t := r.ArchivedAt.Format(time.RFC3339)
		po.ArchivedAt = &t
	}
	return po
}

func dbRowToPurchaseOrderLine(r dbgen.PurchaseOrderLine) PurchaseOrderLine {
	line := PurchaseOrderLine{
		ID:              r.ID,
		PurchaseOrderID: r.PurchaseOrderID,
		ProductID:       r.ProductID,
		Quantity:        int(r.Quantity),
		UnitPrice:       r.UnitPrice,
		UnitCurrency:    r.UnitCurrency,
		LineTotal:       r.LineTotal,
		Version:         int(r.Version),
		CreatedAt:       r.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       r.UpdatedAt.Format(time.RFC3339),
	}
	if r.ArchivedAt != nil {
		line.Archived = true
		t := r.ArchivedAt.Format(time.RFC3339)
		line.ArchivedAt = &t
	}
	return line
}
