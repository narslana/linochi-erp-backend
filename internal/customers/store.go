package customers

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db/dbgen"
)

// Store wraps sqlc queries for the customers module.
type Store struct {
	q *dbgen.Queries
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{q: dbgen.New(pool)}
}

// ─── Customers ────────────────────────────────────────────────────────────────

func (s *Store) ListCustomers(ctx context.Context) ([]Customer, error) {
	rows, err := s.q.ListCustomers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Customer, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToCustomer(r))
	}
	return out, nil
}

func (s *Store) GetCustomer(ctx context.Context, id uuid.UUID) (Customer, error) {
	row, err := s.q.GetCustomer(ctx, id)
	if err != nil {
		return Customer{}, err
	}
	return dbRowToCustomer(row), nil
}

func (s *Store) CreateCustomer(ctx context.Context, id uuid.UUID, name, typeCode, address, city, country, eori, epr, currency, notes string) (Customer, error) {
	row, err := s.q.CreateCustomer(ctx, dbgen.CreateCustomerParams{
		ID:       id,
		Name:     name,
		TypeCode: typeCode,
		Address:  address,
		City:     city,
		Country:  country,
		Eori:     eori,
		Epr:      epr,
		Currency: currency,
		Notes:    notes,
	})
	if err != nil {
		return Customer{}, err
	}
	return dbRowToCustomer(row), nil
}

func (s *Store) UpdateCustomer(ctx context.Context, name, typeCode, address, city, country, eori, epr, currency, notes *string, id uuid.UUID, version int) (Customer, error) {
	var params dbgen.UpdateCustomerParams
	params.ID = id
	params.Version = int32(version) //nolint:gosec // version is bounded by optimistic locking
	if name != nil {
		params.Name = *name
	}
	if typeCode != nil {
		params.TypeCode = *typeCode
	}
	if address != nil {
		params.Address = *address
	}
	if city != nil {
		params.City = *city
	}
	if country != nil {
		params.Country = *country
	}
	if eori != nil {
		params.Eori = *eori
	}
	if epr != nil {
		params.Epr = *epr
	}
	if currency != nil {
		params.Currency = *currency
	}
	if notes != nil {
		params.Notes = *notes
	}
	row, err := s.q.UpdateCustomer(ctx, params)
	if err != nil {
		return Customer{}, err
	}
	return dbRowToCustomer(row), nil
}

func (s *Store) ArchiveCustomer(ctx context.Context, id uuid.UUID, version int) (Customer, error) {
	row, err := s.q.ArchiveCustomer(ctx, dbgen.ArchiveCustomerParams{ID: id, Version: int32(version)}) //nolint:gosec // version is bounded by optimistic locking
	if err != nil {
		return Customer{}, err
	}
	return dbRowToCustomer(row), nil
}

// ─── Contacts ─────────────────────────────────────────────────────────────────

func (s *Store) ListContacts(ctx context.Context, customerID uuid.UUID) ([]Contact, error) {
	rows, err := s.q.ListContacts(ctx, customerID)
	if err != nil {
		return nil, err
	}
	out := make([]Contact, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToContact(r))
	}
	return out, nil
}

func (s *Store) CreateContact(ctx context.Context, id, customerID uuid.UUID, firstName, lastName, email, phone string) (Contact, error) {
	row, err := s.q.CreateContact(ctx, dbgen.CreateContactParams{
		ID:         id,
		CustomerID: customerID,
		FirstName:  firstName,
		LastName:   lastName,
		Email:      email,
		Phone:      phone,
	})
	if err != nil {
		return Contact{}, err
	}
	return dbRowToContact(row), nil
}

func (s *Store) UpdateContact(ctx context.Context, firstName, lastName, email, phone *string, id uuid.UUID, version int) (Contact, error) {
	var params dbgen.UpdateContactParams
	params.ID = id
	params.Version = int32(version) //nolint:gosec // version is bounded by optimistic locking
	if firstName != nil {
		params.FirstName = *firstName
	}
	if lastName != nil {
		params.LastName = *lastName
	}
	if email != nil {
		params.Email = *email
	}
	if phone != nil {
		params.Phone = *phone
	}
	row, err := s.q.UpdateContact(ctx, params)
	if err != nil {
		return Contact{}, err
	}
	return dbRowToContact(row), nil
}

func (s *Store) ArchiveContact(ctx context.Context, id uuid.UUID, version int) (Contact, error) {
	row, err := s.q.ArchiveContact(ctx, dbgen.ArchiveContactParams{ID: id, Version: int32(version)}) //nolint:gosec // version is bounded by optimistic locking
	if err != nil {
		return Contact{}, err
	}
	return dbRowToContact(row), nil
}

func (s *Store) AnonymiseContact(ctx context.Context, id uuid.UUID, version int) (Contact, error) {
	row, err := s.q.AnonymiseContact(ctx, dbgen.AnonymiseContactParams{ID: id, Version: int32(version)}) //nolint:gosec // version is bounded by optimistic locking
	if err != nil {
		return Contact{}, err
	}
	return dbRowToContact(row), nil
}

