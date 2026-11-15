package suppliers

import (
	"errors"
	"fmt"

	"github.com/google/uuid"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
)

// Domain errors.
var (
	ErrStaleSupplier        = db.ErrStaleVersion
	ErrStaleContact         = db.ErrStaleVersion
	ErrStaleSupplierProduct = db.ErrStaleVersion
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
	case errors.Is(err, newNotFound("supplier")):
		return fmt.Errorf("%w: %w", db.ErrNotFound, errors.New("supplier not found"))
	case errors.Is(err, newNotFound("contact")):
		return fmt.Errorf("%w: %w", db.ErrNotFound, errors.New("contact not found"))
	case errors.Is(err, newNotFound("supplier_product")):
		return fmt.Errorf("%w: %w", db.ErrNotFound, errors.New("supplier product link not found"))
	default:
		return err
	}
}

// Supplier is a company that delivers products or services.
type Supplier struct {
	ID            uuid.UUID `json:"id"`
	Name          string    `json:"name"`
	NameEn        string    `json:"name_en"`
	OrgNumber     string    `json:"org_number"`
	VatNumber     string    `json:"vat_number"`
	Country       string    `json:"country"`
	Address       string    `json:"address"`
	City          string    `json:"city"`
	PostalCode    string    `json:"postal_code"`
	Website       string    `json:"website"`
	ContactEmail  string    `json:"contact_email"`
	ContactPhone  string    `json:"contact_phone"`
	Role          string    `json:"role"`
	LeadTimeDays  int       `json:"lead_time_days"`
	Moq           int       `json:"moq"`
	Currency      string    `json:"currency"`
	PaymentTerms  string    `json:"payment_terms"`
	DeliveryTerms string    `json:"delivery_terms"`
	Notes         string    `json:"notes"`
	Archived      bool      `json:"-"`
	ArchivedAt    *string   `json:"archived_at,omitempty"`
	Version       int       `json:"version"`
	CreatedAt     string    `json:"created_at"`
	UpdatedAt     string    `json:"updated_at"`
}

// SupplierContact belongs to a supplier.
type SupplierContact struct {
	ID         uuid.UUID `json:"id"`
	SupplierID uuid.UUID `json:"supplier_id"`
	Name       string    `json:"name"`
	NameEn     string    `json:"name_en"`
	Email      string    `json:"email"`
	Phone      string    `json:"phone"`
	Role       string    `json:"role"`
	Archived   bool      `json:"-"`
	ArchivedAt *string   `json:"archived_at,omitempty"`
	Version    int       `json:"version"`
	CreatedAt  string    `json:"created_at"`
	UpdatedAt  string    `json:"updated_at"`
}

// SupplierProduct links a supplier to a product with purchase details (LEV-02).
type SupplierProduct struct {
	ID               uuid.UUID `json:"id"`
	SupplierID       uuid.UUID `json:"supplier_id"`
	ProductID        uuid.UUID `json:"product_id"`
	PurchasePrice    int64     `json:"purchase_price"`
	PurchaseCurrency string    `json:"purchase_currency"`
	Archived         bool      `json:"-"`
	ArchivedAt       *string   `json:"archived_at,omitempty"`
	Version          int       `json:"version"`
	CreatedAt        string    `json:"created_at"`
	UpdatedAt        string    `json:"updated_at"`
}
