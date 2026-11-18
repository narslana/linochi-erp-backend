package inventory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/audit"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db/dbgen"
)

// Service holds the business logic for inventory (LAG-01 to LAG-05).
type Service struct {
	store     *Store
	rec       *audit.Recorder
	pool      *pgxpool.Pool
	movements *prometheus.CounterVec
	refusals  *prometheus.CounterVec
}

func NewService(store *Store, rec *audit.Recorder, pool *pgxpool.Pool) *Service {
	return &Service{store: store, rec: rec, pool: pool}
}

// ─── Locations ────────────────────────────────────────────────────────────────

// ListLocations returns all active locations (LAG-01).
func (s *Service) ListLocations(ctx context.Context) ([]Location, error) {
	return s.store.ListLocations(ctx)
}

// CreateLocation creates a new location (LAG-01).
func (s *Service) CreateLocation(ctx context.Context, name, nameEn, address, city, country string) (Location, error) {
	var loc Location
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		id := uuid.New()
		var err error
		loc, err = st.CreateLocation(ctx, id, name, nameEn, address, city, country)
		if err != nil {
			return err
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "inventory",
			EntityType: "location",
			EntityID:   loc.ID,
			Action:     "Create",
			After:      map[string]any{"name": name, "city": city},
		})
	})
	return loc, err
}

// GetLocation returns a location by ID.
func (s *Service) GetLocation(ctx context.Context, id uuid.UUID) (Location, error) {
	loc, err := s.store.GetLocation(ctx, id)
	if err != nil {
		return loc, ProblemError(err)
	}
	return loc, nil
}

// UpdateLocation updates a location (LAG-01).
func (s *Service) UpdateLocation(ctx context.Context, id uuid.UUID, name, nameEn, address, city, country *string, version int) (Location, error) {
	var updated Location
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		loc, err := st.GetLocation(ctx, id)
		if err != nil {
			return ProblemError(err)
		}
		updated, err = st.UpdateLocation(ctx, name, nameEn, address, city, country, id, version)
		if err != nil {
			return ProblemError(err)
		}

		after := map[string]any{}
		if name != nil {
			after["name"] = *name
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "inventory",
			EntityType: "location",
			EntityID:   id,
			Action:     "Update",
			Before:     map[string]any{"name": loc.Name},
			After:      after,
		})
	})
	return updated, err
}

// ArchiveLocation archives a location (NFK-09).
func (s *Service) ArchiveLocation(ctx context.Context, id uuid.UUID, version int) (Location, error) {
	var updated Location
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		loc, err := st.GetLocation(ctx, id)
		if err != nil {
			return ProblemError(err)
		}
		updated, err = st.ArchiveLocation(ctx, id, version)
		if err != nil {
			return ProblemError(err)
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "inventory",
			EntityType: "location",
			EntityID:   id,
			Action:     "Archive",
			Before:     map[string]any{"name": loc.Name},
		})
	})
	return updated, err
}

// ─── Stock Movements ──────────────────────────────────────────────────────────

// ListStockMovements returns stock movements (LAG-01).
func (s *Service) ListStockMovements(ctx context.Context) ([]StockMovement, error) {
	return s.store.ListStockMovements(ctx)
}

// CreateStockMovement posts a manual movement (LAG-01, LAG-04). The caller sends
// a magnitude; the sign comes from the type (SignedQuantity). Reservations,
// releases and transfers have their own entry points (Reserve, Release, Transfer)
// because they must keep their invariants. A movement that takes stock out is
// refused with a 409 when it would take the available or batch quantity below zero.
func (s *Service) CreateStockMovement(ctx context.Context, in MovementInput) (StockMovement, error) {
	var movement StockMovement
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		var err error
		movement, err = s.PostMovement(ctx, tx, in)
		return err
	})
	return movement, err
}

