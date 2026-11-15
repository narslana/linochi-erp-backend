package inventory

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/audit"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db/dbgen"
)

// Service holds the business logic for inventory (LAG-01 to LAG-05).
type Service struct {
	store *Store
	rec   *audit.Recorder
	pool  *pgxpool.Pool
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
func (s *Service) CreateLocation(ctx context.Context, name, nameEn, address, city, country string, channelBuffer int) (Location, error) {
	var loc Location
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		id := uuid.New()
		var err error
		loc, err = s.store.CreateLocation(ctx, id, name, nameEn, address, city, country, channelBuffer)
		if err != nil {
			return err
		}

		actor, _ := audit.ActorFrom(ctx)
		_ = s.rec.Record(ctx, tx, audit.Entry{
			Module:     "inventory",
			EntityType: "location",
			EntityID:   loc.ID,
			Action:     "Create",
			After:      map[string]any{"name": name, "city": city},
			Reason:     actor.ID.String(),
		})
		return nil
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
func (s *Service) UpdateLocation(ctx context.Context, id uuid.UUID, name, nameEn, address, city, country *string, channelBuffer *int, version int) (Location, error) {
	loc, err := s.store.UpdateLocation(ctx, name, nameEn, address, city, country, channelBuffer, id, version)
	if err != nil {
		return loc, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "inventory",
		EntityType: "location",
		EntityID:   id,
		Action:     "Update",
		After:      map[string]any{"name": name},
		Reason:     actor.ID.String(),
	})
	return loc, nil
}

// ArchiveLocation archives a location (NFK-09).
func (s *Service) ArchiveLocation(ctx context.Context, id uuid.UUID, version int) (Location, error) {
	loc, err := s.store.ArchiveLocation(ctx, id, version)
	if err != nil {
		return loc, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "inventory",
		EntityType: "location",
		EntityID:   id,
		Action:     "Archive",
		Before:     map[string]any{"name": loc.Name},
		Reason:     actor.ID.String(),
	})
	return loc, nil
}

// ─── Stock Movements ──────────────────────────────────────────────────────────

// ListStockMovements returns stock movements (LAG-01).
func (s *Service) ListStockMovements(ctx context.Context) ([]StockMovement, error) {
	return s.store.ListStockMovements(ctx)
}

// CreateStockMovement creates a stock movement (LAG-01, LAG-04).
// The quantity must be positive; the sign is implicit in the movement_type.
func (s *Service) CreateStockMovement(ctx context.Context, productID, locationID uuid.UUID, batchNumber string, manufacturingDate, bestBeforeDate *string, movementType string, quantity int, reason string, referenceType string, referenceID *string) (StockMovement, error) {
	validTypes := map[string]bool{
		"purchase": true, "sale": true, "adjustment_in": true,
		"adjustment_out": true, "transfer_in": true, "transfer_out": true,
		"reservation": true, "release": true,
	}
	if !validTypes[movementType] {
		return StockMovement{}, ErrInvalidMovement
	}

	// For adjustment_out, quantity is negative (stock decreases)
	if movementType == "adjustment_out" && quantity > 0 {
		quantity = -quantity
	}

	var movement StockMovement
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		id := uuid.New()
		var err error
		movement, err = s.store.CreateStockMovement(ctx, id, productID, locationID, batchNumber, manufacturingDate, bestBeforeDate, movementType, quantity, reason, referenceType, referenceID)
		if err != nil {
			return err
		}

		actor, _ := audit.ActorFrom(ctx)
		_ = s.rec.Record(ctx, tx, audit.Entry{
			Module:     "inventory",
			EntityType: "stockmovement",
			EntityID:   movement.ID,
			Action:     "Create",
			After: map[string]any{
				"product_id":    productID.String(),
				"location_id":   locationID.String(),
				"movement_type": movementType,
				"quantity":      quantity,
				"reason":        reason,
			},
			Reason: actor.ID.String(),
		})
		return nil
	})
	return movement, err
}

// GetBalance returns the balance for a product (LAG-01, LAG-02).
func (s *Service) GetBalance(ctx context.Context, productID uuid.UUID) ([]Balance, error) {
	rows, err := s.store.BalanceByLocation(ctx, productID)
	if err != nil {
		return nil, err
	}
	out := make([]Balance, 0, len(rows))
	for _, r := range rows {
		out = append(out, Balance{
			ProductID:  productID,
			LocationID: r.LocationID,
			InStock:    int(r.InStock.(int64)), //nolint:gosec // sqlc returns interface{} for SUM
		})
	}
	return out, nil
}

// GetFEFOBatches returns batches ordered by FEFO (first-expiry-first-out) (LAG-03).
func (s *Service) GetFEFOBatches(ctx context.Context, productID, locationID uuid.UUID) ([]FEFOBatch, error) {
	rows, err := s.store.MovementsForFEFO(ctx, productID, locationID)
	if err != nil {
		return nil, err
	}
	out := make([]FEFOBatch, 0, len(rows))
	for _, r := range rows {
		out = append(out, FEFOBatch{
			ProductID:         r.ProductID,
			LocationID:        r.LocationID,
			BatchNumber:       r.BatchNumber,
			ManufacturingDate: datePtr(r.ManufacturingDate),
			BestBeforeDate:    datePtr(r.BestBeforeDate),
			InStock:           int(r.InStock),
		})
	}
	return out, nil
}

