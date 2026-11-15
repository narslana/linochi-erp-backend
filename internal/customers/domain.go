package customers

import (
	"errors"
	"fmt"

	"github.com/google/uuid"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
)

// Domain errors.
var (
	ErrStaleCustomer = db.ErrStaleVersion
	ErrStaleContact  = db.ErrStaleVersion
	ErrStaleLead     = db.ErrStaleVersion
	ErrStaleEan      = db.ErrStaleVersion
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
	case errors.Is(err, newNotFound("customer")):
		return fmt.Errorf("%w: %w", db.ErrNotFound, errors.New("customer not found"))
	case errors.Is(err, newNotFound("contact")):
		return fmt.Errorf("%w: %w", db.ErrNotFound, errors.New("contact not found"))
	case errors.Is(err, newNotFound("lead")):
		return fmt.Errorf("%w: %w", db.ErrNotFound, errors.New("lead not found"))
	case errors.Is(err, newNotFound("ean")):
		return fmt.Errorf("%w: %w", db.ErrNotFound, errors.New("EAN series not found"))
	default:
		return err
	}
}

// Customer is a B2B buyer (KUN-01).
type Customer struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	TypeCode  string    `json:"type_code"`
	Address   string    `json:"address"`
	City      string    `json:"city"`
	Country   string    `json:"country"`
	Eori      string    `json:"eori,omitempty"`
	Epr       string    `json:"epr,omitempty"`
	Currency  string    `json:"currency"`
	Notes     string    `json:"notes,omitempty"`
	Archived  bool      `json:"-"`
	ArchivedAt *string  `json:"archived_at,omitempty"`
	Version   int       `json:"version"`
	CreatedAt string    `json:"created_at"`
	UpdatedAt string    `json:"updated_at"`
}

// Contact is a person at a customer company (KUN-01).
type Contact struct {
	ID         uuid.UUID `json:"id"`
	CustomerID uuid.UUID `json:"customer_id"`
	FirstName  string    `json:"first_name"`
	LastName   string    `json:"last_name"`
	Email      string    `json:"email"`
	Phone      string    `json:"phone"`
	Archived   bool      `json:"-"`
	ArchivedAt *string   `json:"archived_at,omitempty"`
	Version    int       `json:"version"`
	CreatedAt  string    `json:"created_at"`
	UpdatedAt  string    `json:"updated_at"`
}

// Lead is a potential customer (KUN-03).
type Lead struct {
	ID          uuid.UUID `json:"id"`
	Company     string    `json:"company"`
	ContactName string    `json:"contact_name"`
	Email       string    `json:"email"`
	Phone       string    `json:"phone"`
	Address     string    `json:"address"`
	City        string    `json:"city"`
	Country     string    `json:"country"`
	Status      string    `json:"status"`
	Notes       string    `json:"notes,omitempty"`
	Archived    bool      `json:"-"`
	ArchivedAt  *string   `json:"archived_at,omitempty"`
	Version     int       `json:"version"`
	CreatedAt   string    `json:"created_at"`
	UpdatedAt   string    `json:"updated_at"`
}

// EanSeries tracks EAN code ranges (REG-04).
type EanSeries struct {
	ID          uuid.UUID `json:"id"`
	Prefix      string    `json:"prefix"`
	StartCode   string    `json:"start_code"`
	EndCode     string    `json:"end_code"`
	Description string    `json:"description,omitempty"`
	Archived    bool      `json:"-"`
	ArchivedAt  *string   `json:"archived_at,omitempty"`
	Version     int       `json:"version"`
	CreatedAt   string    `json:"created_at"`
	UpdatedAt   string    `json:"updated_at"`
}

// CustomerStats holds aggregated order statistics per customer (KUN-04).
type CustomerStats struct {
	TotalOrders    int     `json:"total_orders"`
	TotalValue     int64   `json:"total_value"`
	AvgOrderValue  float64 `json:"avg_order_value"`
	LatestOrder    *string `json:"latest_order,omitempty"`
	Channel        string  `json:"channel"`
	ChannelCount   int     `json:"channel_count"`
}

// TopProduct is a product purchased by a customer (KUN-04).
type TopProduct struct {
	ProductID    uuid.UUID `json:"product_id"`
	ProductName  string    `json:"product_name"`
	TotalQuantity int      `json:"total_quantity"`
}

// InactiveCustomer represents a customer with no recent orders (KUN-05).
type InactiveCustomer struct {
	ID            uuid.UUID `json:"id"`
	Name          string    `json:"name"`
	TypeCode      string    `json:"type_code"`
	LastOrderDate *string   `json:"last_order_date,omitempty"`
	OrderCount    int       `json:"order_count"`
}
