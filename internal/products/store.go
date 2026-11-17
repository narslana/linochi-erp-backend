package products

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db/dbgen"
)

// Store wraps sqlc queries for the products module.
type Store struct {
	q *dbgen.Queries
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{q: dbgen.New(pool)}
}

// ─── Field Definitions ──────────────────────────────────────────────────────

func (s *Store) ListFieldDefinitions(ctx context.Context) ([]FieldDefinition, error) {
	rows, err := s.q.ListFieldDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]FieldDefinition, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToFieldDefinition(r))
	}
	return out, nil
}

func (s *Store) GetFieldDefinition(ctx context.Context, id uuid.UUID) (FieldDefinition, error) {
	row, err := s.q.GetFieldDefinition(ctx, id)
	if err != nil {
		return FieldDefinition{}, err
	}
	return dbRowToFieldDefinition(row), nil
}

func (s *Store) CreateFieldDefinition(ctx context.Context, id uuid.UUID, name, nameEn, fieldType, groupName, helpText string, mandatory bool, choices json.RawMessage, unit string, showOnCard bool) (FieldDefinition, error) {
	row, err := s.q.CreateFieldDefinition(ctx, dbgen.CreateFieldDefinitionParams{
		ID:         id,
		Name:       name,
		NameEn:     nameEn,
		FieldType:  fieldType,
		GroupName:  groupName,
		HelpText:   helpText,
		Mandatory:  mandatory,
		Choices:    choices,
		Unit:       unit,
		ShowOnCard: showOnCard,
	})
	if err != nil {
		return FieldDefinition{}, err
	}
	return dbRowToFieldDefinition(row), nil
}

func (s *Store) UpdateFieldDefinition(ctx context.Context, name, nameEn, fieldType, groupName, helpText, unit string, mandatory bool, choices json.RawMessage, showOnCard bool, id uuid.UUID, version int) (FieldDefinition, error) {
	row, err := s.q.UpdateFieldDefinition(ctx, dbgen.UpdateFieldDefinitionParams{
		Name:       name,
		NameEn:     nameEn,
		FieldType:  fieldType,
		GroupName:  groupName,
		HelpText:   helpText,
		Mandatory:  mandatory,
		Choices:    choices,
		Unit:       unit,
		ShowOnCard: showOnCard,
		ID:         id,
		Version:    int32(version), //nolint:gosec // version is bounded by optimistic locking
	})
	if err != nil {
		return FieldDefinition{}, err
	}
	return dbRowToFieldDefinition(row), nil
}

func (s *Store) ArchiveFieldDefinition(ctx context.Context, id uuid.UUID, version int) (FieldDefinition, error) {
	row, err := s.q.ArchiveFieldDefinition(ctx, dbgen.ArchiveFieldDefinitionParams{ID: id, Version: int32(version)}) //nolint:gosec // version is bounded by optimistic locking
	if err != nil {
		return FieldDefinition{}, err
	}
	return dbRowToFieldDefinition(row), nil
}

// ─── Product Types ──────────────────────────────────────────────────────────

func (s *Store) ListProductTypes(ctx context.Context) ([]ProductType, error) {
	rows, err := s.q.ListProductTypes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ProductType, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToProductType(r))
	}
	return out, nil
}

func (s *Store) GetProductType(ctx context.Context, id uuid.UUID) (ProductType, error) {
	row, err := s.q.GetProductType(ctx, id)
	if err != nil {
		return ProductType{}, err
	}
	return dbRowToProductType(row), nil
}

func (s *Store) CreateProductType(ctx context.Context, id uuid.UUID, name, nameEn, typeCode string) (ProductType, error) {
	row, err := s.q.CreateProductType(ctx, dbgen.CreateProductTypeParams{ID: id, Name: name, NameEn: nameEn, TypeCode: typeCode})
	if err != nil {
		return ProductType{}, err
	}
	return dbRowToProductType(row), nil
}

