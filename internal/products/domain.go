package products

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
)

// Domain errors.
var (
	ErrStaleProduct      = db.ErrStaleVersion
	ErrStaleProductImage = db.ErrStaleVersion
	ErrStaleProductDoc   = db.ErrStaleVersion
	ErrStaleFieldDef     = db.ErrStaleVersion
	ErrStaleProductType  = db.ErrStaleVersion
	ErrStalePTF          = db.ErrStaleVersion
)

// NotFound is a domain error for missing resources.
type NotFound struct {
	resource string
}

func (e NotFound) Error() string { return e.resource + " not found" }
func (e NotFound) ProblemError() {}

func newNotFound(resource string) NotFound { return NotFound{resource: resource} }

// FieldDefinition is a dynamic product field (PRD-13, PRD-14).
type FieldDefinition struct {
	ID         uuid.UUID       `json:"id"`
	Name       string          `json:"name"`
	NameEn     string          `json:"name_en"`
	FieldType  string          `json:"field_type"`
	GroupName  string          `json:"group_name"`
	HelpText   string          `json:"help_text"`
	Mandatory  bool            `json:"mandatory"`
	Choices    json.RawMessage `json:"choices"`
	Unit       string          `json:"unit"`
	ShowOnCard bool            `json:"show_on_card"`
	Version    int             `json:"version"`
	CreatedAt  string          `json:"created_at"`
	UpdatedAt  string          `json:"updated_at"`
}

// ProductType represents a kind of product (e.g. Shampoo, Conditioner) (PRD-15).
type ProductType struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	NameEn    string    `json:"name_en"`
	TypeCode  string    `json:"type_code"`
	Version   int       `json:"version"`
	CreatedAt string    `json:"created_at"`
	UpdatedAt string    `json:"updated_at"`
}

// ProductTypeField links a field definition to a product type.
type ProductTypeField struct {
	ID                uuid.UUID `json:"id"`
	ProductTypeID     uuid.UUID `json:"product_type_id"`
	FieldDefinitionID uuid.UUID `json:"field_definition_id"`
	FieldName         string    `json:"field_name"`
	FieldNameEn       string    `json:"field_name_en"`
	FieldType         string    `json:"field_type"`
	GroupName         string    `json:"group_name"`
	SortOrder         int32     `json:"sort_order"`
	MandatoryOverride *bool     `json:"mandatory_override,omitempty"`
	Version           int       `json:"version"`
	CreatedAt         string    `json:"created_at"`
	UpdatedAt         string    `json:"updated_at"`
}

// Product is a sellable item (PRD-01, PRD-02, PRD-04, PRD-08).
type Product struct {
	ID                uuid.UUID       `json:"id"`
	Name              string          `json:"name"`
	SKU               string          `json:"sku"`
	EAN               *string         `json:"ean,omitempty"`
	ProductTypeID     uuid.UUID       `json:"product_type_id"`
	ShortDescription  string          `json:"short_description"`
	WholesalePriceSEK int64           `json:"wholesale_price_sek"`
	WholesaleCurrency string          `json:"wholesale_currency"`
	MSRPSEK           int64           `json:"msrp_sek"`
	MSRPCurrency      string          `json:"msrp_currency"`
	Status            string          `json:"status"`
	Attributes        json.RawMessage `json:"attributes"`
	SKULocked         bool            `json:"sku_locked"`
	Version           int             `json:"version"`
	CreatedAt         string          `json:"created_at"`
	UpdatedAt         string          `json:"updated_at"`
}

// ProductImage is an image attached to a product (PRD-03).
type ProductImage struct {
	ID        uuid.UUID `json:"id"`
	ProductID uuid.UUID `json:"product_id"`
	URL       string    `json:"url"`
	SortOrder int32     `json:"sort_order"`
	IsMain    bool      `json:"is_main"`
	Version   int       `json:"version"`
	CreatedAt string    `json:"created_at"`
	UpdatedAt string    `json:"updated_at"`
}

// ProductDocument is a PDF or similar document attached to a product (PRD-06).
type ProductDocument struct {
	ID           uuid.UUID `json:"id"`
	ProductID    uuid.UUID `json:"product_id"`
	Title        string    `json:"title"`
	TitleEn      string    `json:"title_en"`
	DocumentType string    `json:"document_type"`
	URL          string    `json:"url"`
	ExpiresAt    *string   `json:"expires_at,omitempty"`
	Version      int       `json:"version"`
	CreatedAt    string    `json:"created_at"`
	UpdatedAt    string    `json:"updated_at"`
}

// ProblemError maps domain errors to RFC 9457 problems.
func ProblemError(err error) error {
	switch {
	case errors.Is(err, newNotFound("field_definition")):
		return fmt.Errorf("%w: %w", db.ErrNotFound, errors.New("field definition not found"))
	case errors.Is(err, newNotFound("product_type")):
		return fmt.Errorf("%w: %w", db.ErrNotFound, errors.New("product type not found"))
	case errors.Is(err, newNotFound("product")):
		return fmt.Errorf("%w: %w", db.ErrNotFound, errors.New("product not found"))
	case errors.Is(err, newNotFound("product_image")):
		return fmt.Errorf("%w: %w", db.ErrNotFound, errors.New("product image not found"))
	case errors.Is(err, newNotFound("product_document")):
		return fmt.Errorf("%w: %w", db.ErrNotFound, errors.New("product document not found"))
	default:
		return err
	}
}
