package inventory

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db/dbgen"
)

// Store wraps sqlc queries for the inventory module.
type Store struct {
	q *dbgen.Queries
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{q: dbgen.New(pool)}
}

// WithTx returns a store that runs all queries in the given transaction.
func (s *Store) WithTx(tx *db.Tx) *Store {
	return &Store{q: tx.Queries()}
}

// ─── Locations ────────────────────────────────────────────────────────────────

func (s *Store) ListLocations(ctx context.Context) ([]Location, error) {
	rows, err := s.q.ListLocations(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Location, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToLocation(r))
	}
	return out, nil
}

func (s *Store) GetLocation(ctx context.Context, id uuid.UUID) (Location, error) {
	row, err := s.q.GetLocation(ctx, id)
	if err != nil {
		return Location{}, err
	}
	return dbRowToLocation(row), nil
}

func (s *Store) CreateLocation(ctx context.Context, id uuid.UUID, name, nameEn, address, city, country string) (Location, error) {
	row, err := s.q.CreateLocation(ctx, dbgen.CreateLocationParams{
		ID:      id,
		Name:    name,
		NameEn:  nameEn,
		Address: address,
		City:    city,
		Country: country,
	})
	if err != nil {
		return Location{}, err
	}
	return dbRowToLocation(row), nil
}

func (s *Store) UpdateLocation(ctx context.Context, name, nameEn, address, city, country *string, id uuid.UUID, version int) (Location, error) {
	var params dbgen.UpdateLocationParams
	params.ID = id
	params.Version = int32(version) //nolint:gosec // version is bounded by optimistic locking
	if name != nil {
		params.Name = *name
	}
	if nameEn != nil {
		params.NameEn = *nameEn
	}
	if address != nil {
		params.Address = *address
	}
	if city != nil {
		params.City = *city
	}
	if country != nil {
		params.Country = *country
	}
	row, err := s.q.UpdateLocation(ctx, params)
	if err != nil {
		return Location{}, err
	}
	return dbRowToLocation(row), nil
}

func (s *Store) ArchiveLocation(ctx context.Context, id uuid.UUID, version int) (Location, error) {
	row, err := s.q.ArchiveLocation(ctx, dbgen.ArchiveLocationParams{ID: id, Version: int32(version)}) //nolint:gosec // version is bounded by optimistic locking
	if err != nil {
		return Location{}, err
	}
	return dbRowToLocation(row), nil
}

// ─── Stock Movements ──────────────────────────────────────────────────────────

func (s *Store) ListStockMovements(ctx context.Context) ([]StockMovement, error) {
	rows, err := s.q.ListStockMovements(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]StockMovement, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToStockMovement(r))
	}
	return out, nil
}

func (s *Store) CreateStockMovement(ctx context.Context, id, productID, locationID uuid.UUID, batchNumber string, manufacturingDate, bestBeforeDate *string, movementType string, quantity int, reason string, referenceType string, referenceID *string) (StockMovement, error) {
	var mDate, bDate pgtype.Date
	if manufacturingDate != nil {
		t, _ := time.Parse("2006-01-02", *manufacturingDate)
		mDate = pgtype.Date{Time: t, Valid: true}
	}
	if bestBeforeDate != nil {
		t, _ := time.Parse("2006-01-02", *bestBeforeDate)
		bDate = pgtype.Date{Time: t, Valid: true}
	}

	var refID *uuid.UUID
	if referenceID != nil {
		parsed := uuid.MustParse(*referenceID)
		refID = &parsed
	}

	row, err := s.q.CreateStockMovement(ctx, dbgen.CreateStockMovementParams{
		ID:                id,
		ProductID:         productID,
		LocationID:        locationID,
		BatchNumber:       batchNumber,
		ManufacturingDate: mDate,
		BestBeforeDate:    bDate,
		MovementType:      movementType,
		Quantity:          int32(quantity), //nolint:gosec // quantity is bounded by business logic
		Reason:            reason,
		ReferenceType:     referenceType,
		ReferenceID:       refID,
	})
	if err != nil {
		return StockMovement{}, err
	}
	return dbRowToStockMovement(row), nil
}

func (s *Store) BalanceByLocation(ctx context.Context, productID uuid.UUID) ([]dbgen.BalanceByLocationRow, error) {
	return s.q.BalanceByLocation(ctx, productID)
}

func (s *Store) BatchStock(ctx context.Context, productID, locationID uuid.UUID) ([]dbgen.BatchStockRow, error) {
	return s.q.BatchStock(ctx, dbgen.BatchStockParams{ProductID: productID, LocationID: locationID})
}

func (s *Store) InventoryValueByLocation(ctx context.Context) ([]dbgen.InventoryValueByLocationRow, error) {
	return s.q.InventoryValueByLocation(ctx)
}

func (s *Store) ProductChannelBuffer(ctx context.Context, productID uuid.UUID) (int, error) {
	n, err := s.q.ProductChannelBuffer(ctx, productID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, newNotFound("product")
		}
		return 0, err
	}
	return int(n), nil
}

// ─── Row helpers ──────────────────────────────────────────────────────────────

func dbRowToLocation(r dbgen.Location) Location {
	l := Location{
		ID:        r.ID,
		Name:      r.Name,
		NameEn:    r.NameEn,
		Address:   r.Address,
		City:      r.City,
		Country:   r.Country,
		Version:   int(r.Version),
		CreatedAt: r.CreatedAt.Format(time.RFC3339),
		UpdatedAt: r.UpdatedAt.Format(time.RFC3339),
	}
	if r.ArchivedAt != nil {
		l.Archived = true
		t := r.ArchivedAt.Format(time.RFC3339)
		l.ArchivedAt = &t
	}
	return l
}

func dbRowToStockMovement(r dbgen.Stockmovement) StockMovement {
	m := StockMovement{
		ID:            r.ID,
		ProductID:     r.ProductID,
		LocationID:    r.LocationID,
		BatchNumber:   r.BatchNumber,
		MovementType:  r.MovementType,
		Quantity:      int(r.Quantity),
		Reason:        r.Reason,
		ReferenceType: r.ReferenceType,
		Version:       int(r.Version),
		CreatedAt:     r.CreatedAt.Format(time.RFC3339),
		UpdatedAt:     r.UpdatedAt.Format(time.RFC3339),
	}
	m.ManufacturingDate = dateToString(r.ManufacturingDate)
	m.BestBeforeDate = dateToString(r.BestBeforeDate)
	if r.ReferenceID != nil {
		s := r.ReferenceID.String()
		m.ReferenceID = &s
	}
	return m
}

func dateToString(d pgtype.Date) *string {
	if !d.Valid {
		return nil
	}
	s := d.Time.Format("2006-01-02")
	return &s
}
