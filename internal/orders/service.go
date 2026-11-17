package orders

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/audit"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
)

// Service holds the business logic for orders (ORD-01 to ORD-08).
type Service struct {
	store *Store
	rec   *audit.Recorder
	pool  *pgxpool.Pool
}

func NewService(store *Store, rec *audit.Recorder, pool *pgxpool.Pool) *Service {
	return &Service{store: store, rec: rec, pool: pool}
}

// ─── Orders ───────────────────────────────────────────────────────────────────

// ListOrders returns all active orders (ORD-01).
func (s *Service) ListOrders(ctx context.Context, status *string, customerID *uuid.UUID, fromDate, toDate *string) ([]Order, error) {
	return s.store.ListOrders(ctx, status, customerID, fromDate, toDate)
}

// CreateOrder creates a new order (ORD-01).
func (s *Service) CreateOrder(ctx context.Context, customerID uuid.UUID, externalID, channel, status, currency string, exchangeRateSek, subtotalSek, taxSek, freightSek, commissionSek, totalSek int64, shippingName, shippingAddress, shippingCity, shippingPostalCode, shippingCountry, billingName, billingAddress, billingCity, billingPostalCode, billingCountry, trackingNumber, carrier, shippingLabelUrl, notes string) (Order, error) {
	var order Order
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		id := uuid.New()
		var err error
		order, err = st.CreateOrder(ctx, id, customerID, externalID, channel, status, currency, exchangeRateSek, subtotalSek, taxSek, freightSek, commissionSek, totalSek, shippingName, shippingAddress, shippingCity, shippingPostalCode, shippingCountry, billingName, billingAddress, billingCity, billingPostalCode, billingCountry, trackingNumber, carrier, shippingLabelUrl, nil, nil, notes)
		if err != nil {
			return err
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "orders",
			EntityType: "order",
			EntityID:   order.ID,
			Action:     "Create",
			After:      map[string]any{"channel": channel, "status": status},
		})
	})
	return order, err
}

// GetOrder returns an order by ID.
func (s *Service) GetOrder(ctx context.Context, id uuid.UUID) (Order, error) {
	order, err := s.store.GetOrder(ctx, id)
	if err != nil {
		return order, ProblemError(err)
	}
	return order, nil
}

// GetOrderLine returns an order line by ID.
func (s *Service) GetOrderLine(ctx context.Context, id uuid.UUID) (OrderLine, error) {
	line, err := s.store.GetOrderLine(ctx, id)
	if err != nil {
		return line, ProblemError(err)
	}
	return line, nil
}

// GetOrderByExternalID returns an order by external marketplace ID (ORD-06).
func (s *Service) GetOrderByExternalID(ctx context.Context, externalID string) (Order, error) {
	order, err := s.store.GetOrderByExternalId(ctx, externalID)
	if err != nil {
		return order, ProblemError(err)
	}
	return order, nil
}

// UpdateOrder updates an order (ORD-01).
func (s *Service) UpdateOrder(ctx context.Context, externalID, channel, status, currency, trackingNumber, carrier, shippingLabelUrl, notes *string, exchangeRateSek, subtotalSek, taxSek, freightSek, commissionSek, totalSek *int64, customerID *uuid.UUID, shippingName, shippingAddress, shippingCity, shippingPostalCode, shippingCountry, billingName, billingAddress, billingCity, billingPostalCode, billingCountry *string, id uuid.UUID, version int) (Order, error) {
	var updated Order
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		order, err := st.GetOrder(ctx, id)
		if err != nil {
			return ProblemError(err)
		}
		updated, err = st.UpdateOrder(ctx, externalID, channel, status, currency, trackingNumber, carrier, shippingLabelUrl, notes, exchangeRateSek, subtotalSek, taxSek, freightSek, commissionSek, totalSek, customerID, shippingName, shippingAddress, shippingCity, shippingPostalCode, shippingCountry, billingName, billingAddress, billingCity, billingPostalCode, billingCountry, nil, nil, id, version)
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
			Module:     "orders",
			EntityType: "order",
			EntityID:   id,
			Action:     "Update",
			Before:     map[string]any{"status": order.Status, "notes": order.Notes},
			After:      after,
		})
	})
	return updated, err
}

