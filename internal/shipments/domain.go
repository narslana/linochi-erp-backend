package shipments

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
)

// Domain errors.
var (
	ErrStaleShipment = db.ErrStaleVersion
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
	case errors.Is(err, newNotFound("shipment")):
		return fmt.Errorf("%w: %w", db.ErrNotFound, errors.New("shipment not found"))
	default:
		return err
	}
}

// Shipment is linked to a purchase order (INK-02).
type Shipment struct {
	ID               uuid.UUID       `json:"id"`
	PurchaseOrderID  uuid.UUID       `json:"purchase_order_id"`
	FromLocation     string          `json:"from_location"`
	ToLocation       string          `json:"to_location"`
	Carrier          string          `json:"carrier"`
	TrackingNumber   string          `json:"tracking_number"`
	ShippedAt        *string         `json:"shipped_at,omitempty"`
	EstimatedArrival *string         `json:"estimated_arrival,omitempty"`
	ActualArrival    *string         `json:"actual_arrival,omitempty"`
	BatchNumber      string          `json:"batch_number"`
	Status           string          `json:"status"`
	Documents        json.RawMessage `json:"documents"`
	Archived         bool            `json:"-"`
	ArchivedAt       *string         `json:"archived_at,omitempty"`
	Version          int             `json:"version"`
	CreatedAt        string          `json:"created_at"`
	UpdatedAt        string          `json:"updated_at"`
}
