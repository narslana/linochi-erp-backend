package customers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/api"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/auth"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/httpx"
)

// Handler provides the HTTP handlers for customers (KUN-01 to KUN-05).
type Handler struct {
	svc    *Service
	mw     *auth.SessionMiddleware
	logger *slog.Logger
}

// NewHandler creates a new customers handler.
func NewHandler(svc *Service, mw *auth.SessionMiddleware, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, mw: mw, logger: logger}
}

// ─── Customers ────────────────────────────────────────────────────────────────

// ListCustomers returns all active customers (KUN-01).
func (h *Handler) ListCustomers(w http.ResponseWriter, r *http.Request, params api.ListCustomersParams) {
	h.mw.Require(http.HandlerFunc(h.listCustomers)).ServeHTTP(w, r)
}

func (h *Handler) listCustomers(w http.ResponseWriter, r *http.Request) {
	customers, err := h.svc.ListCustomers(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, customers)
}

// CreateCustomer creates a new customer (KUN-01).
func (h *Handler) CreateCustomer(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.createCustomer)).ServeHTTP(w, r)
}

func (h *Handler) createCustomer(w http.ResponseWriter, r *http.Request) {
	var body api.Customer
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	var eori, epr, notes string
	if body.Eori != nil {
		eori = *body.Eori
	}
	if body.Epr != nil {
		epr = *body.Epr
	}
	if body.Notes != nil {
		notes = *body.Notes
	}

	cust, err := h.svc.CreateCustomer(r.Context(), body.Name, body.TypeCode, body.Address, body.City, body.Country, eori, epr, body.Currency, notes)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "CreateCustomer failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, cust)
}

// GetCustomer returns a customer by ID.
func (h *Handler) GetCustomer(w http.ResponseWriter, r *http.Request, customerId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.getCustomer(w, r, customerId)
	})).ServeHTTP(w, r)
}

func (h *Handler) getCustomer(w http.ResponseWriter, r *http.Request, customerId openapi_types.UUID) {
	cust, err := h.svc.GetCustomer(r.Context(), uuid.UUID(customerId))
	if err != nil {
		if errors.Is(err, newNotFound("customer")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "GetCustomer failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, cust)
}

// UpdateCustomer updates a customer (KUN-01).
func (h *Handler) UpdateCustomer(w http.ResponseWriter, r *http.Request, customerId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.updateCustomer(w, r, customerId)
	})).ServeHTTP(w, r)
}

func (h *Handler) updateCustomer(w http.ResponseWriter, r *http.Request, customerId openapi_types.UUID) {
	var body api.Customer
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	var name, typeCode, address, city, country, eori, epr, currency, notes *string
	if body.Name != "" {
		name = &body.Name
	}
	if body.TypeCode != "" {
		typeCode = &body.TypeCode
	}
	if body.Address != "" {
		address = &body.Address
	}
	if body.City != "" {
		city = &body.City
	}
	if body.Country != "" {
		country = &body.Country
	}
	if body.Eori != nil {
		eori = body.Eori
	}
	if body.Epr != nil {
		epr = body.Epr
	}
	if body.Currency != "" {
		currency = &body.Currency
	}
	if body.Notes != nil {
		notes = body.Notes
	}

	cust, err := h.svc.UpdateCustomer(r.Context(), name, typeCode, address, city, country, eori, epr, currency, notes, uuid.UUID(customerId), int(body.Version))
	if err != nil {
		if errors.Is(err, newNotFound("customer")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "UpdateCustomer failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, cust)
}

// ArchiveCustomer archives a customer (NFK-09).
func (h *Handler) ArchiveCustomer(w http.ResponseWriter, r *http.Request, customerId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.archiveCustomer(w, r, customerId)
	})).ServeHTTP(w, r)
}

