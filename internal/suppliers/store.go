package suppliers

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db/dbgen"
)

// Store wraps sqlc queries for the suppliers module.
type Store struct {
	q *dbgen.Queries
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{q: dbgen.New(pool)}
}

// ─── Suppliers ────────────────────────────────────────────────────────────────

func (s *Store) ListSuppliers(ctx context.Context) ([]Supplier, error) {
	rows, err := s.q.ListSuppliers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Supplier, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToSupplier(r))
	}
	return out, nil
}

func (s *Store) GetSupplier(ctx context.Context, id uuid.UUID) (Supplier, error) {
	row, err := s.q.GetSupplier(ctx, id)
	if err != nil {
		return Supplier{}, err
	}
	return dbRowToSupplier(row), nil
}

func (s *Store) CreateSupplier(ctx context.Context, id uuid.UUID, name, nameEn, orgNumber, vatNumber, country, address, city, postalCode, website, contactEmail, contactPhone, role string, leadTimeDays, moq int, currency, paymentTerms, deliveryTerms, notes string) (Supplier, error) {
	row, err := s.q.CreateSupplier(ctx, dbgen.CreateSupplierParams{
		ID:            id,
		Name:          name,
		NameEn:        nameEn,
		OrgNumber:     orgNumber,
		VatNumber:     vatNumber,
		Country:       country,
		Address:       address,
		City:          city,
		PostalCode:    postalCode,
		Website:       website,
		ContactEmail:  contactEmail,
		ContactPhone:  contactPhone,
		Role:          role,
		LeadTimeDays:  int32(leadTimeDays), //nolint:gosec // lead time days is bounded by business logic
		Moq:           int32(moq),          //nolint:gosec // MOQ is bounded by business logic
		Currency:      currency,
		PaymentTerms:  paymentTerms,
		DeliveryTerms: deliveryTerms,
		Notes:         notes,
	})
	if err != nil {
		return Supplier{}, err
	}
	return dbRowToSupplier(row), nil
}

func (s *Store) UpdateSupplier(ctx context.Context, name, nameEn, orgNumber, vatNumber, country, address, city, postalCode, website, contactEmail, contactPhone, role, paymentTerms, deliveryTerms, notes string, leadTimeDays, moq *int, id uuid.UUID, version int) (Supplier, error) {
	var params dbgen.UpdateSupplierParams
	params.ID = id
	params.Version = int32(version) //nolint:gosec // version is bounded by optimistic locking
	params.Name = name
	params.NameEn = nameEn
	params.OrgNumber = orgNumber
	params.VatNumber = vatNumber
	params.Country = country
	params.Address = address
	params.City = city
	params.PostalCode = postalCode
	params.Website = website
	params.ContactEmail = contactEmail
	params.ContactPhone = contactPhone
	params.Role = role
	params.PaymentTerms = paymentTerms
	params.DeliveryTerms = deliveryTerms
	params.Notes = notes
	if leadTimeDays != nil {
		params.LeadTimeDays = int32(*leadTimeDays) //nolint:gosec // lead time days is bounded by business logic
	}
	if moq != nil {
		params.Moq = int32(*moq) //nolint:gosec // MOQ is bounded by business logic
	}
	row, err := s.q.UpdateSupplier(ctx, params)
	if err != nil {
		return Supplier{}, err
	}
	return dbRowToSupplier(row), nil
}

func (s *Store) ArchiveSupplier(ctx context.Context, id uuid.UUID, version int) (Supplier, error) {
	row, err := s.q.ArchiveSupplier(ctx, dbgen.ArchiveSupplierParams{ID: id, Version: int32(version)}) //nolint:gosec // version is bounded by optimistic locking
	if err != nil {
		return Supplier{}, err
	}
	return dbRowToSupplier(row), nil
}

// ─── Contacts ─────────────────────────────────────────────────────────────────

func (s *Store) ListSupplierContacts(ctx context.Context, supplierID uuid.UUID) ([]SupplierContact, error) {
	rows, err := s.q.ListSupplierContacts(ctx, supplierID)
	if err != nil {
		return nil, err
	}
	out := make([]SupplierContact, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToContact(r))
	}
	return out, nil
}

func (s *Store) GetSupplierContact(ctx context.Context, id uuid.UUID) (SupplierContact, error) {
	row, err := s.q.GetSupplierContact(ctx, id)
	if err != nil {
		return SupplierContact{}, err
	}
	return dbRowToContact(row), nil
}

func (s *Store) CreateSupplierContact(ctx context.Context, id, supplierID uuid.UUID, name, nameEn, email, phone, role string) (SupplierContact, error) {
	row, err := s.q.CreateSupplierContact(ctx, dbgen.CreateSupplierContactParams{
		ID:         id,
		SupplierID: supplierID,
		Name:       name,
		NameEn:     nameEn,
		Email:      email,
		Phone:      phone,
		Role:       role,
	})
	if err != nil {
		return SupplierContact{}, err
	}
	return dbRowToContact(row), nil
}

