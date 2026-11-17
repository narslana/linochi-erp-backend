package shipments

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/audit"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
)

// Service holds the business logic for shipments.
type Service struct {
	store *Store
	rec   *audit.Recorder
	pool  *pgxpool.Pool
}

func NewService(store *Store, rec *audit.Recorder, pool *pgxpool.Pool) *Service {
	return &Service{store: store, rec: rec, pool: pool}
}

// ListShipments returns all active shipments (INK-03).
func (s *Service) ListShipments(ctx context.Context, status *string, poID *uuid.UUID) ([]Shipment, error) {
	switch {
	case status != nil:
		return s.store.ListShipmentsByStatus(ctx, *status)
	case poID != nil:
		return s.store.ListShipmentsByPurchaseOrder(ctx, *poID)
	default:
		return s.store.ListShipments(ctx)
	}
}

// GetShipment returns a shipment by ID.
func (s *Service) GetShipment(ctx context.Context, id uuid.UUID) (Shipment, error) {
	sh, err := s.store.GetShipment(ctx, id)
	if err != nil {
		return sh, ProblemError(err)
	}
	return sh, nil
}

// CreateShipment creates a new shipment (INK-02).
func (s *Service) CreateShipment(ctx context.Context, poID uuid.UUID, fromLocation, toLocation, carrier, trackingNumber, batchNumber, status string, shippedAt, estimatedArrival, actualArrival *time.Time, documents json.RawMessage) (Shipment, error) {
	var sh Shipment
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		id := uuid.New()
		var err error
		sh, err = st.CreateShipment(ctx, id, poID, fromLocation, toLocation, carrier, trackingNumber, batchNumber, status, shippedAt, estimatedArrival, actualArrival, documents)
		if err != nil {
			return err
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "shipments",
			EntityType: "shipment",
			EntityID:   sh.ID,
			Action:     "Create",
			After:      map[string]any{"po_id": poID, "status": status},
		})
	})
	return sh, err
}

// UpdateShipment updates a shipment (INK-02).
func (s *Service) UpdateShipment(ctx context.Context, id uuid.UUID, fromLocation, toLocation, carrier, trackingNumber, batchNumber, status *string, shippedAt, estimatedArrival, actualArrival *time.Time, documents *json.RawMessage, version int) (Shipment, error) {
	var sh Shipment
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		var err error
		sh, err = st.UpdateShipment(ctx, fromLocation, toLocation, carrier, trackingNumber, batchNumber, status, shippedAt, estimatedArrival, actualArrival, documents, id, version)
		if err != nil {
			return ProblemError(err)
		}

		after := map[string]any{}
		if status != nil {
			after["status"] = *status
		}
		if trackingNumber != nil {
			after["tracking_number"] = *trackingNumber
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "shipments",
			EntityType: "shipment",
			EntityID:   id,
			Action:     "Update",
			After:      after,
		})
	})
	return sh, err
}

// ArchiveShipment archives a shipment (NFK-09).
func (s *Service) ArchiveShipment(ctx context.Context, id uuid.UUID, version int) (Shipment, error) {
	var updated Shipment
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		sh, err := st.GetShipment(ctx, id)
		if err != nil {
			return ProblemError(err)
		}
		if version == 0 {
			version = sh.Version
		}
		updated, err = st.ArchiveShipment(ctx, id, version)
		if err != nil {
			return ProblemError(err)
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "shipments",
			EntityType: "shipment",
			EntityID:   id,
			Action:     "Archive",
			Before:     map[string]any{"po_id": sh.PurchaseOrderID, "status": sh.Status},
		})
	})
	return updated, err
}
