package inventory

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/httpx"
)

// Domain errors.
var (
	ErrStaleLocation = db.ErrStaleVersion
	ErrStaleMovement = db.ErrStaleVersion
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
	default:
		return err
	}
}

// InsufficientStockError means the withdrawal or reservation would take the
// available quantity (or the batch quantity) below zero. httpx maps it to 409.
type InsufficientStockError struct {
	Detail string
}

func (e *InsufficientStockError) Error() string { return "insufficient stock: " + e.Detail }

func (e *InsufficientStockError) Problem() httpx.Problem {
	return httpx.Problem{Code: "insufficient-stock", Title: "Not enough stock", Status: http.StatusConflict, Detail: e.Detail}
}

func insufficient(format string, args ...any) error {
	return &InsufficientStockError{Detail: fmt.Sprintf(format, args...)}
}

// InvalidMovementError means the request itself is wrong: unknown type, zero or
// signed quantity, missing reason. httpx maps it to 422.
type InvalidMovementError struct {
	Detail string
}

func (e *InvalidMovementError) Error() string { return "invalid movement: " + e.Detail }

func (e *InvalidMovementError) Problem() httpx.Problem {
	return httpx.Problem{Code: "invalid-movement", Title: "Invalid stock movement", Status: http.StatusUnprocessableEntity, Detail: e.Detail}
}

func invalid(format string, args ...any) error {
	return &InvalidMovementError{Detail: fmt.Sprintf(format, args...)}
}

// Movement types (LAG-02, LAG-04). The sign of a stored quantity is decided by
// the type, here and nowhere else: callers send a magnitude.
const (
	TypeReceipt       = "receipt"
	TypeReturn        = "return"
	TypeSale          = "sale"
	TypeAdjustmentIn  = "adjustment_in"
	TypeAdjustmentOut = "adjustment_out"
	TypeTransferIn    = "transfer_in"
	TypeTransferOut   = "transfer_out"
	TypeReservation   = "reservation"
	TypeRelease       = "release"
)

var movementSign = map[string]int{
	TypeReceipt: 1, TypeAdjustmentIn: 1, TypeTransferIn: 1, TypeRelease: 1, TypeReturn: 1,
	TypeSale: -1, TypeAdjustmentOut: -1, TypeTransferOut: -1, TypeReservation: -1,
}

// SignedQuantity is the one place that turns a magnitude into the stored
// quantity. It refuses an unknown type, zero and a negative number: a sign sent
// by the client is never trusted.
func SignedQuantity(movementType string, quantity int) (int, error) {
	sign, ok := movementSign[movementType]
	if !ok {
		return 0, invalid("unknown movement type %q", movementType)
	}
	if quantity == 0 {
		return 0, invalid("quantity must not be zero")
	}
	if quantity < 0 {
		return 0, invalid("quantity must be a positive number, the sign follows the movement type %q", movementType)
	}
	return sign * quantity, nil
}

// MovementInput is a manual or system movement posted through CreateStockMovement.
type MovementInput struct {
	ProductID         uuid.UUID
	LocationID        uuid.UUID
	BatchNumber       string
	ManufacturingDate *string
	BestBeforeDate    *string
	Type              string
	Quantity          int // magnitude, always positive
	Reason            string
	ReferenceType     string
	ReferenceID       *string
}

// StockRef is a quantity of a product at a location tied to a reference, usually an order line.
type StockRef struct {
	ProductID     uuid.UUID
	LocationID    uuid.UUID
	Quantity      int
	ReferenceType string
	ReferenceID   uuid.UUID
}

// ShipInput ships quantity from stock (LAG-02, LAG-03).
type ShipInput struct {
	StockRef
	// OrderLineID is the line the shipped batches are recorded on. Nil: nothing is recorded.
	OrderLineID uuid.UUID
	// BatchNumber chooses one batch manually. Empty means FEFO.
	BatchNumber string
}

// ShippedBatch is a batch a shipment took stock from.
type ShippedBatch struct {
	BatchNumber string `json:"batch_number"`
	Quantity    int    `json:"quantity"`
}

// TransferInput moves stock between two locations (LAG-01).
type TransferInput struct {
	ProductID      uuid.UUID
	FromLocationID uuid.UUID
	ToLocationID   uuid.UUID
	BatchNumber    string
	Quantity       int
	Reason         string
}

// Location is a place where stock is kept (LAG-01).
type Location struct {
	ID         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	NameEn     string    `json:"name_en"`
	Address    string    `json:"address"`
	City       string    `json:"city"`
	Country    string    `json:"country"`
	Archived   bool      `json:"-"`
	ArchivedAt *string   `json:"archived_at,omitempty"`
	Version    int       `json:"version"`
	CreatedAt  string    `json:"created_at"`
	UpdatedAt  string    `json:"updated_at"`
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
	// MarketplaceQuantity is the product's total available over all locations
	// minus its channel buffer, never below zero (LAG-07). It is the same on
	// every location row of the product.
	MarketplaceQuantity int `json:"marketplace_quantity"`
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