// ─── Leads ────────────────────────────────────────────────────────────────────

func (s *Store) ListLeads(ctx context.Context) ([]Lead, error) {
	rows, err := s.q.ListLeads(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Lead, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToLead(r))
	}
	return out, nil
}

func (s *Store) GetLead(ctx context.Context, id uuid.UUID) (Lead, error) {
	row, err := s.q.GetLead(ctx, id)
	if err != nil {
		return Lead{}, err
	}
	return dbRowToLead(row), nil
}

func (s *Store) CreateLead(ctx context.Context, id uuid.UUID, company, contactName, email, phone, address, city, country, status, notes string) (Lead, error) {
	row, err := s.q.CreateLead(ctx, dbgen.CreateLeadParams{
		ID:          id,
		Company:     company,
		ContactName: contactName,
		Email:       email,
		Phone:       phone,
		Address:     address,
		City:        city,
		Country:     country,
		Status:      status,
		Notes:       notes,
	})
	if err != nil {
		return Lead{}, err
	}
	return dbRowToLead(row), nil
}

func (s *Store) UpdateLead(ctx context.Context, company, contactName, email, phone, address, city, country, status, notes *string, id uuid.UUID, version int) (Lead, error) {
	var params dbgen.UpdateLeadParams
	params.ID = id
	params.Version = int32(version) //nolint:gosec // version is bounded by optimistic locking
	if company != nil {
		params.Company = *company
	}
	if contactName != nil {
		params.ContactName = *contactName
	}
	if email != nil {
		params.Email = *email
	}
	if phone != nil {
		params.Phone = *phone
	}
	if address != nil {
		params.Address = *address
	}
	if city != nil {
		params.City = *city
	}
	if country != nil {
		params.Country = *country
	}
	if status != nil {
		params.Status = *status
	}
	if notes != nil {
		params.Notes = *notes
	}
	row, err := s.q.UpdateLead(ctx, params)
	if err != nil {
		return Lead{}, err
	}
	return dbRowToLead(row), nil
}

func (s *Store) ArchiveLead(ctx context.Context, id uuid.UUID, version int) (Lead, error) {
	row, err := s.q.ArchiveLead(ctx, dbgen.ArchiveLeadParams{ID: id, Version: int32(version)}) //nolint:gosec // version is bounded by optimistic locking
	if err != nil {
		return Lead{}, err
	}
	return dbRowToLead(row), nil
}

// ─── EAN Series ───────────────────────────────────────────────────────────────

func (s *Store) ListEanSeries(ctx context.Context) ([]EanSeries, error) {
	rows, err := s.q.ListEanSeries(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]EanSeries, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToEanSeries(r))
	}
	return out, nil
}

func (s *Store) GetEanSeries(ctx context.Context, id uuid.UUID) (EanSeries, error) {
	row, err := s.q.GetEanSeries(ctx, id)
	if err != nil {
		return EanSeries{}, err
	}
	return dbRowToEanSeries(row), nil
}

func (s *Store) CreateEanSeries(ctx context.Context, id uuid.UUID, prefix, startCode, endCode, description string) (EanSeries, error) {
	row, err := s.q.CreateEanSeries(ctx, dbgen.CreateEanSeriesParams{
		ID:          id,
		Prefix:      prefix,
		StartCode:   startCode,
		EndCode:     endCode,
		Description: description,
	})
	if err != nil {
		return EanSeries{}, err
	}
	return dbRowToEanSeries(row), nil
}

func (s *Store) UpdateEanSeries(ctx context.Context, prefix, startCode, endCode, description *string, id uuid.UUID, version int) (EanSeries, error) {
	var params dbgen.UpdateEanSeriesParams
	params.ID = id
	params.Version = int32(version) //nolint:gosec // version is bounded by optimistic locking
	if prefix != nil {
		params.Prefix = *prefix
	}
	if startCode != nil {
		params.StartCode = *startCode
	}
	if endCode != nil {
		params.EndCode = *endCode
	}
	if description != nil {
		params.Description = *description
	}
	row, err := s.q.UpdateEanSeries(ctx, params)
	if err != nil {
		return EanSeries{}, err
	}
	return dbRowToEanSeries(row), nil
}

func (s *Store) ArchiveEanSeries(ctx context.Context, id uuid.UUID, version int) (EanSeries, error) {
	row, err := s.q.ArchiveEanSeries(ctx, dbgen.ArchiveEanSeriesParams{ID: id, Version: int32(version)}) //nolint:gosec // version is bounded by optimistic locking
	if err != nil {
		return EanSeries{}, err
	}
	return dbRowToEanSeries(row), nil
}

func (s *Store) NextFreeEanCode(ctx context.Context) (string, error) {
	return s.q.NextFreeEanCode(ctx)
}

// ─── Stats ────────────────────────────────────────────────────────────────────

