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

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "suppliers",
			EntityType: "supplier",
			EntityID:   sup.ID,
			Action:     "Create",
			After:      map[string]any{"name": name, "role": role},
		})
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
	var updated Supplier
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		sup, err := s.store.GetSupplier(ctx, id)
		if err != nil {
			return ProblemError(err)
		}
		updated, err = s.store.UpdateSupplier(ctx, name, nameEn, orgNumber, vatNumber, country, address, city, postalCode, website, contactEmail, contactPhone, role, paymentTerms, deliveryTerms, notes, leadTimeDays, moq, id, version)
		if err != nil {
			return ProblemError(err)
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "suppliers",
			EntityType: "supplier",
			EntityID:   id,
			Action:     "Update",
			Before:     map[string]any{"name": sup.Name, "role": sup.Role},
			After:      map[string]any{"name": name, "role": role},
		})
	})
	return updated, err
}

// ArchiveSupplier archives a supplier (NFK-09).
func (s *Service) ArchiveSupplier(ctx context.Context, id uuid.UUID, version int) (Supplier, error) {
	var updated Supplier
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		sup, err := s.store.GetSupplier(ctx, id)
		if err != nil {
			return ProblemError(err)
		}
		updated, err = s.store.ArchiveSupplier(ctx, id, version)
		if err != nil {
			return ProblemError(err)
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "suppliers",
			EntityType: "supplier",
			EntityID:   id,
			Action:     "Archive",
			Before:     map[string]any{"name": sup.Name},
		})
	})
	return updated, err
}

// ListContacts returns the contacts for a supplier.
func (s *Service) ListContacts(ctx context.Context, supplierID uuid.UUID) ([]SupplierContact, error) {
	return s.store.ListSupplierContacts(ctx, supplierID)
}

// CreateContact adds a contact to a supplier (LEV-01).
func (s *Service) CreateContact(ctx context.Context, supplierID, id uuid.UUID, name, nameEn, email, phone, role string) (SupplierContact, error) {
	var c SupplierContact
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		var err error
		c, err = s.store.CreateSupplierContact(ctx, id, supplierID, name, nameEn, email, phone, role)
		if err != nil {
			return err
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "suppliers",
			EntityType: "contact",
			EntityID:   c.ID,
			Action:     "Create",
			After:      map[string]any{"name": name, "supplier_id": supplierID},
		})
	})
	return c, err
}

// UpdateContact updates a contact (LEV-01).
func (s *Service) UpdateContact(ctx context.Context, id uuid.UUID, name, nameEn, email, phone, role string, version int) (SupplierContact, error) {
	var updated SupplierContact
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		c, err := s.store.GetSupplierContact(ctx, id)
		if err != nil {
			return ProblemError(err)
		}
		updated, err = s.store.UpdateSupplierContact(ctx, name, nameEn, email, phone, role, id, version)
		if err != nil {
			return ProblemError(err)
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "suppliers",
			EntityType: "contact",
			EntityID:   id,
			Action:     "Update",
			Before:     map[string]any{"name": c.Name, "email": c.Email},
			After:      map[string]any{"name": name, "email": email},
		})
	})
	return updated, err
}

// ArchiveContact archives a contact (NFK-09).
func (s *Service) ArchiveContact(ctx context.Context, id uuid.UUID, version int) (SupplierContact, error) {
	var updated SupplierContact
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		c, err := s.store.GetSupplierContact(ctx, id)
		if err != nil {
			return ProblemError(err)
		}
		updated, err = s.store.ArchiveSupplierContact(ctx, id, version)
		if err != nil {
			return ProblemError(err)
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "suppliers",
			EntityType: "contact",
			EntityID:   id,
			Action:     "Archive",
			Before:     map[string]any{"name": c.Name},
		})
	})
	return updated, err
}

// ListSupplierProducts returns the products delivered by a supplier (LEV-02).
func (s *Service) ListSupplierProducts(ctx context.Context, supplierID uuid.UUID) ([]SupplierProduct, error) {
	return s.store.ListSupplierProducts(ctx, supplierID)
}

// CreateSupplierProduct links a product to a supplier (LEV-02).
func (s *Service) CreateSupplierProduct(ctx context.Context, supplierID, productID uuid.UUID, purchasePrice int64, purchaseCurrency string) (SupplierProduct, error) {
	var sp SupplierProduct
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		id := uuid.New()
		var err error
		sp, err = s.store.CreateSupplierProduct(ctx, id, supplierID, productID, purchasePrice, purchaseCurrency)
		if err != nil {
			return err
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "suppliers",
			EntityType: "supplier_product",
			EntityID:   sp.ID,
			Action:     "Create",
			After:      map[string]any{"supplier_id": supplierID, "product_id": productID},
		})
	})
	return sp, err
}

// ArchiveSupplierProduct removes a product link from a supplier (NFK-09).
func (s *Service) ArchiveSupplierProduct(ctx context.Context, id uuid.UUID, version int) (SupplierProduct, error) {
	var updated SupplierProduct
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		sp, err := s.store.GetSupplierProduct(ctx, id)
		if err != nil {
			return ProblemError(err)
		}
		updated, err = s.store.ArchiveSupplierProduct(ctx, id, version)
		if err != nil {
			return ProblemError(err)
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "suppliers",
			EntityType: "supplier_product",
			EntityID:   id,
			Action:     "Archive",
			Before:     map[string]any{"product_id": sp.ProductID},
		})
	})
	return updated, err
}