// ArchiveOrder archives an order (NFK-09).
func (s *Service) ArchiveOrder(ctx context.Context, id uuid.UUID, version int) (Order, error) {
	var updated Order
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		order, err := st.GetOrder(ctx, id)
		if err != nil {
			return ProblemError(err)
		}
		updated, err = st.ArchiveOrder(ctx, id, version)
		if err != nil {
			return ProblemError(err)
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "orders",
			EntityType: "order",
			EntityID:   id,
			Action:     "Archive",
			Before:     map[string]any{"channel": order.Channel},
		})
	})
	return updated, err
}

// UpdateOrderStatus changes the order status and runs workflow effects (WF-06).
func (s *Service) UpdateOrderStatus(ctx context.Context, status string, shippedAt, deliveredAt *string, trackingNumber, carrier *string, id uuid.UUID, version int) (Order, error) {
	var updated Order
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		order, err := st.GetOrder(ctx, id)
		if err != nil {
			return ProblemError(err)
		}
		updated, err = st.UpdateOrderStatus(ctx, status, shippedAt, deliveredAt, trackingNumber, carrier, id, version)
		if err != nil {
			return ProblemError(err)
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "orders",
			EntityType: "order",
			EntityID:   id,
			Action:     "UpdateStatus",
			Before:     map[string]any{"status": order.Status},
			After:      map[string]any{"status": status},
		})
	})
	return updated, err
}

// ─── Order Lines ──────────────────────────────────────────────────────────────

// ListOrderLines returns all active lines for an order (ORD-01).
func (s *Service) ListOrderLines(ctx context.Context, orderID uuid.UUID) ([]OrderLine, error) {
	return s.store.ListOrderLines(ctx, orderID)
}

// CreateOrderLine creates a new order line (ORD-01).
func (s *Service) CreateOrderLine(ctx context.Context, orderID, productID uuid.UUID, quantity, quantityShipped, unitPriceSek, taxAmountSek, totalSek int64, taxRate float64) (OrderLine, error) {
	var line OrderLine
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		id := uuid.New()
		var err error
		line, err = st.CreateOrderLine(ctx, id, orderID, productID, quantity, quantityShipped, unitPriceSek, taxAmountSek, totalSek, taxRate)
		if err != nil {
			return err
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "orders",
			EntityType: "orderline",
			EntityID:   line.ID,
			Action:     "Create",
			After:      map[string]any{"product_id": productID.String(), "quantity": quantity},
		})
	})
	return line, err
}

// UpdateOrderLine updates an order line (ORD-01).
func (s *Service) UpdateOrderLine(ctx context.Context, quantity, quantityShipped *int, unitPriceSek, taxAmountSek, totalSek *int64, taxRate *float64, id uuid.UUID, version int) (OrderLine, error) {
	var updated OrderLine
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		line, err := st.GetOrderLine(ctx, id)
		if err != nil {
			return ProblemError(err)
		}
		updated, err = st.UpdateOrderLine(ctx, quantity, quantityShipped, unitPriceSek, taxAmountSek, totalSek, taxRate, id, version)
		if err != nil {
			return ProblemError(err)
		}

		after := map[string]any{}
		if quantity != nil {
			after["quantity"] = *quantity
		}
		if quantityShipped != nil {
			after["quantity_shipped"] = *quantityShipped
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "orders",
			EntityType: "orderline",
			EntityID:   id,
			Action:     "Update",
			Before:     map[string]any{"quantity": line.Quantity, "quantity_shipped": line.QuantityShipped},
			After:      after,
		})
	})
	return updated, err
}

