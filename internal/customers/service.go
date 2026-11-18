package customers

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/audit"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db/dbgen"
)

// Service holds the business logic for customers (KUN-01 to KUN-05).
type Service struct {
	store *Store
	rec   *audit.Recorder
	pool  *pgxpool.Pool
}

func NewService(store *Store, rec *audit.Recorder, pool *pgxpool.Pool) *Service {
	return &Service{store: store, rec: rec, pool: pool}
}

// ─── Customers ────────────────────────────────────────────────────────────────

// ListCustomers returns all active customers (KUN-01).
func (s *Service) ListCustomers(ctx context.Context) ([]Customer, error) {
	return s.store.ListCustomers(ctx)
}

// CreateCustomer creates a new customer (KUN-01).
func (s *Service) CreateCustomer(ctx context.Context, name, typeCode, address, city, country, eori, epr, currency, notes string) (Customer, error) {
	var cust Customer
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		id := uuid.New()
		var err error
		cust, err = st.CreateCustomer(ctx, id, name, typeCode, address, city, country, eori, epr, currency, notes)
		if err != nil {
			return err
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "customers",
			EntityType: "customer",
			EntityID:   cust.ID,
			Action:     "Create",
			After:      map[string]any{"name": name, "type_code": typeCode},
		})
	})
	return cust, err
}

// GetCustomer returns a customer by ID.
func (s *Service) GetCustomer(ctx context.Context, id uuid.UUID) (Customer, error) {
	cust, err := s.store.GetCustomer(ctx, id)
	if err != nil {
		return cust, ProblemError(err)
	}
	return cust, nil
}

// UpdateCustomer updates a customer (KUN-01).
func (s *Service) UpdateCustomer(ctx context.Context, name, typeCode, address, city, country, eori, epr, currency, notes *string, id uuid.UUID, version int) (Customer, error) {
	var updated Customer
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		cust, err := st.GetCustomer(ctx, id)
		if err != nil {
			return ProblemError(err)
		}
		updated, err = st.UpdateCustomer(ctx, name, typeCode, address, city, country, eori, epr, currency, notes, id, version)
		if err != nil {
			return ProblemError(err)
		}

		after := map[string]any{}
		if name != nil {
			after["name"] = *name
		}
		if typeCode != nil {
			after["type_code"] = *typeCode
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "customers",
			EntityType: "customer",
			EntityID:   id,
			Action:     "Update",
			Before:     map[string]any{"name": cust.Name, "type_code": cust.TypeCode},
			After:      after,
		})
	})
	return updated, err
}

// ArchiveCustomer archives a customer (NFK-09).
func (s *Service) ArchiveCustomer(ctx context.Context, id uuid.UUID, version int) (Customer, error) {
	var updated Customer
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		cust, err := st.GetCustomer(ctx, id)
		if err != nil {
			return ProblemError(err)
		}
		updated, err = st.ArchiveCustomer(ctx, id, version)
		if err != nil {
			return ProblemError(err)
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "customers",
			EntityType: "customer",
			EntityID:   id,
			Action:     "Archive",
			Before:     map[string]any{"name": cust.Name},
		})
	})
	return updated, err
}

// ─── Contacts ─────────────────────────────────────────────────────────────────

// ListContacts returns all active contacts for a customer (KUN-01).
func (s *Service) ListContacts(ctx context.Context, customerID uuid.UUID) ([]Contact, error) {
	return s.store.ListContacts(ctx, customerID)
}

// CreateContact creates a new contact (KUN-01).
func (s *Service) CreateContact(ctx context.Context, customerID, id uuid.UUID, firstName, lastName, email, phone string) (Contact, error) {
	var contact Contact
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		var err error
		contact, err = st.CreateContact(ctx, id, customerID, firstName, lastName, email, phone)
		if err != nil {
			return err
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "customers",
			EntityType: "contact",
			EntityID:   contact.ID,
			Action:     "Create",
			After:      map[string]any{"first_name": firstName, "last_name": lastName},
		})
	})
	return contact, err
}

// UpdateContact updates a contact (KUN-01).
func (s *Service) UpdateContact(ctx context.Context, firstName, lastName, email, phone *string, id uuid.UUID, version int) (Contact, error) {
	var updated Contact
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		contact, err := st.GetContact(ctx, id)
		if err != nil {
			return ProblemError(err)
		}
		updated, err = st.UpdateContact(ctx, firstName, lastName, email, phone, id, version)
		if err != nil {
			return ProblemError(err)
		}

		after := map[string]any{}
		if firstName != nil {
			after["first_name"] = *firstName
		}
		if lastName != nil {
			after["last_name"] = *lastName
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "customers",
			EntityType: "contact",
			EntityID:   id,
			Action:     "Update",
			Before:     map[string]any{"first_name": contact.FirstName, "last_name": contact.LastName},
			After:      after,
		})
	})
	return updated, err
}

