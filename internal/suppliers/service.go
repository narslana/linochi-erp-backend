package suppliers

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/audit"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
)

// Service holds the business logic for suppliers, contacts and supplier products.
type Service struct {
	store *Store
	rec   *audit.Recorder
	pool  *pgxpool.Pool
}

func NewService(store *Store, rec *audit.Recorder, pool *pgxpool.Pool) *Service {
	return &Service{store: store, rec: rec, pool: pool}
}

// ListSuppliers returns all active suppliers.
func (s *Service) ListSuppliers(ctx context.Context) ([]Supplier, error) {
	return s.store.ListSuppliers(ctx)
}

// CreateSupplier creates a new supplier (LEV-01).
func (s *Service) CreateSupplier(ctx context.Context, name, nameEn, orgNumber, vatNumber, country, address, city, postalCode, website, contactEmail, contactPhone, role string, leadTimeDays, moq int, currency, paymentTerms, deliveryTerms, notes string) (Supplier, error) {
	var sup Supplier
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		id := uuid.New()
		var err error
		sup, err = s.store.CreateSupplier(ctx, id, name, nameEn, orgNumber, vatNumber, country, address, city, postalCode, website, contactEmail, contactPhone, role, leadTimeDays, moq, currency, paymentTerms, deliveryTerms, notes)
		if err != nil {
			return err
		}

		actor, _ := audit.ActorFrom(ctx)
		_ = s.rec.Record(ctx, tx, audit.Entry{
			Module:     "suppliers",
			EntityType: "supplier",
			EntityID:   sup.ID,
			Action:     "Create",
			After:      map[string]any{"name": name, "role": role},
			Reason:     actor.ID.String(),
		})
		return nil
	})
	return sup, err
}

// GetSupplier returns a supplier by ID.
func (s *Service) GetSupplier(ctx context.Context, id uuid.UUID) (Supplier, error) {
	sup, err := s.store.GetSupplier(ctx, id)
	if err != nil {
		return sup, ProblemError(err)
	}
	return sup, nil
}

// UpdateSupplier updates a supplier (LEV-01).
func (s *Service) UpdateSupplier(ctx context.Context, id uuid.UUID, name, nameEn, orgNumber, vatNumber, country, address, city, postalCode, website, contactEmail, contactPhone, role, paymentTerms, deliveryTerms, notes string, leadTimeDays, moq *int, version int) (Supplier, error) {
	sup, err := s.store.UpdateSupplier(ctx, name, nameEn, orgNumber, vatNumber, country, address, city, postalCode, website, contactEmail, contactPhone, role, paymentTerms, deliveryTerms, notes, leadTimeDays, moq, id, version)
	if err != nil {
		return sup, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "suppliers",
		EntityType: "supplier",
		EntityID:   id,
		Action:     "Update",
		After:      map[string]any{"name": name},
		Reason:     actor.ID.String(),
	})
	return sup, nil
}

// ArchiveSupplier archives a supplier (NFK-09).
func (s *Service) ArchiveSupplier(ctx context.Context, id uuid.UUID, version int) (Supplier, error) {
	sup, err := s.store.ArchiveSupplier(ctx, id, version)
	if err != nil {
		return sup, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "suppliers",
		EntityType: "supplier",
		EntityID:   id,
		Action:     "Archive",
		Before:     map[string]any{"name": sup.Name},
		Reason:     actor.ID.String(),
	})
	return sup, nil
}

// ListContacts returns the contacts for a supplier.
func (s *Service) ListContacts(ctx context.Context, supplierID uuid.UUID) ([]SupplierContact, error) {
	return s.store.ListSupplierContacts(ctx, supplierID)
}

// CreateContact adds a contact to a supplier (LEV-01).
func (s *Service) CreateContact(ctx context.Context, supplierID, id uuid.UUID, name, nameEn, email, phone, role string) (SupplierContact, error) {
	c, err := s.store.CreateSupplierContact(ctx, id, supplierID, name, nameEn, email, phone, role)
	if err != nil {
		return c, err
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "suppliers",
		EntityType: "contact",
		EntityID:   c.ID,
		Action:     "Create",
		After:      map[string]any{"name": name, "supplier_id": supplierID},
		Reason:     actor.ID.String(),
	})
	return c, nil
}

// UpdateContact updates a contact (LEV-01).
func (s *Service) UpdateContact(ctx context.Context, id uuid.UUID, name, nameEn, email, phone, role string, version int) (SupplierContact, error) {
	c, err := s.store.UpdateSupplierContact(ctx, name, nameEn, email, phone, role, id, version)
	if err != nil {
		return c, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "suppliers",
		EntityType: "contact",
		EntityID:   id,
		Action:     "Update",
		After:      map[string]any{"name": name},
		Reason:     actor.ID.String(),
	})
	return c, nil
}

// ArchiveContact archives a contact (NFK-09).
func (s *Service) ArchiveContact(ctx context.Context, id uuid.UUID, version int) (SupplierContact, error) {
	c, err := s.store.ArchiveSupplierContact(ctx, id, version)
	if err != nil {
		return c, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "suppliers",
		EntityType: "contact",
		EntityID:   id,
		Action:     "Archive",
		Reason:     actor.ID.String(),
	})
	return c, nil
}

// ListSupplierProducts returns the products delivered by a supplier (LEV-02).
func (s *Service) ListSupplierProducts(ctx context.Context, supplierID uuid.UUID) ([]SupplierProduct, error) {
	return s.store.ListSupplierProducts(ctx, supplierID)
}

// CreateSupplierProduct links a product to a supplier (LEV-02).
func (s *Service) CreateSupplierProduct(ctx context.Context, supplierID, productID uuid.UUID, purchasePrice int64, purchaseCurrency string) (SupplierProduct, error) {
	id := uuid.New()
	sp, err := s.store.CreateSupplierProduct(ctx, id, supplierID, productID, purchasePrice, purchaseCurrency)
	if err != nil {
		return sp, err
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "suppliers",
		EntityType: "supplier_product",
		EntityID:   sp.ID,
		Action:     "Create",
		After:      map[string]any{"supplier_id": supplierID, "product_id": productID},
		Reason:     actor.ID.String(),
	})
	return sp, nil
}

// ArchiveSupplierProduct removes a product link from a supplier (NFK-09).
func (s *Service) ArchiveSupplierProduct(ctx context.Context, id uuid.UUID, version int) (SupplierProduct, error) {
	sp, err := s.store.ArchiveSupplierProduct(ctx, id, version)
	if err != nil {
		return sp, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "suppliers",
		EntityType: "supplier_product",
		EntityID:   id,
		Action:     "Archive",
		Reason:     actor.ID.String(),
	})
	return sp, nil
}