// GetInventoryValue returns the inventory value per product per location (LAG-05).
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
			InStock:     int(r.InStock.(int64)), //nolint:gosec // sqlc returns interface{} for SUM
			Value:       r.Value.(int64),        //nolint:gosec // sqlc returns interface{} for SUM
		})
	}
	return out, nil
}

// ReserveStock reserves stock for an order (LAG-02).
func (s *Service) ReserveStock(ctx context.Context, productID uuid.UUID, locationID uuid.UUID, quantity int, referenceType string, referenceID uuid.UUID) error {
	return db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		movementID := uuid.New()
		actor, _ := audit.ActorFrom(ctx)
		refID := referenceID.String()

		if _, err := tx.Queries().CreateStockMovement(ctx, dbgen.CreateStockMovementParams{
			ID:            movementID,
			ProductID:     productID,
			LocationID:    locationID,
			MovementType:  "reservation",
			Quantity:      int32(-quantity), //nolint:gosec // negative reservation
			Reason:        "order reservation",
			ReferenceType: referenceType,
			ReferenceID:   &referenceID,
		}); err != nil {
			return err
		}

		_ = s.rec.Record(ctx, tx, audit.Entry{
			Module:     "inventory",
			EntityType: "reservation",
			EntityID:   movementID,
			Action:     "Reserve",
			After: map[string]any{
				"product_id": productID.String(),
				"quantity":   quantity,
				"reference":  refID,
			},
			Reason: actor.ID.String(),
		})
		return nil
	})
}

// ReleaseStock releases a reservation (LAG-02).
func (s *Service) ReleaseStock(ctx context.Context, productID uuid.UUID, locationID uuid.UUID, quantity int, referenceType string, referenceID uuid.UUID) error {
	return db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		movementID := uuid.New()
		actor, _ := audit.ActorFrom(ctx)
		refID := referenceID.String()

		if _, err := tx.Queries().CreateStockMovement(ctx, dbgen.CreateStockMovementParams{
			ID:            movementID,
			ProductID:     productID,
			LocationID:    locationID,
			MovementType:  "release",
			Quantity:      int32(quantity), //nolint:gosec // positive release
			Reason:        "reservation released",
			ReferenceType: referenceType,
			ReferenceID:   &referenceID,
		}); err != nil {
			return err
		}

		_ = s.rec.Record(ctx, tx, audit.Entry{
			Module:     "inventory",
			EntityType: "release",
			EntityID:   movementID,
			Action:     "Release",
			After: map[string]any{
				"product_id": productID.String(),
				"quantity":   quantity,
				"reference":  refID,
			},
			Reason: actor.ID.String(),
		})
		return nil
	})
}

// WithdrawStock withdraws stock when shipped (LAG-02, LAG-03).
func (s *Service) WithdrawStock(ctx context.Context, productID uuid.UUID, locationID uuid.UUID, quantity int, referenceType string, referenceID uuid.UUID) error {
	return db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		batches, err := tx.Queries().MovementsForFEFO(ctx, dbgen.MovementsForFEFOParams{ProductID: productID, LocationID: locationID})
		if err != nil {
			return err
		}

		remaining := int64(quantity)
		for _, batch := range batches {
			if remaining <= 0 {
				break
			}
			take := batch.InStock
			if take > remaining {
				take = remaining
			}

			movementID := uuid.New()
			actor, _ := audit.ActorFrom(ctx)
			refID := referenceID.String()

			if _, err := tx.Queries().CreateStockMovement(ctx, dbgen.CreateStockMovementParams{
				ID:            movementID,
				ProductID:     productID,
				LocationID:    locationID,
				BatchNumber:   batch.BatchNumber,
				MovementType:  "sale",
				Quantity:      int32(-take), //nolint:gosec // negative sale
				Reason:        "order shipment",
				ReferenceType: referenceType,
				ReferenceID:   &referenceID,
			}); err != nil {
				return err
			}

			_ = s.rec.Record(ctx, tx, audit.Entry{
				Module:     "inventory",
				EntityType: "sale",
				EntityID:   movementID,
				Action:     "Withdraw",
				After: map[string]any{
					"product_id": productID.String(),
					"quantity":   take,
					"batch":      batch.BatchNumber,
					"reference":  refID,
				},
				Reason: actor.ID.String(),
			})

			remaining -= take
		}

		if remaining > 0 {
			return fmt.Errorf("%w: need %d, only have %d available", ErrNegativeBalance, quantity, int(int64(quantity)-remaining))
		}
		return nil
	})
}

func datePtr(d pgtype.Date) *string {
	if !d.Valid {
		return nil
	}
	s := d.Time.Format("2006-01-02")
	return &s
}
