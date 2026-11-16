package products

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/audit"
)

// Service holds the business logic for products, field definitions and product types.
type Service struct {
	store *Store
	rec   *audit.Recorder
	pool  *pgxpool.Pool
}

func NewService(store *Store, rec *audit.Recorder, pool *pgxpool.Pool) *Service {
	return &Service{store: store, rec: rec, pool: pool}
}

// ─── Field Definitions (PRD-13, PRD-14, PRD-17, PRD-18) ────────────────────

func (s *Service) ListFieldDefinitions(ctx context.Context) ([]FieldDefinition, error) {
	return s.store.ListFieldDefinitions(ctx)
}

func (s *Service) GetFieldDefinition(ctx context.Context, id uuid.UUID) (FieldDefinition, error) {
	fd, err := s.store.GetFieldDefinition(ctx, id)
	if err != nil {
		return fd, ProblemError(err)
	}
	return fd, nil
}

func (s *Service) CreateFieldDefinition(ctx context.Context, name, nameEn, fieldType, groupName, helpText string, mandatory bool, choices json.RawMessage, unit string, showOnCard bool) (FieldDefinition, error) {
	id := uuid.New()
	fd, err := s.store.CreateFieldDefinition(ctx, id, name, nameEn, fieldType, groupName, helpText, mandatory, choices, unit, showOnCard)
	if err != nil {
		return fd, err
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "field_definitions",
		EntityType: "field_definition",
		EntityID:   id,
		Action:     "Create",
		After:      map[string]any{"name": name, "field_type": fieldType, "group_name": groupName},
		Reason:     actor.ID.String(),
	})
	return fd, nil
}

func (s *Service) UpdateFieldDefinition(ctx context.Context, id uuid.UUID, name, nameEn, fieldType, groupName, helpText, unit string, mandatory bool, choices json.RawMessage, showOnCard bool, version int) (FieldDefinition, error) {
	fd, err := s.store.GetFieldDefinition(ctx, id)
	if err != nil {
		return fd, ProblemError(err)
	}
	if version == 0 {
		version = fd.Version
	}
	updated, err := s.store.UpdateFieldDefinition(ctx, name, nameEn, fieldType, groupName, helpText, unit, mandatory, choices, showOnCard, id, version)
	if err != nil {
		return updated, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "field_definitions",
		EntityType: "field_definition",
		EntityID:   id,
		Action:     "Update",
		After:      map[string]any{"name": name, "field_type": fieldType},
		Reason:     actor.ID.String(),
	})
	return updated, nil
}

func (s *Service) ArchiveFieldDefinition(ctx context.Context, id uuid.UUID, version int) (FieldDefinition, error) {
	fd, err := s.store.GetFieldDefinition(ctx, id)
	if err != nil {
		return fd, ProblemError(err)
	}
	if version == 0 {
		version = fd.Version
	}
	updated, err := s.store.ArchiveFieldDefinition(ctx, id, version)
	if err != nil {
		return updated, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "field_definitions",
		EntityType: "field_definition",
		EntityID:   id,
		Action:     "Archive",
		Before:     map[string]any{"name": fd.Name},
		Reason:     actor.ID.String(),
	})
	return updated, nil
}

// ─── Product Types (PRD-15) ─────────────────────────────────────────────────

func (s *Service) ListProductTypes(ctx context.Context) ([]ProductType, error) {
	return s.store.ListProductTypes(ctx)
}

func (s *Service) GetProductType(ctx context.Context, id uuid.UUID) (ProductType, error) {
	pt, err := s.store.GetProductType(ctx, id)
	if err != nil {
		return pt, ProblemError(err)
	}
	return pt, nil
}

func (s *Service) CreateProductType(ctx context.Context, name, nameEn, typeCode string) (ProductType, error) {
	id := uuid.New()
	pt, err := s.store.CreateProductType(ctx, id, name, nameEn, typeCode)
	if err != nil {
		return pt, err
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "product_types",
		EntityType: "product_type",
		EntityID:   id,
		Action:     "Create",
		After:      map[string]any{"name": name, "type_code": typeCode},
		Reason:     actor.ID.String(),
	})
	return pt, nil
}