// PostMovement is CreateStockMovement inside the caller's transaction.
func (s *Service) PostMovement(ctx context.Context, tx *db.Tx, in MovementInput) (StockMovement, error) {
	switch in.Type {
	case TypeReservation, TypeRelease:
		return StockMovement{}, s.refuse("use_reserve", invalid("%s is posted through reserve and release, not as a manual movement", in.Type))
	case TypeTransferIn, TypeTransferOut:
		return StockMovement{}, s.refuse("use_transfer", invalid("%s is posted as a pair through transfer, not as a manual movement", in.Type))
	}
	signed, err := SignedQuantity(in.Type, in.Quantity)
	if err != nil {
		return StockMovement{}, s.refuse("invalid", err)
	}
	if (in.Type == TypeAdjustmentIn || in.Type == TypeAdjustmentOut) && strings.TrimSpace(in.Reason) == "" {
		return StockMovement{}, s.refuse("no_reason", invalid("a manual adjustment needs a reason (LAG-04)"))
	}

	q := tx.Queries()
	if err := q.LockProductStock(ctx, in.ProductID); err != nil {
		return StockMovement{}, fmt.Errorf("lock product stock: %w", err)
	}
	if signed < 0 {
		if err := s.checkWithdrawal(ctx, q, in.ProductID, in.LocationID, in.BatchNumber, in.Quantity); err != nil {
			return StockMovement{}, s.refuse("insufficient", err)
		}
	}
	return s.append(ctx, tx, "Create", in, signed)
}

// checkWithdrawal refuses when taking qty out of a batch would take its
// quantity, or the location's available quantity, below zero.
func (s *Service) checkWithdrawal(ctx context.Context, q *dbgen.Queries, productID, locationID uuid.UUID, batch string, qty int) error {
	bal, err := q.LocationBalance(ctx, dbgen.LocationBalanceParams{ProductID: productID, LocationID: locationID})
	if err != nil {
		return fmt.Errorf("location balance: %w", err)
	}
	if avail := bal.InStock - bal.Reserved; avail < int64(qty) {
		return insufficient("need %d, only %d available", qty, avail)
	}
	batches, err := q.BatchStock(ctx, dbgen.BatchStockParams{ProductID: productID, LocationID: locationID})
	if err != nil {
		return fmt.Errorf("batch stock: %w", err)
	}
	for _, b := range batches {
		if b.BatchNumber == batch {
			if b.InStock < int64(qty) {
				return insufficient("batch %q has %d, need %d", batch, b.InStock, qty)
			}
			return nil
		}
	}
	return insufficient("batch %q has no stock", batch)
}

// append writes one ledger row and its audit entry. signed already has the sign.
func (s *Service) append(ctx context.Context, tx *db.Tx, action string, in MovementInput, signed int) (StockMovement, error) {
	var refID *uuid.UUID
	if in.ReferenceID != nil {
		parsed, err := uuid.Parse(*in.ReferenceID)
		if err != nil {
			return StockMovement{}, invalid("reference_id is not a UUID")
		}
		refID = &parsed
	}
	mDate, err := parseDate(in.ManufacturingDate)
	if err != nil {
		return StockMovement{}, err
	}
	bDate, err := parseDate(in.BestBeforeDate)
	if err != nil {
		return StockMovement{}, err
	}
	row, err := tx.Queries().CreateStockMovement(ctx, dbgen.CreateStockMovementParams{
		ID:                uuid.New(),
		ProductID:         in.ProductID,
		LocationID:        in.LocationID,
		BatchNumber:       in.BatchNumber,
		ManufacturingDate: mDate,
		BestBeforeDate:    bDate,
		MovementType:      in.Type,
		Quantity:          int32(signed), //nolint:gosec // bounded by business logic
		Reason:            in.Reason,
		ReferenceType:     in.ReferenceType,
		ReferenceID:       refID,
	})
	if err != nil {
		return StockMovement{}, fmt.Errorf("insert stock movement: %w", err)
	}
	movement := dbRowToStockMovement(row)
	if err := s.rec.Record(ctx, tx, audit.Entry{
		Module:     "inventory",
		EntityType: "stockmovement",
		EntityID:   movement.ID,
		Action:     action,
		After: map[string]any{
			"product_id":    in.ProductID.String(),
			"location_id":   in.LocationID.String(),
			"batch_number":  in.BatchNumber,
			"movement_type": in.Type,
			"quantity":      signed,
			"reason":        in.Reason,
			"reference":     in.ReferenceType,
		},
	}); err != nil {
		return StockMovement{}, err
	}
	if s.movements != nil {
		s.movements.WithLabelValues(in.Type).Inc()
	}
	return movement, nil
}

