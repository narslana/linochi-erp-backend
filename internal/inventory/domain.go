package inventory

import (
	"errors"
	"fmt"

	"github.com/google/uuid"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
)

// Domain errors.
var (
	ErrStaleLocation   = db.ErrStaleVersion
	ErrStaleMovement   = db.ErrStaleVersion
	ErrNegativeBalance = errors.New("negative balance not allowed")
	ErrInvalidMovement = errors.New("invalid movement type")
)

// NotFound is a domain error for missing resources.
type NotFound struct {
	resource string
}

func (e NotFound) Error() string { return e.resource + " not found" }
func (e NotFound) ProblemError() {}

func newNotFound(resource string) NotFound { return NotFound{resource: resource} }

// ProblemError returns a Problem for the domain error.
func ProblemError(err error) error {
	switch {
	case errors.Is(err, newNotFound("location")):
		return fmt.Errorf("%w: %w", db.ErrNotFound, errors.New("location not found"))
	case errors.Is(err, newNotFound("stockmovement")):
		return fmt.Errorf("%w: %w", db.ErrNotFound, errors.New("stock movement not found"))
	case errors.Is(err, ErrNegativeBalance):
		return fmt.Errorf("%w: %w", db.ErrNotFound, errors.New("negative balance"))
	case errors.Is(err, ErrInvalidMovement):
		return fmt.Errorf("%w: %w", db.ErrNotFound, errors.New("invalid movement type"))
	default:
		return err
	}
}

// Location is a place where stock is kept (LAG-01).
type Location struct {
	ID            uuid.UUID `json:"id"`
	Name          string    `json:"name"`
	NameEn        string    `json:"name_en"`
	Address       string    `json:"address"`
	City          string    `json:"city"`
	Country       string    `json:"country"`
	ChannelBuffer int       `json:"channel_buffer"`
	Archived      bool      `json:"-"`
	ArchivedAt    *string   `json:"archived_at,omitempty"`
	Version       int       `json:"version"`
	CreatedAt     string    `json:"created_at"`
	UpdatedAt     string    `json:"updated_at"`
}

// StockMovement is a change in inventory (LAG-01, LAG-02, LAG-03, LAG-04).
type StockMovement struct {
	ID                uuid.UUID `json:"id"`
	ProductID         uuid.UUID `json:"product_id"`
	LocationID        uuid.UUID `json:"location_id"`
	BatchNumber       string    `json:"batch_number"`
	ManufacturingDate *string   `json:"manufacturing_date,omitempty"`
	BestBeforeDate    *string   `json:"best_before_date,omitempty"`
	MovementType      string    `json:"movement_type"`
	Quantity          int       `json:"quantity"`
	Reason            string    `json:"reason"`
	ReferenceType     string    `json:"reference_type"`
	ReferenceID       *string   `json:"reference_id,omitempty"`
	Version           int       `json:"version"`
	CreatedAt         string    `json:"created_at"`
	UpdatedAt         string    `json:"updated_at"`
}

// Balance is the computed balance for a product at a location (LAG-01, LAG-02).
type Balance struct {
	ProductID  uuid.UUID `json:"product_id"`
	LocationID uuid.UUID `json:"location_id"`
	InStock    int       `json:"in_stock"`
	Reserved   int       `json:"reserved"`
	Available  int       `json:"available"`
}

// InventoryValue is the value of inventory per product per location (LAG-05).
type InventoryValue struct {
	LocationID  uuid.UUID `json:"location_id"`
	ProductID   uuid.UUID `json:"product_id"`
	ProductName string    `json:"product_name"`
	InStock     int       `json:"in_stock"`
	Value       int64     `json:"value"`
}

// FEFOBatch is a batch ordered by FEFO (first-expiry-first-out) (LAG-03).
type FEFOBatch struct {
	ProductID         uuid.UUID `json:"product_id"`
	LocationID        uuid.UUID `json:"location_id"`
	BatchNumber       string    `json:"batch_number"`
	ManufacturingDate *string   `json:"manufacturing_date,omitempty"`
	BestBeforeDate    *string   `json:"best_before_date,omitempty"`
	InStock           int       `json:"in_stock"`
}