func (s *Store) UpdateProductType(ctx context.Context, name, nameEn, typeCode string, id uuid.UUID, version int) (ProductType, error) {
	row, err := s.q.UpdateProductType(ctx, dbgen.UpdateProductTypeParams{Name: name, NameEn: nameEn, TypeCode: typeCode, ID: id, Version: int32(version)}) //nolint:gosec // version is bounded by optimistic locking
	if err != nil {
		return ProductType{}, err
	}
	return dbRowToProductType(row), nil
}

func (s *Store) ArchiveProductType(ctx context.Context, id uuid.UUID, version int) (ProductType, error) {
	row, err := s.q.ArchiveProductType(ctx, dbgen.ArchiveProductTypeParams{ID: id, Version: int32(version)}) //nolint:gosec // version is bounded by optimistic locking
	if err != nil {
		return ProductType{}, err
	}
	return dbRowToProductType(row), nil
}

// ─── Product Type Fields ────────────────────────────────────────────────────

func (s *Store) ListProductTypeFields(ctx context.Context, productTypeID uuid.UUID) ([]ProductTypeField, error) {
	rows, err := s.q.ListProductTypeFields(ctx, productTypeID)
	if err != nil {
		return nil, err
	}
	out := make([]ProductTypeField, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToListProductTypeFieldsRow(r))
	}
	return out, nil
}

func (s *Store) GetProductTypeField(ctx context.Context, id uuid.UUID) (ProductTypeField, error) {
	row, err := s.q.GetProductTypeField(ctx, id)
	if err != nil {
		return ProductTypeField{}, err
	}
	return dbRowToProductTypeField(row), nil
}

func (s *Store) AssignFieldToProductType(ctx context.Context, id, productTypeID, fieldDefinitionID uuid.UUID, sortOrder int32, mandatoryOverride *bool) (ProductTypeField, error) {
	row, err := s.q.AssignFieldToProductType(ctx, dbgen.AssignFieldToProductTypeParams{
		ID:                id,
		ProductTypeID:     productTypeID,
		FieldDefinitionID: fieldDefinitionID,
		SortOrder:         sortOrder,
		MandatoryOverride: mandatoryOverride,
	})
	if err != nil {
		return ProductTypeField{}, err
	}
	return dbRowToProductTypeField(row), nil
}

func (s *Store) UpdateProductTypeField(ctx context.Context, sortOrder int32, mandatoryOverride *bool, id uuid.UUID, version int) (ProductTypeField, error) {
	row, err := s.q.UpdateProductTypeField(ctx, dbgen.UpdateProductTypeFieldParams{SortOrder: sortOrder, MandatoryOverride: mandatoryOverride, ID: id, Version: int32(version)}) //nolint:gosec // version is bounded by optimistic locking
	if err != nil {
		return ProductTypeField{}, err
	}
	return dbRowToProductTypeField(row), nil
}

func (s *Store) ArchiveProductTypeField(ctx context.Context, id uuid.UUID, version int) (ProductTypeField, error) {
	row, err := s.q.ArchiveProductTypeField(ctx, dbgen.ArchiveProductTypeFieldParams{ID: id, Version: int32(version)}) //nolint:gosec // version is bounded by optimistic locking
	if err != nil {
		return ProductTypeField{}, err
	}
	return dbRowToProductTypeField(row), nil
}

// ─── Products ───────────────────────────────────────────────────────────────

func (s *Store) ListProducts(ctx context.Context) ([]Product, error) {
	rows, err := s.q.ListProducts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Product, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToProduct(r))
	}
	return out, nil
}

func (s *Store) SearchProducts(ctx context.Context, q string) ([]Product, error) {
	rows, err := s.q.SearchProducts(ctx, "%"+q+"%")
	if err != nil {
		return nil, err
	}
	out := make([]Product, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToProduct(r))
	}
	return out, nil
}

func (s *Store) GetProduct(ctx context.Context, id uuid.UUID) (Product, error) {
	row, err := s.q.GetProduct(ctx, id)
	if err != nil {
		return Product{}, err
	}
	return dbRowToProduct(row), nil
}

