package shipments

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db/dbgen"
)

// Store wraps sqlc queries for the shipments module.
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

// ─── Shipments ────────────────────────────────────────────────────────────────

func (s *Store) ListShipments(ctx context.Context) ([]Shipment, error) {
	rows, err := s.q.ListShipments(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Shipment, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToShipment(r))
	}
	return out, nil
}

func (s *Store) ListShipmentsByPurchaseOrder(ctx context.Context, poID uuid.UUID) ([]Shipment, error) {
	rows, err := s.q.ListShipmentsByPurchaseOrder(ctx, poID)
	if err != nil {
		return nil, err
	}
	out := make([]Shipment, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToShipment(r))
	}
	return out, nil
}

func (s *Store) ListShipmentsByStatus(ctx context.Context, status string) ([]Shipment, error) {
	rows, err := s.q.ListShipmentsByStatus(ctx, status)
	if err != nil {
		return nil, err
	}
	out := make([]Shipment, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToShipment(r))
	}
	return out, nil
}

func (s *Store) GetShipment(ctx context.Context, id uuid.UUID) (Shipment, error) {
	row, err := s.q.GetShipment(ctx, id)
	if err != nil {
		return Shipment{}, err
	}
	return dbRowToShipment(row), nil
}

func (s *Store) CreateShipment(ctx context.Context, id, poID uuid.UUID, fromLocation, toLocation, carrier, trackingNumber, batchNumber, status string, shippedAt, estimatedArrival, actualArrival *time.Time, documents json.RawMessage) (Shipment, error) {
	var estArrivalPg, actualArrivalPg pgtype.Date
	if estimatedArrival != nil {
		estArrivalPg = pgtype.Date{Time: *estimatedArrival, Valid: true}
	}
	if actualArrival != nil {
		actualArrivalPg = pgtype.Date{Time: *actualArrival, Valid: true}
	}
	if documents == nil {
		documents = json.RawMessage("[]")
	}
	row, err := s.q.CreateShipment(ctx, dbgen.CreateShipmentParams{
		ID:               id,
		PurchaseOrderID:  poID,
		FromLocation:     fromLocation,
		ToLocation:       toLocation,
		Carrier:          carrier,
		TrackingNumber:   trackingNumber,
		ShippedAt:        shippedAt,
		EstimatedArrival: estArrivalPg,
		ActualArrival:    actualArrivalPg,
		BatchNumber:      batchNumber,
		Status:           status,
		Documents:        documents,
	})
	if err != nil {
		return Shipment{}, err
	}
	return dbRowToShipment(row), nil
}

func (s *Store) UpdateShipment(ctx context.Context, fromLocation, toLocation, carrier, trackingNumber, batchNumber, status *string, shippedAt, estimatedArrival, actualArrival *time.Time, documents *json.RawMessage, id uuid.UUID, version int) (Shipment, error) {
	var params dbgen.UpdateShipmentParams
	params.ID = id
	params.Version = int32(version) //nolint:gosec // version is bounded by optimistic locking
	if fromLocation != nil {
		params.FromLocation = *fromLocation
	}
	if toLocation != nil {
		params.ToLocation = *toLocation
	}
	if carrier != nil {
		params.Carrier = *carrier
	}
	if trackingNumber != nil {
		params.TrackingNumber = *trackingNumber
	}
	if batchNumber != nil {
		params.BatchNumber = *batchNumber
	}
	if status != nil {
		params.Status = *status
	}
	if documents != nil {
		params.Documents = *documents
	}
	if shippedAt != nil {
		params.ShippedAt = shippedAt
	}
	if estimatedArrival != nil {
		params.EstimatedArrival = pgtype.Date{Time: *estimatedArrival, Valid: true}
	}
	if actualArrival != nil {
		params.ActualArrival = pgtype.Date{Time: *actualArrival, Valid: true}
	}
	row, err := s.q.UpdateShipment(ctx, params)
	if err != nil {
		return Shipment{}, err
	}
	return dbRowToShipment(row), nil
}

func (s *Store) ArchiveShipment(ctx context.Context, id uuid.UUID, version int) (Shipment, error) {
	row, err := s.q.ArchiveShipment(ctx, dbgen.ArchiveShipmentParams{ID: id, Version: int32(version)}) //nolint:gosec // version is bounded by optimistic locking
	if err != nil {
		return Shipment{}, err
	}
	return dbRowToShipment(row), nil
}

// ─── Row helpers ──────────────────────────────────────────────────────────────

func dbRowToShipment(r dbgen.Shipment) Shipment {
	s := Shipment{
		ID:              r.ID,
		PurchaseOrderID: r.PurchaseOrderID,
		FromLocation:    r.FromLocation,
		ToLocation:      r.ToLocation,
		Carrier:         r.Carrier,
		TrackingNumber:  r.TrackingNumber,
		BatchNumber:     r.BatchNumber,
		Status:          r.Status,
		Documents:       r.Documents,
		Version:         int(r.Version),
		CreatedAt:       r.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       r.UpdatedAt.Format(time.RFC3339),
	}
	if r.ShippedAt != nil {
		t := r.ShippedAt.Format(time.RFC3339)
		s.ShippedAt = &t
	}
	if r.EstimatedArrival.Valid {
		t := r.EstimatedArrival.Time.Format("2006-01-02")
		s.EstimatedArrival = &t
	}
	if r.ActualArrival.Valid {
		t := r.ActualArrival.Time.Format("2006-01-02")
		s.ActualArrival = &t
	}
	if r.ArchivedAt != nil {
		s.Archived = true
		t := r.ArchivedAt.Format(time.RFC3339)
		s.ArchivedAt = &t
	}
	return s
}