func parseDate(d *string) (pgtype.Date, error) {
	if d == nil || *d == "" {
		return pgtype.Date{}, nil
	}
	t, err := time.Parse("2006-01-02", *d)
	if err != nil {
		return pgtype.Date{}, invalid("date %q is not YYYY-MM-DD", *d)
	}
	return pgtype.Date{Time: t, Valid: true}, nil
}

// Transfer moves stock between locations as a transfer_out plus a transfer_in
// in one transaction (LAG-01).
func (s *Service) Transfer(ctx context.Context, in TransferInput) error {
	return db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		if in.FromLocationID == in.ToLocationID {
			return s.refuse("invalid", invalid("a transfer needs two different locations"))
		}
		if _, err := SignedQuantity(TypeTransferOut, in.Quantity); err != nil {
			return s.refuse("invalid", err)
		}
		q := tx.Queries()
		if err := q.LockProductStock(ctx, in.ProductID); err != nil {
			return fmt.Errorf("lock product stock: %w", err)
		}
		if err := s.checkWithdrawal(ctx, q, in.ProductID, in.FromLocationID, in.BatchNumber, in.Quantity); err != nil {
			return s.refuse("insufficient", err)
		}
		// The batch keeps its dates at the new location.
		batches, err := q.BatchStock(ctx, dbgen.BatchStockParams{ProductID: in.ProductID, LocationID: in.FromLocationID})
		if err != nil {
			return fmt.Errorf("batch stock: %w", err)
		}
		var mDate, bDate *string
		for _, b := range batches {
			if b.BatchNumber == in.BatchNumber {
				mDate, bDate = datePtr(b.ManufacturingDate), datePtr(b.BestBeforeDate)
			}
		}
		reason := in.Reason
		if reason == "" {
			reason = "transfer"
		}
		out := MovementInput{
			ProductID: in.ProductID, LocationID: in.FromLocationID, BatchNumber: in.BatchNumber,
			Type: TypeTransferOut, Quantity: in.Quantity, Reason: reason,
		}
		if _, err := s.append(ctx, tx, "Transfer", out, -in.Quantity); err != nil {
			return err
		}
		inn := MovementInput{
			ProductID: in.ProductID, LocationID: in.ToLocationID, BatchNumber: in.BatchNumber,
			ManufacturingDate: mDate, BestBeforeDate: bDate,
			Type: TypeTransferIn, Quantity: in.Quantity, Reason: reason,
		}
		_, err = s.append(ctx, tx, "Transfer", inn, in.Quantity)
		return err
	})
}

// ─── Balances ─────────────────────────────────────────────────────────────────

// GetBalance returns in stock, reserved and available per location for a
// product (LAG-01, LAG-02), plus the marketplace quantity (LAG-07).
func (s *Service) GetBalance(ctx context.Context, productID uuid.UUID) ([]Balance, error) {
	rows, err := s.store.BalanceByLocation(ctx, productID)
	if err != nil {
		return nil, err
	}
	out := make([]Balance, 0, len(rows))
	totalAvailable := 0
	for _, r := range rows {
		b := Balance{
			ProductID:  productID,
			LocationID: r.LocationID,
			InStock:    int(r.InStock),
			Reserved:   int(r.Reserved),
			Available:  int(r.InStock - r.Reserved),
		}
		totalAvailable += b.Available
		out = append(out, b)
	}
	if len(out) == 0 {
		return out, nil
	}
	buffer, err := s.store.ProductChannelBuffer(ctx, productID)
	if err != nil {
		return nil, err
	}
	marketplace := marketplaceQuantity(totalAvailable, buffer)
	for i := range out {
		out[i].MarketplaceQuantity = marketplace
	}
	return out, nil
}