func (s *Store) UpdateSupplierContact(ctx context.Context, name, nameEn, email, phone, role string, id uuid.UUID, version int) (SupplierContact, error) {
	row, err := s.q.UpdateSupplierContact(ctx, dbgen.UpdateSupplierContactParams{
		ID:      id,
		Version: int32(version), //nolint:gosec // version is bounded by optimistic locking
		Name:    name,
		NameEn:  nameEn,
		Email:   email,
		Phone:   phone,
		Role:    role,
	})
	if err != nil {
		return SupplierContact{}, err
	}
	return dbRowToContact(row), nil
}

func (s *Store) ArchiveSupplierContact(ctx context.Context, id uuid.UUID, version int) (SupplierContact, error) {
	row, err := s.q.ArchiveSupplierContact(ctx, dbgen.ArchiveSupplierContactParams{ID: id, Version: int32(version)}) //nolint:gosec // version is bounded by optimistic locking
	if err != nil {
		return SupplierContact{}, err
	}
	return dbRowToContact(row), nil
}

// ─── Supplier Products ────────────────────────────────────────────────────────

func (s *Store) ListSupplierProducts(ctx context.Context, supplierID uuid.UUID) ([]SupplierProduct, error) {
	rows, err := s.q.ListSupplierProducts(ctx, supplierID)
	if err != nil {
		return nil, err
	}
	out := make([]SupplierProduct, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToSupplierProduct(r))
	}
	return out, nil
}

func (s *Store) GetSupplierProduct(ctx context.Context, id uuid.UUID) (SupplierProduct, error) {
	row, err := s.q.GetSupplierProduct(ctx, id)
	if err != nil {
		return SupplierProduct{}, err
	}
	return dbRowToSupplierProduct(row), nil
}

func (s *Store) CreateSupplierProduct(ctx context.Context, id, supplierID, productID uuid.UUID, purchasePrice int64, purchaseCurrency string) (SupplierProduct, error) {
	row, err := s.q.CreateSupplierProduct(ctx, dbgen.CreateSupplierProductParams{
		ID:               id,
		SupplierID:       supplierID,
		ProductID:        productID,
		PurchasePrice:    purchasePrice,
		PurchaseCurrency: purchaseCurrency,
	})
	if err != nil {
		return SupplierProduct{}, err
	}
	return dbRowToSupplierProduct(row), nil
}

func (s *Store) ArchiveSupplierProduct(ctx context.Context, id uuid.UUID, version int) (SupplierProduct, error) {
	row, err := s.q.ArchiveSupplierProduct(ctx, dbgen.ArchiveSupplierProductParams{ID: id, Version: int32(version)}) //nolint:gosec // version is bounded by optimistic locking
	if err != nil {
		return SupplierProduct{}, err
	}
	return dbRowToSupplierProduct(row), nil
}

// ─── Row helpers ──────────────────────────────────────────────────────────────

func dbRowToSupplier(r dbgen.Supplier) Supplier {
	s := Supplier{
		ID:            r.ID,
		Name:          r.Name,
		NameEn:        r.NameEn,
		OrgNumber:     r.OrgNumber,
		VatNumber:     r.VatNumber,
		Country:       r.Country,
		Address:       r.Address,
		City:          r.City,
		PostalCode:    r.PostalCode,
		Website:       r.Website,
		ContactEmail:  r.ContactEmail,
		ContactPhone:  r.ContactPhone,
		Role:          r.Role,
		LeadTimeDays:  int(r.LeadTimeDays),
		Moq:           int(r.Moq),
		Currency:      r.Currency,
		PaymentTerms:  r.PaymentTerms,
		DeliveryTerms: r.DeliveryTerms,
		Notes:         r.Notes,
		Version:       int(r.Version),
		CreatedAt:     r.CreatedAt.Format(time.RFC3339),
		UpdatedAt:     r.UpdatedAt.Format(time.RFC3339),
	}
	if r.ArchivedAt != nil {
		s.Archived = true
		t := r.ArchivedAt.Format(time.RFC3339)
		s.ArchivedAt = &t
	}
	return s
}

func dbRowToContact(r dbgen.SupplierContact) SupplierContact {
	c := SupplierContact{
		ID:         r.ID,
		SupplierID: r.SupplierID,
		Name:       r.Name,
		NameEn:     r.NameEn,
		Email:      r.Email,
		Phone:      r.Phone,
		Role:       r.Role,
		Version:    int(r.Version),
		CreatedAt:  r.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  r.UpdatedAt.Format(time.RFC3339),
	}
	if r.ArchivedAt != nil {
		c.Archived = true
		t := r.ArchivedAt.Format(time.RFC3339)
		c.ArchivedAt = &t
	}
	return c
}

func dbRowToSupplierProduct(r dbgen.SupplierProduct) SupplierProduct {
	sp := SupplierProduct{
		ID:               r.ID,
		SupplierID:       r.SupplierID,
		ProductID:        r.ProductID,
		PurchasePrice:    r.PurchasePrice,
		PurchaseCurrency: r.PurchaseCurrency,
		Version:          int(r.Version),
		CreatedAt:        r.CreatedAt.Format(time.RFC3339),
		UpdatedAt:        r.UpdatedAt.Format(time.RFC3339),
	}
	if r.ArchivedAt != nil {
		sp.Archived = true
		t := r.ArchivedAt.Format(time.RFC3339)
		sp.ArchivedAt = &t
	}
	return sp
}