func (s *Service) UpdateProductType(ctx context.Context, id uuid.UUID, name, nameEn, typeCode string, version int) (ProductType, error) {
	pt, err := s.store.GetProductType(ctx, id)
	if err != nil {
		return pt, ProblemError(err)
	}
	if version == 0 {
		version = pt.Version
	}
	updated, err := s.store.UpdateProductType(ctx, name, nameEn, typeCode, id, version)
	if err != nil {
		return updated, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "product_types",
		EntityType: "product_type",
		EntityID:   id,
		Action:     "Update",
		After:      map[string]any{"name": name, "type_code": typeCode},
		Reason:     actor.ID.String(),
	})
	return updated, nil
}

func (s *Service) ArchiveProductType(ctx context.Context, id uuid.UUID, version int) (ProductType, error) {
	pt, err := s.store.GetProductType(ctx, id)
	if err != nil {
		return pt, ProblemError(err)
	}
	if version == 0 {
		version = pt.Version
	}
	updated, err := s.store.ArchiveProductType(ctx, id, version)
	if err != nil {
		return updated, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "product_types",
		EntityType: "product_type",
		EntityID:   id,
		Action:     "Archive",
		Before:     map[string]any{"name": pt.Name},
		Reason:     actor.ID.String(),
	})
	return updated, nil
}

// ─── Product Type Fields (PRD-15) ───────────────────────────────────────────

func (s *Service) ListProductTypeFields(ctx context.Context, productTypeID uuid.UUID) ([]ProductTypeField, error) {
	return s.store.ListProductTypeFields(ctx, productTypeID)
}

func (s *Service) AssignFieldToProductType(ctx context.Context, productTypeID, fieldDefinitionID uuid.UUID, sortOrder *int32, mandatoryOverride *bool) (ProductTypeField, error) {
	order := int32(0)
	if sortOrder != nil {
		order = *sortOrder
	} else {
		fields, err := s.store.ListProductTypeFields(ctx, productTypeID)
		if err != nil {
			return ProductTypeField{}, err
		}
		for _, f := range fields {
			if int32(f.SortOrder) >= order {
				order = int32(f.SortOrder) + 1
			}
		}
	}
	id := uuid.New()
	ptf, err := s.store.AssignFieldToProductType(ctx, id, productTypeID, fieldDefinitionID, order, mandatoryOverride)
	if err != nil {
		return ptf, err
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "product_type_fields",
		EntityType: "product_type_field",
		EntityID:   id,
		Action:     "Create",
		After:      map[string]any{"product_type_id": productTypeID, "field_definition_id": fieldDefinitionID},
		Reason:     actor.ID.String(),
	})
	return ptf, nil
}

func (s *Service) UpdateProductTypeField(ctx context.Context, id uuid.UUID, sortOrder *int32, mandatoryOverride *bool, version int) (ProductTypeField, error) {
	var sortOrderVal int32 = 0
	if sortOrder != nil {
		sortOrderVal = *sortOrder
	}
	return s.store.UpdateProductTypeField(ctx, sortOrderVal, mandatoryOverride, id, version)
}

func (s *Service) ArchiveProductTypeField(ctx context.Context, id uuid.UUID, version int) (ProductTypeField, error) {
	return s.store.ArchiveProductTypeField(ctx, id, version)
}

// ─── Products (PRD-01, PRD-02, PRD-04, PRD-08, PRD-09, PRD-10, PRD-16, PRD-17) ──

func (s *Service) ListProducts(ctx context.Context, searchQuery *string) ([]Product, error) {
	if searchQuery != nil && *searchQuery != "" {
		return s.store.SearchProducts(ctx, *searchQuery)
	}
	return s.store.ListProducts(ctx)
}

func (s *Service) GetProduct(ctx context.Context, id uuid.UUID) (Product, error) {
	p, err := s.store.GetProduct(ctx, id)
	if err != nil {
		return p, ProblemError(err)
	}
	return p, nil
}