// MarketplaceQuantity is what marketplaces may sell: the product's available
// quantity over all locations minus its channel buffer, never below zero (LAG-07).
func (s *Service) MarketplaceQuantity(ctx context.Context, productID uuid.UUID) (int, error) {
	rows, err := s.GetBalance(ctx, productID)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	return rows[0].MarketplaceQuantity, nil
}

func marketplaceQuantity(available, buffer int) int {
	return max(0, available-buffer)
}

// GetFEFOBatches returns batches ordered by FEFO (first-expiry-first-out) (LAG-03).
func (s *Service) GetFEFOBatches(ctx context.Context, productID, locationID uuid.UUID) ([]FEFOBatch, error) {
	rows, err := s.store.BatchStock(ctx, productID, locationID)
	if err != nil {
		return nil, err
	}
	out := make([]FEFOBatch, 0, len(rows))
	for _, r := range rows {
		out = append(out, FEFOBatch{
			ProductID:         productID,
			LocationID:        locationID,
			BatchNumber:       r.BatchNumber,
			ManufacturingDate: datePtr(r.ManufacturingDate),
			BestBeforeDate:    datePtr(r.BestBeforeDate),
			InStock:           int(r.InStock),
		})
	}
	return out, nil
}

// GetInventoryValue returns the inventory value per product per location, in
// öre: quantity in stock x manufacturing cost (LAG-05).
func (s *Service) GetInventoryValue(ctx context.Context) ([]InventoryValue, error) {
	rows, err := s.store.InventoryValueByLocation(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]InventoryValue, 0, len(rows))
	for _, r := range rows {
		out = append(out, InventoryValue{
			LocationID:  r.LocationID,
			ProductID:   r.ProductID,
			ProductName: r.ProductName,
			InStock:     int(r.InStock),
			Value:       r.Value,
		})
	}
	return out, nil
}

// ─── Reserve, release, ship (called by the order workflow in its transaction) ─

// Reserve reserves stock for a reference, normally an order line, when an order
// is accepted (LAG-02). It locks the product and refuses with a 409 when the
// available quantity is too small.
func (s *Service) Reserve(ctx context.Context, tx *db.Tx, in StockRef) error {
	if _, err := SignedQuantity(TypeReservation, in.Quantity); err != nil {
		return s.refuse("invalid", err)
	}
	q := tx.Queries()
	if err := q.LockProductStock(ctx, in.ProductID); err != nil {
		return fmt.Errorf("lock product stock: %w", err)
	}
	bal, err := q.LocationBalance(ctx, dbgen.LocationBalanceParams{ProductID: in.ProductID, LocationID: in.LocationID})
	if err != nil {
		return fmt.Errorf("location balance: %w", err)
	}
	if avail := bal.InStock - bal.Reserved; avail < int64(in.Quantity) {
		return s.refuse("insufficient", insufficient("need %d, only %d available", in.Quantity, avail))
	}
	_, err = s.append(ctx, tx, "Reserve", refMovement(in, TypeReservation, "order reservation"), -in.Quantity)
	return err
}

// Release gives a reservation back, for example when an order is cancelled (LAG-02).
func (s *Service) Release(ctx context.Context, tx *db.Tx, in StockRef) error {
	if _, err := SignedQuantity(TypeRelease, in.Quantity); err != nil {
		return s.refuse("invalid", err)
	}
	q := tx.Queries()
	if err := q.LockProductStock(ctx, in.ProductID); err != nil {
		return fmt.Errorf("lock product stock: %w", err)
	}
	reserved, err := s.reservedFor(ctx, q, in)
	if err != nil {
		return err
	}
	if reserved < int64(in.Quantity) {
		return s.refuse("insufficient", insufficient("only %d reserved for this reference, cannot release %d", reserved, in.Quantity))
	}
	_, err = s.append(ctx, tx, "Release", refMovement(in, TypeRelease, "reservation released"), in.Quantity)
	return err
}