func (h *Handler) archiveCustomer(w http.ResponseWriter, r *http.Request, customerId openapi_types.UUID) {
	var body struct {
		Version int `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	version := body.Version
	if version == 0 {
		cust, err := h.svc.GetCustomer(r.Context(), uuid.UUID(customerId))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		version = cust.Version
	}

	cust, err := h.svc.ArchiveCustomer(r.Context(), uuid.UUID(customerId), version)
	if err != nil {
		if errors.Is(err, newNotFound("customer")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "ArchiveCustomer failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, cust)
}

// ─── Contacts ─────────────────────────────────────────────────────────────────

// ListContacts returns all active contacts for a customer (KUN-01).
func (h *Handler) ListContacts(w http.ResponseWriter, r *http.Request, customerId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.listContacts(w, r, customerId)
	})).ServeHTTP(w, r)
}

func (h *Handler) listContacts(w http.ResponseWriter, r *http.Request, customerId openapi_types.UUID) {
	contacts, err := h.svc.ListContacts(r.Context(), uuid.UUID(customerId))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, contacts)
}

// CreateContact creates a new contact (KUN-01).
func (h *Handler) CreateContact(w http.ResponseWriter, r *http.Request, customerId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.createContact(w, r, customerId)
	})).ServeHTTP(w, r)
}

// CreateCustomerContact creates a new contact for a customer (KUN-03).
func (h *Handler) CreateCustomerContact(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.createContact(w, r, id)
	})).ServeHTTP(w, r)
}

func (h *Handler) createContact(w http.ResponseWriter, r *http.Request, customerId openapi_types.UUID) {
	var body api.Contact
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	var phone string
	if body.Phone != nil {
		phone = *body.Phone
	}

	id := body.Id
	if id == uuid.Nil {
		id = uuid.New()
	}
	contact, err := h.svc.CreateContact(r.Context(), customerId, id, body.FirstName, body.LastName, body.Email, phone)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "CreateContact failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, contact)
}

// UpdateContact updates a contact (KUN-01).
func (h *Handler) UpdateContact(w http.ResponseWriter, r *http.Request, customerId openapi_types.UUID, contactId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.updateContact(w, r, customerId, contactId)
	})).ServeHTTP(w, r)
}

func (h *Handler) updateContact(w http.ResponseWriter, r *http.Request, customerId openapi_types.UUID, contactId openapi_types.UUID) {
	var body api.Contact
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	var firstName, lastName, email, phone *string
	if body.FirstName != "" {
		firstName = &body.FirstName
	}
	if body.LastName != "" {
		lastName = &body.LastName
	}
	if body.Email != "" {
		email = &body.Email
	}
	if body.Phone != nil {
		phone = body.Phone
	}

	contact, err := h.svc.UpdateContact(r.Context(), firstName, lastName, email, phone, uuid.UUID(contactId), int(body.Version))
	if err != nil {
		if errors.Is(err, newNotFound("contact")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "UpdateContact failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, contact)
}

// ArchiveContact archives a contact (NFK-09).
func (h *Handler) ArchiveContact(w http.ResponseWriter, r *http.Request, customerId openapi_types.UUID, contactId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.archiveContact(w, r, customerId, contactId)
	})).ServeHTTP(w, r)
}

func (h *Handler) archiveContact(w http.ResponseWriter, r *http.Request, customerId openapi_types.UUID, contactId openapi_types.UUID) {
	var body struct {
		Version int `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	contact, err := h.svc.ArchiveContact(r.Context(), uuid.UUID(contactId), body.Version)
	if err != nil {
		if errors.Is(err, newNotFound("contact")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "ArchiveContact failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, contact)
}

// AnonymiseContact anonymises a contact for GDPR (INS-04).
func (h *Handler) AnonymiseContact(w http.ResponseWriter, r *http.Request, customerId openapi_types.UUID, contactId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.anonymiseContact(w, r, customerId, contactId)
	})).ServeHTTP(w, r)
}