// ArchiveContact archives a contact (NFK-09).
func (s *Service) ArchiveContact(ctx context.Context, id uuid.UUID, version int) (Contact, error) {
	var updated Contact
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		contact, err := st.GetContact(ctx, id)
		if err != nil {
			return ProblemError(err)
		}
		updated, err = st.ArchiveContact(ctx, id, version)
		if err != nil {
			return ProblemError(err)
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "customers",
			EntityType: "contact",
			EntityID:   id,
			Action:     "Archive",
			Before:     map[string]any{"first_name": contact.FirstName, "last_name": contact.LastName},
		})
	})
	return updated, err
}

// AnonymiseContact anonymises a contact for GDPR (INS-04).
func (s *Service) AnonymiseContact(ctx context.Context, id uuid.UUID, version int) (Contact, error) {
	var updated Contact
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		contact, err := st.GetContact(ctx, id)
		if err != nil {
			return ProblemError(err)
		}
		updated, err = st.AnonymiseContact(ctx, id, version)
		if err != nil {
			return ProblemError(err)
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "customers",
			EntityType: "contact",
			EntityID:   id,
			Action:     "Anonymise",
			Before:     map[string]any{"first_name": contact.FirstName, "email": contact.Email},
		})
	})
	return updated, err
}

// ─── Leads ────────────────────────────────────────────────────────────────────

// ListLeads returns all active leads (KUN-03).
func (s *Service) ListLeads(ctx context.Context) ([]Lead, error) {
	return s.store.ListLeads(ctx)
}

// CreateLead creates a new lead (KUN-03).
func (s *Service) CreateLead(ctx context.Context, company, contactName, email, phone, address, city, country, status, notes string) (Lead, error) {
	var lead Lead
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		id := uuid.New()
		var err error
		lead, err = st.CreateLead(ctx, id, company, contactName, email, phone, address, city, country, status, notes)
		if err != nil {
			return err
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "customers",
			EntityType: "lead",
			EntityID:   lead.ID,
			Action:     "Create",
			After:      map[string]any{"company": company, "status": status},
		})
	})
	return lead, err
}

// GetLead returns a lead by ID.
func (s *Service) GetLead(ctx context.Context, id uuid.UUID) (Lead, error) {
	lead, err := s.store.GetLead(ctx, id)
	if err != nil {
		return lead, ProblemError(err)
	}
	return lead, nil
}

// UpdateLead updates a lead (KUN-03).
func (s *Service) UpdateLead(ctx context.Context, company, contactName, email, phone, address, city, country, status, notes *string, id uuid.UUID, version int) (Lead, error) {
	var updated Lead
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		lead, err := st.GetLead(ctx, id)
		if err != nil {
			return ProblemError(err)
		}
		updated, err = st.UpdateLead(ctx, company, contactName, email, phone, address, city, country, status, notes, id, version)
		if err != nil {
			return ProblemError(err)
		}

		after := map[string]any{}
		if company != nil {
			after["company"] = *company
		}
		if status != nil {
			after["status"] = *status
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "customers",
			EntityType: "lead",
			EntityID:   id,
			Action:     "Update",
			Before:     map[string]any{"company": lead.Company, "status": lead.Status},
			After:      after,
		})
	})
	return updated, err
}

// ArchiveLead archives a lead (NFK-09).
func (s *Service) ArchiveLead(ctx context.Context, id uuid.UUID, version int) (Lead, error) {
	var updated Lead
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		lead, err := st.GetLead(ctx, id)
		if err != nil {
			return ProblemError(err)
		}
		if version == 0 {
			updated, err = st.ArchiveLeadNoVersion(ctx, id)
		} else {
			updated, err = st.ArchiveLead(ctx, id, version)
		}
		if err != nil {
			return ProblemError(err)
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "customers",
			EntityType: "lead",
			EntityID:   id,
			Action:     "Archive",
			Before:     map[string]any{"company": lead.Company},
		})
	})
	return updated, err
}

// ─── EAN Series ───────────────────────────────────────────────────────────────

// ListEanSeries returns all active EAN series (REG-04).
func (s *Service) ListEanSeries(ctx context.Context) ([]EanSeries, error) {
	return s.store.ListEanSeries(ctx)
}