func (s *Service) reservedFor(ctx context.Context, q *dbgen.Queries, in StockRef) (int64, error) {
	reserved, err := q.ReservedForReference(ctx, dbgen.ReservedForReferenceParams{
		ProductID: in.ProductID, LocationID: in.LocationID,
		ReferenceType: in.ReferenceType, ReferenceID: &in.ReferenceID,
	})
	if err != nil {
		return 0, fmt.Errorf("reserved for reference: %w", err)
	}
	return reserved, nil
}

func refMovement(in StockRef, typ, reason string) MovementInput {
	refID := in.ReferenceID.String()
	return MovementInput{
		ProductID: in.ProductID, LocationID: in.LocationID, Type: typ, Quantity: in.Quantity,
		Reason: reason, ReferenceType: in.ReferenceType, ReferenceID: &refID,
	}
}

// Ship takes the shipped quantity out of stock (LAG-02, LAG-03). The reservation
// for the reference is converted: one release for what was reserved plus one
// sale per batch, in the caller's transaction. In stock and reserved both go down
// by the quantity and available does not change. The batch is the earliest
// best-before one (FEFO) unless in.BatchNumber chooses one. The batches are
// recorded on the order line for recall traceability.
func (s *Service) Ship(ctx context.Context, tx *db.Tx, in ShipInput) ([]ShippedBatch, error) {
	if _, err := SignedQuantity(TypeSale, in.Quantity); err != nil {
		return nil, s.refuse("invalid", err)
	}
	q := tx.Queries()
	if err := q.LockProductStock(ctx, in.ProductID); err != nil {
		return nil, fmt.Errorf("lock product stock: %w", err)
	}

	reservedForLine, err := s.reservedFor(ctx, q, in.StockRef)
	if err != nil {
		return nil, err
	}
	releaseQty := min(reservedForLine, int64(in.Quantity))
	if extra := int64(in.Quantity) - releaseQty; extra > 0 {
		// Not reserved for this line: it must be available to anybody else.
		bal, err := q.LocationBalance(ctx, dbgen.LocationBalanceParams{ProductID: in.ProductID, LocationID: in.LocationID})
		if err != nil {
			return nil, fmt.Errorf("location balance: %w", err)
		}
		if avail := bal.InStock - bal.Reserved; avail < extra {
			return nil, s.refuse("insufficient", insufficient("need %d more than reserved, only %d available", extra, avail))
		}
	}

	picks, err := s.pickBatches(ctx, q, in)
	if err != nil {
		return nil, s.refuse("insufficient", err)
	}

	if releaseQty > 0 {
		rel := in.StockRef
		rel.Quantity = int(releaseQty)
		if _, err := s.append(ctx, tx, "Release", refMovement(rel, TypeRelease, "reservation converted by shipment"), rel.Quantity); err != nil {
			return nil, err
		}
	}
	for _, pick := range picks {
		sale := refMovement(StockRef{
			ProductID: in.ProductID, LocationID: in.LocationID, Quantity: pick.Quantity,
			ReferenceType: in.ReferenceType, ReferenceID: in.ReferenceID,
		}, TypeSale, "order shipment")
		sale.BatchNumber = pick.BatchNumber
		m, err := s.append(ctx, tx, "Ship", sale, -pick.Quantity)
		if err != nil {
			return nil, err
		}
		if in.OrderLineID != uuid.Nil {
			if err := q.InsertOrderLineBatch(ctx, dbgen.InsertOrderLineBatchParams{
				OrderLineID: in.OrderLineID, BatchNumber: pick.BatchNumber,
				Quantity: int32(pick.Quantity), MovementID: m.ID, //nolint:gosec // bounded by business logic
			}); err != nil {
				return nil, fmt.Errorf("record shipped batch: %w", err)
			}
		}
	}
	if in.OrderLineID != uuid.Nil {
		if err := q.AddOrderLineShipped(ctx, dbgen.AddOrderLineShippedParams{
			ID: in.OrderLineID, QuantityShipped: int32(in.Quantity), //nolint:gosec // bounded by business logic
		}); err != nil {
			return nil, fmt.Errorf("update quantity shipped: %w", err)
		}
	}
	return picks, nil
}

