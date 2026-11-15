package purchasing

import (
	"errors"
	"fmt"

	"github.com/google/uuid"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
)

// Domain errors.
var (
	ErrStalePurchaseOrder     = db.ErrStaleVersion
	ErrStalePurchaseOrderLine = db.ErrStaleVersion
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
	case errors.Is(err, newNotFound("purchase_order")):
		return fmt.Errorf("%w: %w", db.ErrNotFound, errors.New("purchase order not found"))
	case errors.Is(err, newNotFound("purchase_order_line")):
		return fmt.Errorf("%w: %w", db.ErrNotFound, errors.New("purchase order line not found"))
	default:
		return err
	}
}

// PurchaseOrder is an order to a supplier (INK-01).
type PurchaseOrder struct {
	ID           uuid.UUID `json:"id"`
	SupplierID   uuid.UUID `json:"supplier_id"`
	OrderNumber  string    `json:"order_number"`
	Currency     string    `json:"currency"`
	OrderDate    string    `json:"order_date"`
	ExpectedDate *string   `json:"expected_date,omitempty"`
	Status       string    `json:"status"`
	TotalAmount  int64     `json:"total_amount"`
	Notes        string    `json:"notes"`
	Archived     bool      `json:"-"`
	ArchivedAt   *string   `json:"archived_at,omitempty"`
	Version      int       `json:"version"`
	CreatedAt    string    `json:"created_at"`
	UpdatedAt    string    `json:"updated_at"`
}

// PurchaseOrderLine is a line on a purchase order.
type PurchaseOrderLine struct {
	ID              uuid.UUID `json:"id"`
	PurchaseOrderID uuid.UUID `json:"purchase_order_id"`
	ProductID       uuid.UUID `json:"product_id"`
	Quantity        int       `json:"quantity"`
	UnitPrice       int64     `json:"unit_price"`
	UnitCurrency    string    `json:"unit_currency"`
	LineTotal       int64     `json:"line_total"`
	Archived        bool      `json:"-"`
	ArchivedAt      *string   `json:"archived_at,omitempty"`
	Version         int       `json:"version"`
	CreatedAt       string    `json:"created_at"`
	UpdatedAt       string    `json:"updated_at"`
}