// ArchiveOrderLine archives an order line (NFK-09).
func (s *Service) ArchiveOrderLine(ctx context.Context, id uuid.UUID, version int) (OrderLine, error) {
	var updated OrderLine
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		line, err := st.GetOrderLine(ctx, id)
		if err != nil {
			return ProblemError(err)
		}
		updated, err = st.ArchiveOrderLine(ctx, id, version)
		if err != nil {
			return ProblemError(err)
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "orders",
			EntityType: "orderline",
			EntityID:   id,
			Action:     "Archive",
			Before:     map[string]any{"product_id": line.ProductID, "quantity": line.Quantity},
		})
	})
	return updated, err
}

// AcceptOrder accepts an order with preview and optional override (ORD-04, WF-05).
func (s *Service) AcceptOrder(ctx context.Context, status string, reason *string, id uuid.UUID) (Order, error) {
	var updated Order
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		order, err := st.GetOrder(ctx, id)
		if err != nil {
			return ProblemError(err)
		}
		updated, err = st.UpdateOrderStatus(ctx, status, nil, nil, nil, nil, id, order.Version)
		if err != nil {
			return ProblemError(err)
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "orders",
			EntityType: "order",
			EntityID:   id,
			Action:     "Accept",
			Before:     map[string]any{"status": order.Status},
			Reason:     reasonString(reason),
		})
	})
	return updated, err
}

// ShipOrder ships an order (ORD-06).
func (s *Service) ShipOrder(ctx context.Context, status string, id uuid.UUID) (Order, error) {
	var updated Order
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		order, err := st.GetOrder(ctx, id)
		if err != nil {
			return ProblemError(err)
		}
		updated, err = st.UpdateOrderStatus(ctx, status, nil, nil, nil, nil, id, order.Version)
		if err != nil {
			return ProblemError(err)
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "orders",
			EntityType: "order",
			EntityID:   id,
			Action:     "Ship",
			Before:     map[string]any{"status": order.Status},
		})
	})
	return updated, err
}

// CancelOrder cancels an order (ORD-07).
func (s *Service) CancelOrder(ctx context.Context, status string, id uuid.UUID) (Order, error) {
	var updated Order
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		order, err := st.GetOrder(ctx, id)
		if err != nil {
			return ProblemError(err)
		}
		updated, err = st.UpdateOrderStatus(ctx, status, nil, nil, nil, nil, id, order.Version)
		if err != nil {
			return ProblemError(err)
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "orders",
			EntityType: "order",
			EntityID:   id,
			Action:     "Cancel",
			Before:     map[string]any{"status": order.Status},
		})
	})
	return updated, err
}

// DeleteOrderLine deletes an order line (NFK-09).
func (s *Service) DeleteOrderLine(ctx context.Context, id uuid.UUID) (OrderLine, error) {
	var updated OrderLine
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		line, err := st.GetOrderLine(ctx, id)
		if err != nil {
			return ProblemError(err)
		}
		updated, err = st.ArchiveOrderLine(ctx, id, line.Version)
		if err != nil {
			return ProblemError(err)
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "order_lines",
			EntityType: "order_line",
			EntityID:   id,
			Action:     "Delete",
			Before:     map[string]any{"product_id": line.ProductID, "quantity": line.Quantity},
		})
	})
	return updated, err
}

// CalculateTotals calculates order totals from lines (ORD-02).
func (s *Service) CalculateTotals(lines []OrderLine) (subtotalSek, taxSek, totalSek int64) {
	for _, l := range lines {
		subtotalSek += l.TotalSek - l.TaxAmountSek
		taxSek += l.TaxAmountSek
		totalSek += l.TotalSek
	}
	return subtotalSek, taxSek, totalSek
}

// FormatAmount returns an amount in öre formatted as SEK string (ORD-02).
func FormatAmount(amountSek int64) string {
	return fmt.Sprintf("%d kr", amountSek/100)
}

func reasonString(r *string) string {
	if r == nil {
		return ""
	}
	return *r
}