func (s *Store) GetProductBySKU(ctx context.Context, sku string) (Product, error) {
	row, err := s.q.GetProductBySKU(ctx, sku)
	if err != nil {
		return Product{}, err
	}
	return dbRowToProduct(row), nil
}

func (s *Store) CreateProduct(ctx context.Context, id, productTypeID uuid.UUID, name, sku, ean, shortDesc, wholesaleCurrency, msrpCurrency, status string, wholesalePriceSEK, msrpSEK int64, attributes json.RawMessage) (Product, error) {
	var eanPtr *string
	if ean != "" {
		eanPtr = &ean
	}
	row, err := s.q.CreateProduct(ctx, dbgen.CreateProductParams{
		ID:                id,
		Name:              name,
		Sku:               sku,
		Ean:               eanPtr,
		ProductTypeID:     productTypeID,
		ShortDescription:  shortDesc,
		WholesalePriceSek: wholesalePriceSEK,
		WholesaleCurrency: wholesaleCurrency,
		MsrpSek:           msrpSEK,
		MsrpCurrency:      msrpCurrency,
		Status:            status,
		Attributes:        attributes,
	})
	if err != nil {
		return Product{}, err
	}
	return dbRowToProduct(row), nil
}

func (s *Store) UpdateProduct(ctx context.Context, name, sku, ean string, productTypeID uuid.UUID, shortDesc string, wholesalePriceSEK, msrpSEK int64, wholesaleCurrency, msrpCurrency, status string, attributes json.RawMessage, id uuid.UUID, version int) (Product, error) {
	var eanPtr *string
	if ean != "" {
		eanPtr = &ean
	}
	row, err := s.q.UpdateProduct(ctx, dbgen.UpdateProductParams{
		Name:              name,
		Sku:               sku,
		Ean:               eanPtr,
		ProductTypeID:     productTypeID,
		ShortDescription:  shortDesc,
		WholesalePriceSek: wholesalePriceSEK,
		WholesaleCurrency: wholesaleCurrency,
		MsrpSek:           msrpSEK,
		MsrpCurrency:      msrpCurrency,
		Status:            status,
		Attributes:        attributes,
		ID:                id,
		Version:           int32(version), //nolint:gosec // version is bounded by optimistic locking
	})
	if err != nil {
		return Product{}, err
	}
	return dbRowToProduct(row), nil
}

func (s *Store) ArchiveProduct(ctx context.Context, id uuid.UUID, version int) (Product, error) {
	row, err := s.q.ArchiveProduct(ctx, dbgen.ArchiveProductParams{ID: id, Version: int32(version)}) //nolint:gosec // version is bounded by optimistic locking
	if err != nil {
		return Product{}, err
	}
	return dbRowToProduct(row), nil
}

func (s *Store) LockSKU(ctx context.Context, id uuid.UUID, version int) (Product, error) {
	row, err := s.q.LockSKU(ctx, dbgen.LockSKUParams{ID: id, Version: int32(version)}) //nolint:gosec // version is bounded by optimistic locking
	if err != nil {
		return Product{}, err
	}
	return dbRowToProduct(row), nil
}

// ─── Product Images ─────────────────────────────────────────────────────────

func (s *Store) ListProductImages(ctx context.Context, productID uuid.UUID) ([]ProductImage, error) {
	rows, err := s.q.ListProductImages(ctx, productID)
	if err != nil {
		return nil, err
	}
	out := make([]ProductImage, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToProductImage(r))
	}
	return out, nil
}

func (s *Store) GetProductImage(ctx context.Context, id uuid.UUID) (ProductImage, error) {
	row, err := s.q.GetProductImage(ctx, id)
	if err != nil {
		return ProductImage{}, err
	}
	return dbRowToProductImage(row), nil
}