func (s *Store) InactiveCustomers(ctx context.Context, days int) ([]InactiveCustomer, error) {
	rows, err := s.q.InactiveCustomers(ctx, int32(days)) //nolint:gosec // days is bounded by business logic
	if err != nil {
		return nil, err
	}
	out := make([]InactiveCustomer, 0, len(rows))
	for _, r := range rows {
		var lastOrderDate *string
		if r.LastOrderDate != nil {
			switch v := r.LastOrderDate.(type) {
			case time.Time:
				t := v.Format(time.RFC3339)
				lastOrderDate = &t
			case *time.Time:
				if v != nil {
					t := v.Format(time.RFC3339)
					lastOrderDate = &t
				}
			}
		}
		out = append(out, InactiveCustomer{
			ID:            r.ID,
			Name:          r.Name,
			TypeCode:      r.TypeCode,
			LastOrderDate: lastOrderDate,
			OrderCount:    int(r.OrderCount),
		})
	}
	return out, nil
}

func (s *Store) CustomerStats(ctx context.Context, customerID uuid.UUID) (dbgen.CustomerStatsRow, error) {
	return s.q.CustomerStats(ctx, customerID)
}

func (s *Store) TopProductsByCustomer(ctx context.Context, customerID uuid.UUID) ([]dbgen.TopProductsByCustomerRow, error) {
	return s.q.TopProductsByCustomer(ctx, customerID)
}

// ─── Customer-specific EAN and Leads (KUN-05, KUN-07) ────────────────────────

func (s *Store) ListCustomerEANSeries(ctx context.Context) ([]EanSeries, error) {
	rows, err := s.q.ListCustomerEANSeries(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]EanSeries, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToEanSeries(r))
	}
	return out, nil
}

func (s *Store) ListCustomerLeads(ctx context.Context) ([]Lead, error) {
	rows, err := s.q.ListCustomerLeads(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Lead, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToLead(r))
	}
	return out, nil
}

func (s *Store) RegisterCustomerEANSeries(ctx context.Context, prefix, startCode, endCode, description string) (EanSeries, error) {
	row, err := s.q.RegisterCustomerEANSeries(ctx, dbgen.RegisterCustomerEANSeriesParams{
		Prefix:      prefix,
		StartCode:   startCode,
		EndCode:     endCode,
		Description: description,
	})
	if err != nil {
		return EanSeries{}, err
	}
	return dbRowToEanSeries(row), nil
}

func (s *Store) RegisterEANSeries(ctx context.Context, prefix, startCode, endCode, description string) (EanSeries, error) {
	row, err := s.q.RegisterEANSeries(ctx, dbgen.RegisterEANSeriesParams{
		Prefix:      prefix,
		StartCode:   startCode,
		EndCode:     endCode,
		Description: description,
	})
	if err != nil {
		return EanSeries{}, err
	}
	return dbRowToEanSeries(row), nil
}

// ─── Row helpers ──────────────────────────────────────────────────────────────

func dbRowToCustomer(r dbgen.Customer) Customer {
	c := Customer{
		ID:        r.ID,
		Name:      r.Name,
		TypeCode:  r.TypeCode,
		Address:   r.Address,
		City:      r.City,
		Country:   r.Country,
		Eori:      r.Eori,
		Epr:       r.Epr,
		Currency:  r.Currency,
		Notes:     r.Notes,
		Version:   int(r.Version),
		CreatedAt: r.CreatedAt.Format(time.RFC3339),
		UpdatedAt: r.UpdatedAt.Format(time.RFC3339),
	}
	if r.ArchivedAt != nil {
		c.Archived = true
		t := r.ArchivedAt.Format(time.RFC3339)
		c.ArchivedAt = &t
	}
	return c
}

func dbRowToContact(r dbgen.Contact) Contact {
	c := Contact{
		ID:         r.ID,
		CustomerID: r.CustomerID,
		FirstName:  r.FirstName,
		LastName:   r.LastName,
		Email:      r.Email,
		Phone:      r.Phone,
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

func dbRowToLead(r dbgen.Lead) Lead {
	l := Lead{
		ID:          r.ID,
		Company:     r.Company,
		ContactName: r.ContactName,
		Email:       r.Email,
		Phone:       r.Phone,
		Address:     r.Address,
		City:        r.City,
		Country:     r.Country,
		Status:      r.Status,
		Notes:       r.Notes,
		Version:     int(r.Version),
		CreatedAt:   r.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   r.UpdatedAt.Format(time.RFC3339),
	}
	if r.ArchivedAt != nil {
		l.Archived = true
		t := r.ArchivedAt.Format(time.RFC3339)
		l.ArchivedAt = &t
	}
	return l
}

func dbRowToEanSeries(r dbgen.EanSeries) EanSeries {
	e := EanSeries{
		ID:          r.ID,
		Prefix:      r.Prefix,
		StartCode:   r.StartCode,
		EndCode:     r.EndCode,
		Description: r.Description,
		Version:     int(r.Version),
		CreatedAt:   r.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   r.UpdatedAt.Format(time.RFC3339),
	}
	if r.ArchivedAt != nil {
		e.Archived = true
		t := r.ArchivedAt.Format(time.RFC3339)
		e.ArchivedAt = &t
	}
	return e
}