func (s *Service) CreateProduct(ctx context.Context, name, sku, ean string, productTypeID uuid.UUID, shortDesc string, wholesalePriceSEK, msrpSEK int64, wholesaleCurrency, msrpCurrency, status string, attributes json.RawMessage) (Product, error) {
	id := uuid.New()
	if status == "" {
		status = "draft"
	}
	if wholesaleCurrency == "" {
		wholesaleCurrency = "SEK"
	}
	if msrpCurrency == "" {
		msrpCurrency = "SEK"
	}
	if attributes == nil {
		attributes = json.RawMessage("{}")
	}
	p, err := s.store.CreateProduct(ctx, id, productTypeID, name, sku, ean, shortDesc, wholesaleCurrency, msrpCurrency, status, wholesalePriceSEK, msrpSEK, attributes)
	if err != nil {
		return p, err
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "products",
		EntityType: "product",
		EntityID:   id,
		Action:     "Create",
		After:      map[string]any{"name": name, "sku": sku, "status": status},
		Reason:     actor.ID.String(),
	})
	return p, nil
}

func (s *Service) UpdateProduct(ctx context.Context, id uuid.UUID, name, sku, ean string, productTypeID uuid.UUID, shortDesc string, wholesalePriceSEK, msrpSEK int64, wholesaleCurrency, msrpCurrency, status string, attributes json.RawMessage, version int) (Product, error) {
	p, err := s.store.GetProduct(ctx, id)
	if err != nil {
		return p, ProblemError(err)
	}
	if version == 0 {
		version = p.Version
	}
	if status == "" {
		status = p.Status
	}
	if attributes == nil {
		attributes = p.Attributes
	}
	if productTypeID == uuid.Nil {
		productTypeID = p.ProductTypeID
	}
	updated, err := s.store.UpdateProduct(ctx, name, sku, ean, productTypeID, shortDesc, wholesalePriceSEK, msrpSEK, wholesaleCurrency, msrpCurrency, status, attributes, id, version)
	if err != nil {
		return updated, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "products",
		EntityType: "product",
		EntityID:   id,
		Action:     "Update",
		After:      map[string]any{"name": name, "sku": sku, "status": status},
		Reason:     actor.ID.String(),
	})
	return updated, nil
}

func (s *Service) ArchiveProduct(ctx context.Context, id uuid.UUID, version int) (Product, error) {
	p, err := s.store.GetProduct(ctx, id)
	if err != nil {
		return p, ProblemError(err)
	}
	if version == 0 {
		version = p.Version
	}
	updated, err := s.store.ArchiveProduct(ctx, id, version)
	if err != nil {
		return updated, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "products",
		EntityType: "product",
		EntityID:   id,
		Action:     "Archive",
		Before:     map[string]any{"name": p.Name, "sku": p.SKU},
		Reason:     actor.ID.String(),
	})
	return updated, nil
}

func (s *Service) LockSKU(ctx context.Context, id uuid.UUID, version int) (Product, error) {
	p, err := s.store.GetProduct(ctx, id)
	if err != nil {
		return p, ProblemError(err)
	}
	if version == 0 {
		version = p.Version
	}
	updated, err := s.store.LockSKU(ctx, id, version)
	if err != nil {
		return updated, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "products",
		EntityType: "product",
		EntityID:   id,
		Action:     "LockSKU",
		After:      map[string]any{"sku_locked": true},
		Reason:     actor.ID.String(),
	})
	return updated, nil
}

// ─── Product Images (PRD-03) ────────────────────────────────────────────────

func (s *Service) ListProductImages(ctx context.Context, productID uuid.UUID) ([]ProductImage, error) {
	return s.store.ListProductImages(ctx, productID)
}