func (h *Handler) anonymiseContact(w http.ResponseWriter, r *http.Request, customerId openapi_types.UUID, contactId openapi_types.UUID) {
	var body struct {
		Version int `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	contact, err := h.svc.AnonymiseContact(r.Context(), uuid.UUID(contactId), body.Version)
	if err != nil {
		if errors.Is(err, newNotFound("contact")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "AnonymiseContact failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, contact)
}

// ─── Leads ────────────────────────────────────────────────────────────────────

// ListLeads returns all active leads (KUN-03).
func (h *Handler) ListLeads(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.listLeads)).ServeHTTP(w, r)
}

func (h *Handler) listLeads(w http.ResponseWriter, r *http.Request) {
	leads, err := h.svc.ListLeads(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, leads)
}

// CreateLead creates a new lead (KUN-03).
func (h *Handler) CreateLead(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.createLead)).ServeHTTP(w, r)
}

func (h *Handler) createLead(w http.ResponseWriter, r *http.Request) {
	var body api.Lead
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	var contactName, phone, address, city, country, notes string
	if body.ContactName != nil {
		contactName = *body.ContactName
	}
	if body.Phone != nil {
		phone = *body.Phone
	}
	if body.Address != nil {
		address = *body.Address
	}
	if body.City != nil {
		city = *body.City
	}
	if body.Country != nil {
		country = *body.Country
	}
	if body.Notes != nil {
		notes = *body.Notes
	}

	lead, err := h.svc.CreateLead(r.Context(), body.Company, contactName, body.Email, phone, address, city, country, string(body.Status), notes)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "CreateLead failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, lead)
}

// GetLead returns a lead by ID.
func (h *Handler) GetLead(w http.ResponseWriter, r *http.Request, leadId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.getLead(w, r, leadId)
	})).ServeHTTP(w, r)
}

func (h *Handler) getLead(w http.ResponseWriter, r *http.Request, leadId openapi_types.UUID) {
	lead, err := h.svc.GetLead(r.Context(), uuid.UUID(leadId))
	if err != nil {
		if errors.Is(err, newNotFound("lead")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "GetLead failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, lead)
}

// UpdateLead updates a lead (KUN-03).
func (h *Handler) UpdateLead(w http.ResponseWriter, r *http.Request, leadId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.updateLead(w, r, leadId)
	})).ServeHTTP(w, r)
}

func (h *Handler) updateLead(w http.ResponseWriter, r *http.Request, leadId openapi_types.UUID) {
	var body api.Lead
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	var company, contactName, email, phone, address, city, country, status, notes *string
	if body.Company != "" {
		company = &body.Company
	}
	if body.ContactName != nil {
		contactName = body.ContactName
	}
	if body.Email != "" {
		email = &body.Email
	}
	if body.Phone != nil {
		phone = body.Phone
	}
	if body.Address != nil {
		address = body.Address
	}
	if body.City != nil {
		city = body.City
	}
	if body.Country != nil {
		country = body.Country
	}
	if body.Status != "" {
		s := string(body.Status)
		status = &s
	}
	if body.Notes != nil {
		notes = body.Notes
	}

	lead, err := h.svc.UpdateLead(r.Context(), company, contactName, email, phone, address, city, country, status, notes, uuid.UUID(leadId), int(body.Version))
	if err != nil {
		if errors.Is(err, newNotFound("lead")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "UpdateLead failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, lead)
}

// ArchiveLead archives a lead (NFK-09).
func (h *Handler) ArchiveLead(w http.ResponseWriter, r *http.Request, leadId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.archiveLead(w, r, leadId)
	})).ServeHTTP(w, r)
}

func (h *Handler) archiveLead(w http.ResponseWriter, r *http.Request, leadId openapi_types.UUID) {
	var body struct {
		Version int `json:"version"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body) // version is optional for archive

	lead, err := h.svc.ArchiveLead(r.Context(), uuid.UUID(leadId), body.Version)
	if err != nil {
		if errors.Is(err, newNotFound("lead")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "ArchiveLead failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, lead)
}

// ─── EAN Series ───────────────────────────────────────────────────────────────

// ListEANSeries returns all active EAN series (REG-04).
func (h *Handler) ListEANSeries(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.listEanSeries)).ServeHTTP(w, r)
}

func (h *Handler) listEanSeries(w http.ResponseWriter, r *http.Request) {
	series, err := h.svc.ListEanSeries(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, series)
}

// CreateEanSeries creates a new EAN series (REG-04).
func (h *Handler) CreateEanSeries(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.createEanSeries)).ServeHTTP(w, r)
}

func (h *Handler) createEanSeries(w http.ResponseWriter, r *http.Request) {
	var body api.EANSeries
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	var description string
	if body.Description != nil {
		description = *body.Description
	}

	ean, err := h.svc.CreateEanSeries(r.Context(), body.Prefix, body.StartCode, body.EndCode, description)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "CreateEanSeries failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, ean)
}

// GetEanSeries returns an EAN series by ID.
func (h *Handler) GetEanSeries(w http.ResponseWriter, r *http.Request, eanSeriesId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.getEanSeries(w, r, eanSeriesId)
	})).ServeHTTP(w, r)
}

func (h *Handler) getEanSeries(w http.ResponseWriter, r *http.Request, eanSeriesId openapi_types.UUID) {
	ean, err := h.svc.GetEanSeries(r.Context(), uuid.UUID(eanSeriesId))
	if err != nil {
		if errors.Is(err, newNotFound("ean")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "GetEanSeries failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ean)
}

// UpdateEanSeries updates an EAN series (REG-04).
func (h *Handler) UpdateEanSeries(w http.ResponseWriter, r *http.Request, eanSeriesId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.updateEanSeries(w, r, eanSeriesId)
	})).ServeHTTP(w, r)
}