// pickBatches chooses the batches to take quantity from: the manual batch, or
// FEFO (earliest best-before first).
func (s *Service) pickBatches(ctx context.Context, q *dbgen.Queries, in ShipInput) ([]ShippedBatch, error) {
	batches, err := q.BatchStock(ctx, dbgen.BatchStockParams{ProductID: in.ProductID, LocationID: in.LocationID})
	if err != nil {
		return nil, fmt.Errorf("batch stock: %w", err)
	}
	need := int64(in.Quantity)
	var picks []ShippedBatch
	for _, b := range batches {
		if in.BatchNumber != "" {
			if b.BatchNumber != in.BatchNumber {
				continue
			}
			if b.InStock < need {
				return nil, insufficient("batch %q has %d, need %d", b.BatchNumber, b.InStock, need)
			}
			return []ShippedBatch{{BatchNumber: b.BatchNumber, Quantity: in.Quantity}}, nil
		}
		take := min(b.InStock, need)
		picks = append(picks, ShippedBatch{BatchNumber: b.BatchNumber, Quantity: int(take)})
		if need -= take; need == 0 {
			return picks, nil
		}
	}
	if in.BatchNumber != "" {
		return nil, insufficient("batch %q has no stock", in.BatchNumber)
	}
	return nil, insufficient("need %d, batches hold %d less", in.Quantity, need)
}

// ReserveStock, ReleaseStock and WithdrawStock run the same logic in a
// transaction of their own, for the HTTP endpoints.
func (s *Service) ReserveStock(ctx context.Context, in StockRef) error {
	return db.InTx(ctx, s.pool, func(tx *db.Tx) error { return s.Reserve(ctx, tx, in) })
}

func (s *Service) ReleaseStock(ctx context.Context, in StockRef) error {
	return db.InTx(ctx, s.pool, func(tx *db.Tx) error { return s.Release(ctx, tx, in) })
}

func (s *Service) WithdrawStock(ctx context.Context, in ShipInput) ([]ShippedBatch, error) {
	var out []ShippedBatch
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) (err error) {
		out, err = s.Ship(ctx, tx, in)
		return err
	})
	return out, err
}

// RegisterMetrics registers linochi_inventory_movements_total and
// linochi_inventory_refusals_total. Without it the service runs without them.
func (s *Service) RegisterMetrics(reg prometheus.Registerer) error {
	s.movements = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "linochi_inventory_movements_total",
		Help: "Stock ledger rows written, by movement type.",
	}, []string{"type"})
	s.refusals = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "linochi_inventory_refusals_total",
		Help: "Stock operations refused, by reason (insufficient, invalid, no_reason, ...).",
	}, []string{"reason"})
	for _, c := range []prometheus.Collector{s.movements, s.refusals} {
		if err := reg.Register(c); err != nil {
			return fmt.Errorf("register inventory metric: %w", err)
		}
	}
	return nil
}

// refuse counts a refused operation and returns err unchanged.
func (s *Service) refuse(reason string, err error) error {
	if s.refusals != nil {
		s.refusals.WithLabelValues(reason).Inc()
	}
	return err
}

func datePtr(d pgtype.Date) *string {
	if !d.Valid {
		return nil
	}
	s := d.Time.Format("2006-01-02")
	return &s
}