func (s *Store) CreateProductImage(ctx context.Context, id, productID uuid.UUID, url string, sortOrder int32, isMain bool) (ProductImage, error) {
	row, err := s.q.CreateProductImage(ctx, dbgen.CreateProductImageParams{ID: id, ProductID: productID, Url: url, SortOrder: sortOrder, IsMain: isMain})
	if err != nil {
		return ProductImage{}, err
	}
	return dbRowToProductImage(row), nil
}

func (s *Store) UpdateProductImage(ctx context.Context, url string, sortOrder int32, isMain bool, id uuid.UUID, version int) (ProductImage, error) {
	row, err := s.q.UpdateProductImage(ctx, dbgen.UpdateProductImageParams{Url: url, SortOrder: sortOrder, IsMain: isMain, ID: id, Version: int32(version)}) //nolint:gosec // version is bounded by optimistic locking
	if err != nil {
		return ProductImage{}, err
	}
	return dbRowToProductImage(row), nil
}

func (s *Store) ArchiveProductImage(ctx context.Context, id uuid.UUID, version int) (ProductImage, error) {
	row, err := s.q.ArchiveProductImage(ctx, dbgen.ArchiveProductImageParams{ID: id, Version: int32(version)}) //nolint:gosec // version is bounded by optimistic locking
	if err != nil {
		return ProductImage{}, err
	}
	return dbRowToProductImage(row), nil
}

// ─── Product Documents ──────────────────────────────────────────────────────

func (s *Store) ListProductDocuments(ctx context.Context, productID uuid.UUID) ([]ProductDocument, error) {
	rows, err := s.q.ListProductDocuments(ctx, productID)
	if err != nil {
		return nil, err
	}
	out := make([]ProductDocument, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToProductDocument(r))
	}
	return out, nil
}

func (s *Store) GetProductDocument(ctx context.Context, id uuid.UUID) (ProductDocument, error) {
	row, err := s.q.GetProductDocument(ctx, id)
	if err != nil {
		return ProductDocument{}, err
	}
	return dbRowToProductDocument(row), nil
}

func (s *Store) CreateProductDocument(ctx context.Context, id, productID uuid.UUID, title, titleEn, documentType, url string, expiresAt *time.Time) (ProductDocument, error) {
	row, err := s.q.CreateProductDocument(ctx, dbgen.CreateProductDocumentParams{
		ID:           id,
		ProductID:    productID,
		Title:        title,
		TitleEn:      titleEn,
		DocumentType: documentType,
		Url:          url,
		ExpiresAt:    expiresAt,
	})
	if err != nil {
		return ProductDocument{}, err
	}
	return dbRowToProductDocument(row), nil
}

func (s *Store) UpdateProductDocument(ctx context.Context, title, titleEn, documentType, url string, expiresAt *time.Time, id uuid.UUID, version int) (ProductDocument, error) {
	row, err := s.q.UpdateProductDocument(ctx, dbgen.UpdateProductDocumentParams{
		Title:        title,
		TitleEn:      titleEn,
		DocumentType: documentType,
		Url:          url,
		ExpiresAt:    expiresAt,
		ID:           id,
		Version:      int32(version), //nolint:gosec // version is bounded by optimistic locking
	})
	if err != nil {
		return ProductDocument{}, err
	}
	return dbRowToProductDocument(row), nil
}

func (s *Store) ArchiveProductDocument(ctx context.Context, id uuid.UUID, version int) (ProductDocument, error) {
	row, err := s.q.ArchiveProductDocument(ctx, dbgen.ArchiveProductDocumentParams{ID: id, Version: int32(version)}) //nolint:gosec // version is bounded by optimistic locking
	if err != nil {
		return ProductDocument{}, err
	}
	return dbRowToProductDocument(row), nil
}

// ─── Row Helpers ────────────────────────────────────────────────────────────

