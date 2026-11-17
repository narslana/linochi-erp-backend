package orders

import (
	"errors"
	"fmt"

	"github.com/google/uuid"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
)

// Domain errors.
var (
	ErrStaleOrder     = db.ErrStaleVersion
	ErrStaleOrderLine = db.ErrStaleVersion
	ErrInvalidStatus  = errors.New("invalid order status")
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
	case errors.Is(err, newNotFound("order")):
		return fmt.Errorf("%w: %w", db.ErrNotFound, errors.New("order not found"))
	case errors.Is(err, newNotFound("orderline")):
		return fmt.Errorf("%w: %w", db.ErrNotFound, errors.New("order line not found"))
	case errors.Is(err, ErrInvalidStatus):
		return fmt.Errorf("%w: %w", db.ErrNotFound, errors.New("invalid order status"))
	default:
		return err
	}
}

// Order is a sales order (ORD-01).
type Order struct {
	ID                 uuid.UUID `json:"id"`
	ExternalID         *string   `json:"external_id"`
	Channel            string    `json:"channel"`
	Status             string    `json:"status"`
	Currency           string    `json:"currency"`
	ExchangeRateSek    int64     `json:"exchange_rate_sek"`
	SubtotalSek        int64     `json:"subtotal_sek"`
	TaxSek             int64     `json:"tax_sek"`
	FreightSek         int64     `json:"freight_sek"`
	CommissionSek      int64     `json:"commission_sek"`
	TotalSek           int64     `json:"total_sek"`
	CustomerID         uuid.UUID `json:"customer_id"`
	ShippingName       string    `json:"shipping_name"`
	ShippingAddress    string    `json:"shipping_address"`
	ShippingCity       string    `json:"shipping_city"`
	ShippingPostalCode string    `json:"shipping_postal_code"`
	ShippingCountry    string    `json:"shipping_country"`
	BillingName        string    `json:"billing_name"`
	BillingAddress     string    `json:"billing_address"`
	BillingCity        string    `json:"billing_city"`
	BillingPostalCode  string    `json:"billing_postal_code"`
	BillingCountry     string    `json:"billing_country"`
	TrackingNumber     string    `json:"tracking_number,omitempty"`
	Carrier            string    `json:"carrier,omitempty"`
	ShippingLabelUrl   string    `json:"shipping_label_url,omitempty"`
	ShippedAt          *string   `json:"shipped_at,omitempty"`
	DeliveredAt        *string   `json:"delivered_at,omitempty"`
	Notes              string    `json:"notes,omitempty"`
	Archived           bool      `json:"-"`
	ArchivedAt         *string   `json:"archived_at,omitempty"`
	Version            int       `json:"version"`
	CreatedAt          string    `json:"created_at"`
	UpdatedAt          string    `json:"updated_at"`
}

// OrderLine is a line item in an order (ORD-01).
type OrderLine struct {
	ID              uuid.UUID `json:"id"`
	OrderID         uuid.UUID `json:"order_id"`
	ProductID       uuid.UUID `json:"product_id"`
	Quantity        int       `json:"quantity"`
	QuantityShipped int       `json:"quantity_shipped"`
	UnitPriceSek    int64     `json:"unit_price_sek"`
	TaxRate         float64   `json:"tax_rate"`
	TaxAmountSek    int64     `json:"tax_amount_sek"`
	TotalSek        int64     `json:"total_sek"`
	Archived        bool      `json:"-"`
	ArchivedAt      *string   `json:"archived_at,omitempty"`
	Version         int       `json:"version"`
	CreatedAt       string    `json:"created_at"`
	UpdatedAt       string    `json:"updated_at"`
}