func (s *Service) CreateProductImage(ctx context.Context, productID uuid.UUID, url string, sortOrder *int32, isMain bool) (ProductImage, error) {
	order := int32(0)
	if sortOrder != nil {
		order = *sortOrder
	} else {
		images, err := s.store.ListProductImages(ctx, productID)
		if err != nil {
			return ProductImage{}, err
		}
		for _, img := range images {
			if int32(img.SortOrder) >= order {
				order = int32(img.SortOrder) + 1
			}
		}
	}
	id := uuid.New()
	img, err := s.store.CreateProductImage(ctx, id, productID, url, order, isMain)
	if err != nil {
		return img, err
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "product_images",
		EntityType: "product_image",
		EntityID:   id,
		Action:     "Create",
		After:      map[string]any{"product_id": productID, "url": url, "is_main": isMain},
		Reason:     actor.ID.String(),
	})
	return img, nil
}

func (s *Service) UpdateProductImage(ctx context.Context, id uuid.UUID, url string, sortOrder *int32, isMain *bool, version int) (ProductImage, error) {
	images, _ := s.store.ListProductImages(ctx, uuid.Nil)
	_ = images
	if version == 0 {
		// ASSUMPTION: version must be provided for updates
		return ProductImage{}, fmt.Errorf("version is required for update")
	}
	img, err := s.store.UpdateProductImage(ctx, url, int32(0), false, id, version)
	if err != nil {
		return img, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "product_images",
		EntityType: "product_image",
		EntityID:   id,
		Action:     "Update",
		After:      map[string]any{"url": url},
		Reason:     actor.ID.String(),
	})
	return img, nil
}

func (s *Service) ArchiveProductImage(ctx context.Context, id uuid.UUID, version int) (ProductImage, error) {
	if version == 0 {
		return ProductImage{}, fmt.Errorf("version is required for archive")
	}
	img, err := s.store.ArchiveProductImage(ctx, id, version)
	if err != nil {
		return img, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "product_images",
		EntityType: "product_image",
		EntityID:   id,
		Action:     "Archive",
		Reason:     actor.ID.String(),
	})
	return img, nil
}

// ─── Product Documents (PRD-06) ─────────────────────────────────────────────

func (s *Service) ListProductDocuments(ctx context.Context, productID uuid.UUID) ([]ProductDocument, error) {
	return s.store.ListProductDocuments(ctx, productID)
}

func (s *Service) CreateProductDocument(ctx context.Context, productID uuid.UUID, title, titleEn, documentType, url string, expiresAt *time.Time) (ProductDocument, error) {
	id := uuid.New()
	doc, err := s.store.CreateProductDocument(ctx, id, productID, title, titleEn, documentType, url, expiresAt)
	if err != nil {
		return doc, err
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "product_documents",
		EntityType: "product_document",
		EntityID:   id,
		Action:     "Create",
		After:      map[string]any{"product_id": productID, "title": title, "document_type": documentType},
		Reason:     actor.ID.String(),
	})
	return doc, nil
}

func (s *Service) UpdateProductDocument(ctx context.Context, id uuid.UUID, title, titleEn, documentType, url string, expiresAt *time.Time, version int) (ProductDocument, error) {
	if version == 0 {
		return ProductDocument{}, fmt.Errorf("version is required for update")
	}
	doc, err := s.store.UpdateProductDocument(ctx, title, titleEn, documentType, url, expiresAt, id, version)
	if err != nil {
		return doc, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "product_documents",
		EntityType: "product_document",
		EntityID:   id,
		Action:     "Update",
		After:      map[string]any{"title": title, "document_type": documentType},
		Reason:     actor.ID.String(),
	})
	return doc, nil
}

func (s *Service) ArchiveProductDocument(ctx context.Context, id uuid.UUID, version int) (ProductDocument, error) {
	if version == 0 {
		return ProductDocument{}, fmt.Errorf("version is required for archive")
	}
	doc, err := s.store.ArchiveProductDocument(ctx, id, version)
	if err != nil {
		return doc, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "product_documents",
		EntityType: "product_document",
		EntityID:   id,
		Action:     "Archive",
		Reason:     actor.ID.String(),
	})
	return doc, nil
}