func dbRowToFieldDefinition(r dbgen.FieldDefinition) FieldDefinition {
	return FieldDefinition{
		ID:         r.ID,
		Name:       r.Name,
		NameEn:     r.NameEn,
		FieldType:  r.FieldType,
		GroupName:  r.GroupName,
		HelpText:   r.HelpText,
		Mandatory:  r.Mandatory,
		Choices:    r.Choices,
		Unit:       r.Unit,
		ShowOnCard: r.ShowOnCard,
		Version:    int(r.Version),
		CreatedAt:  r.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  r.UpdatedAt.Format(time.RFC3339),
	}
}

func dbRowToProductType(r dbgen.ProductType) ProductType {
	return ProductType{
		ID:        r.ID,
		Name:      r.Name,
		NameEn:    r.NameEn,
		TypeCode:  r.TypeCode,
		Version:   int(r.Version),
		CreatedAt: r.CreatedAt.Format(time.RFC3339),
		UpdatedAt: r.UpdatedAt.Format(time.RFC3339),
	}
}

func dbRowToProductTypeField(r dbgen.ProductTypeField) ProductTypeField {
	p := ProductTypeField{
		ID:                r.ID,
		ProductTypeID:     r.ProductTypeID,
		FieldDefinitionID: r.FieldDefinitionID,
		SortOrder:         r.SortOrder,
		Version:           int(r.Version),
		CreatedAt:         r.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         r.UpdatedAt.Format(time.RFC3339),
	}
	if r.MandatoryOverride != nil {
		v := *r.MandatoryOverride
		p.MandatoryOverride = &v
	}
	return p
}

func dbRowToListProductTypeFieldsRow(r dbgen.ListProductTypeFieldsRow) ProductTypeField {
	p := ProductTypeField{
		ID:                r.ID,
		ProductTypeID:     r.ProductTypeID,
		FieldDefinitionID: r.FieldDefinitionID,
		FieldName:         r.FieldName,
		FieldNameEn:       r.FieldNameEn,
		FieldType:         r.FieldType,
		GroupName:         r.GroupName,
		SortOrder:         r.SortOrder,
		Version:           int(r.Version),
		CreatedAt:         r.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         r.UpdatedAt.Format(time.RFC3339),
	}
	if r.MandatoryOverride != nil {
		v := *r.MandatoryOverride
		p.MandatoryOverride = &v
	}
	return p
}

func dbRowToProduct(r dbgen.Product) Product {
	p := Product{
		ID:                r.ID,
		Name:              r.Name,
		SKU:               r.Sku,
		ProductTypeID:     r.ProductTypeID,
		ShortDescription:  r.ShortDescription,
		WholesalePriceSEK: r.WholesalePriceSek,
		WholesaleCurrency: r.WholesaleCurrency,
		MSRPSEK:           r.MsrpSek,
		MSRPCurrency:      r.MsrpCurrency,
		Status:            r.Status,
		Attributes:        r.Attributes,
		SKULocked:         r.SkuLocked,
		Version:           int(r.Version),
		CreatedAt:         r.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         r.UpdatedAt.Format(time.RFC3339),
	}
	if r.Ean != nil {
		p.EAN = r.Ean
	}
	return p
}

func dbRowToProductImage(r dbgen.ProductImage) ProductImage {
	return ProductImage{
		ID:        r.ID,
		ProductID: r.ProductID,
		URL:       r.Url,
		SortOrder: r.SortOrder,
		IsMain:    r.IsMain,
		Version:   int(r.Version),
		CreatedAt: r.CreatedAt.Format(time.RFC3339),
		UpdatedAt: r.UpdatedAt.Format(time.RFC3339),
	}
}

func dbRowToProductDocument(r dbgen.ProductDocument) ProductDocument {
	p := ProductDocument{
		ID:           r.ID,
		ProductID:    r.ProductID,
		Title:        r.Title,
		TitleEn:      r.TitleEn,
		DocumentType: r.DocumentType,
		URL:          r.Url,
		Version:      int(r.Version),
		CreatedAt:    r.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    r.UpdatedAt.Format(time.RFC3339),
	}
	if r.ExpiresAt != nil {
		s := r.ExpiresAt.Format(time.RFC3339)
		p.ExpiresAt = &s
	}
	return p
}