func (h *Handler) updateEanSeries(w http.ResponseWriter, r *http.Request, eanSeriesId openapi_types.UUID) {
	var body api.EANSeries
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	var prefix, startCode, endCode, description *string
	if body.Prefix != "" {
		prefix = &body.Prefix
	}
	if body.StartCode != "" {
		startCode = &body.StartCode
	}
	if body.EndCode != "" {
		endCode = &body.EndCode
	}
	if body.Description != nil {
		description = body.Description
	}

	ean, err := h.svc.UpdateEanSeries(r.Context(), prefix, startCode, endCode, description, uuid.UUID(eanSeriesId), int(body.Version))
	if err != nil {
		if errors.Is(err, newNotFound("ean")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "UpdateEanSeries failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ean)
}

// ArchiveEanSeries archives an EAN series (NFK-09).
func (h *Handler) ArchiveEanSeries(w http.ResponseWriter, r *http.Request, eanSeriesId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.archiveEanSeries(w, r, eanSeriesId)
	})).ServeHTTP(w, r)
}

func (h *Handler) archiveEanSeries(w http.ResponseWriter, r *http.Request, eanSeriesId openapi_types.UUID) {
	var body struct {
		Version int `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	ean, err := h.svc.ArchiveEanSeries(r.Context(), uuid.UUID(eanSeriesId), body.Version)
	if err != nil {
		if errors.Is(err, newNotFound("ean")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "ArchiveEanSeries failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ean)
}

// NextFreeEanCode returns the next free EAN code (REG-04).
func (h *Handler) NextFreeEanCode(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.nextFreeEanCode)).ServeHTTP(w, r)
}

func (h *Handler) nextFreeEanCode(w http.ResponseWriter, r *http.Request) {
	code, err := h.svc.NextFreeEanCode(r.Context())
	if err != nil {
		h.logger.ErrorContext(r.Context(), "NextFreeEanCode failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"code": code})
}

// ListCustomerContacts returns contacts for a customer (KUN-03).
func (h *Handler) ListCustomerContacts(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.listCustomerContacts(w, r, id)
	})).ServeHTTP(w, r)
}

func (h *Handler) listCustomerContacts(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	contacts, err := h.svc.ListContacts(r.Context(), uuid.UUID(id))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, contacts)
}

// ListCustomerEANSeries returns EAN series for a customer (KUN-07).
func (h *Handler) ListCustomerEANSeries(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(h.listCustomerEANSeries)).ServeHTTP(w, r)
}

func (h *Handler) listCustomerEANSeries(w http.ResponseWriter, r *http.Request) {
	series, err := h.svc.ListCustomerEANSeries(r.Context())
	if err != nil {
		h.logger.ErrorContext(r.Context(), "ListCustomerEANSeries failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, series)
}

// ListCustomerLeads returns leads for a customer (KUN-05).
func (h *Handler) ListCustomerLeads(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(h.listCustomerLeads)).ServeHTTP(w, r)
}

func (h *Handler) listCustomerLeads(w http.ResponseWriter, r *http.Request) {
	leads, err := h.svc.ListCustomerLeads(r.Context())
	if err != nil {
		h.logger.ErrorContext(r.Context(), "ListCustomerLeads failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, leads)
}

// RegisterCustomerEANSeries registers an EAN series for a customer (KUN-07).
func (h *Handler) RegisterCustomerEANSeries(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(h.registerCustomerEANSeries)).ServeHTTP(w, r)
}

func (h *Handler) registerCustomerEANSeries(w http.ResponseWriter, r *http.Request) {
	var body api.RegisterEANSeries
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	var description string
	if body.Description != nil {
		description = *body.Description
	}

	ean, err := h.svc.RegisterCustomerEANSeries(r.Context(), body.Prefix, body.StartCode, body.EndCode, description)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "RegisterCustomerEANSeries failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, ean)
}

// RegisterEANSeries registers a new EAN series (REG-04).
func (h *Handler) RegisterEANSeries(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.registerEANSeries)).ServeHTTP(w, r)
}

func (h *Handler) registerEANSeries(w http.ResponseWriter, r *http.Request) {
	var body api.RegisterEANSeries
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	var description string
	if body.Description != nil {
		description = *body.Description
	}

	ean, err := h.svc.RegisterEANSeries(r.Context(), body.Prefix, body.StartCode, body.EndCode, description)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "RegisterEANSeries failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, ean)
}

// writeJSON encodes data as JSON and writes it to the response writer.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