// CreateEanSeries creates a new EAN series (REG-04).
func (s *Service) CreateEanSeries(ctx context.Context, prefix, startCode, endCode, description string) (EanSeries, error) {
	var ean EanSeries
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		id := uuid.New()
		var err error
		ean, err = st.CreateEanSeries(ctx, id, prefix, startCode, endCode, description)
		if err != nil {
			return err
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "customers",
			EntityType: "ean_series",
			EntityID:   ean.ID,
			Action:     "Create",
			After:      map[string]any{"prefix": prefix, "start_code": startCode},
		})
	})
	return ean, err
}

// GetEanSeries returns an EAN series by ID.
func (s *Service) GetEanSeries(ctx context.Context, id uuid.UUID) (EanSeries, error) {
	ean, err := s.store.GetEanSeries(ctx, id)
	if err != nil {
		return ean, ProblemError(err)
	}
	return ean, nil
}

// UpdateEanSeries updates an EAN series (REG-04).
func (s *Service) UpdateEanSeries(ctx context.Context, prefix, startCode, endCode, description *string, id uuid.UUID, version int) (EanSeries, error) {
	var updated EanSeries
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		ean, err := st.GetEanSeries(ctx, id)
		if err != nil {
			return ProblemError(err)
		}
		updated, err = st.UpdateEanSeries(ctx, prefix, startCode, endCode, description, id, version)
		if err != nil {
			return ProblemError(err)
		}

		after := map[string]any{}
		if prefix != nil {
			after["prefix"] = *prefix
		}
		if startCode != nil {
			after["start_code"] = *startCode
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "customers",
			EntityType: "ean_series",
			EntityID:   id,
			Action:     "Update",
			Before:     map[string]any{"prefix": ean.Prefix, "start_code": ean.StartCode},
			After:      after,
		})
	})
	return updated, err
}

// ArchiveEanSeries archives an EAN series (NFK-09).
func (s *Service) ArchiveEanSeries(ctx context.Context, id uuid.UUID, version int) (EanSeries, error) {
	var updated EanSeries
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		ean, err := st.GetEanSeries(ctx, id)
		if err != nil {
			return ProblemError(err)
		}
		updated, err = st.ArchiveEanSeries(ctx, id, version)
		if err != nil {
			return ProblemError(err)
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "customers",
			EntityType: "ean_series",
			EntityID:   id,
			Action:     "Archive",
			Before:     map[string]any{"prefix": ean.Prefix},
		})
	})
	return updated, err
}

// NextFreeEanCode returns the next free EAN code (REG-04).
func (s *Service) NextFreeEanCode(ctx context.Context) (string, error) {
	code, err := s.store.NextFreeEanCode(ctx)
	if err != nil {
		return "", fmt.Errorf("next free EAN code: %w", err)
	}
	return code, nil
}

// ─── Stats ────────────────────────────────────────────────────────────────────

// InactiveCustomers returns customers who have not ordered in X days (KUN-05).
func (s *Service) InactiveCustomers(ctx context.Context, days int) ([]InactiveCustomer, error) {
	return s.store.InactiveCustomers(ctx, days)
}

// CustomerStats returns aggregated order statistics for a customer (KUN-04).
func (s *Service) CustomerStats(ctx context.Context, customerID uuid.UUID) ([]dbgen.CustomerStatsRow, error) {
	row, err := s.store.CustomerStats(ctx, customerID)
	if err != nil {
		return nil, err
	}
	return []dbgen.CustomerStatsRow{row}, nil
}

// TopProductsByCustomer returns the most purchased products for a customer (KUN-04).
func (s *Service) TopProductsByCustomer(ctx context.Context, customerID uuid.UUID) ([]dbgen.TopProductsByCustomerRow, error) {
	return s.store.TopProductsByCustomer(ctx, customerID)
}

// ListCustomerEANSeries returns EAN series for a customer (KUN-07).
func (s *Service) ListCustomerEANSeries(ctx context.Context) ([]EanSeries, error) {
	return s.store.ListCustomerEANSeries(ctx)
}

// ListCustomerLeads returns leads for a customer (KUN-05).
func (s *Service) ListCustomerLeads(ctx context.Context) ([]Lead, error) {
	return s.store.ListCustomerLeads(ctx)
}

// RegisterCustomerEANSeries registers an EAN series for a customer (KUN-07).
func (s *Service) RegisterCustomerEANSeries(ctx context.Context, prefix, startCode, endCode, description string) (EanSeries, error) {
	return s.store.RegisterCustomerEANSeries(ctx, prefix, startCode, endCode, description)
}

// RegisterEANSeries registers a new EAN series (REG-04).
func (s *Service) RegisterEANSeries(ctx context.Context, prefix, startCode, endCode, description string) (EanSeries, error) {
	return s.store.RegisterEANSeries(ctx, prefix, startCode, endCode, description)
}
