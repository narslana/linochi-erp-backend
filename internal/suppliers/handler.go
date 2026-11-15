package suppliers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/api"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/auth"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/httpx"
)

// Handler implements api.ServerInterface for the suppliers module.
type Handler struct {
	svc    *Service
	mw     *auth.SessionMiddleware
	logger *slog.Logger
}

// NewHandler creates a suppliers handler.
func NewHandler(svc *Service, mw *auth.SessionMiddleware, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, mw: mw, logger: logger}
}

// ─── Suppliers ────────────────────────────────────────────────────────────────

func (h *Handler) ListSuppliers(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.listSuppliers)).ServeHTTP(w, r)
}

func (h *Handler) listSuppliers(w http.ResponseWriter, r *http.Request) {
	suppliers, err := h.svc.ListSuppliers(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, suppliers)
}

func (h *Handler) CreateSupplier(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.createSupplier)).ServeHTTP(w, r)
}

func (h *Handler) createSupplier(w http.ResponseWriter, r *http.Request) {
	var body api.CreateSupplierJSONBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteProblem(w, r, httpx.Problem{Code: "invalid-request", Title: "Invalid request body", Status: http.StatusBadRequest})
		return
	}

	var role string
	if body.Role != nil {
		role = string(*body.Role)
	}
	var leadTimeDays, moq int
	if body.LeadTimeDays != nil {
		leadTimeDays = int(*body.LeadTimeDays)
	}
	if body.Moq != nil {
		moq = int(*body.Moq)
	}

	currency := "SEK"
	if body.Currency != nil {
		currency = string(*body.Currency)
	}

	sup, err := h.svc.CreateSupplier(r.Context(),
		body.Name, stringOr(body.NameEn, ""),
		stringOr(body.OrgNumber, ""), stringOr(body.VatNumber, ""),
		stringOr(body.Country, ""), stringOr(body.Address, ""),
		stringOr(body.City, ""), stringOr(body.PostalCode, ""),
		stringOr(body.Website, ""), stringOr(body.ContactEmail, ""),
		stringOr(body.ContactPhone, ""), role,
		leadTimeDays, moq, currency,
		stringOr(body.PaymentTerms, ""), stringOr(body.DeliveryTerms, ""),
		stringOr(body.Notes, ""),
	)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/suppliers/"+sup.ID.String())
	writeJSON(w, http.StatusCreated, sup)
}

func (h *Handler) GetSupplier(w http.ResponseWriter, r *http.Request, supplierId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sup, err := h.svc.GetSupplier(r.Context(), uuid.UUID(supplierId))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, sup)
	})).ServeHTTP(w, r)
}

func (h *Handler) UpdateSupplier(w http.ResponseWriter, r *http.Request, supplierId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body api.UpdateSupplierJSONBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, httpx.Problem{Code: "invalid-request", Title: "Invalid request body", Status: http.StatusBadRequest})
			return
		}

		var version int
		if body.Version != 0 {
			version = body.Version
		} else {
			sup, err := h.svc.GetSupplier(r.Context(), uuid.UUID(supplierId))
			if err != nil {
				httpx.Error(w, r, err)
				return
			}
			version = sup.Version
		}

		var name, nameEn, orgNumber, vatNumber, country, address, city, postalCode, website, contactEmail, contactPhone, role, paymentTerms, deliveryTerms, notes string
		if body.Name != nil {
			name = *body.Name
		}
		if body.NameEn != nil {
			nameEn = *body.NameEn
		}
		if body.OrgNumber != nil {
			orgNumber = *body.OrgNumber
		}
		if body.VatNumber != nil {
			vatNumber = *body.VatNumber
		}
		if body.Country != nil {
			country = *body.Country
		}
		if body.Address != nil {
			address = *body.Address
		}
		if body.City != nil {
			city = *body.City
		}
		if body.PostalCode != nil {
			postalCode = *body.PostalCode
		}
		if body.Website != nil {
			website = *body.Website
		}
		if body.ContactEmail != nil {
			contactEmail = *body.ContactEmail
		}
		if body.ContactPhone != nil {
			contactPhone = *body.ContactPhone
		}
		if body.Role != nil {
			role = string(*body.Role)
		}
		if body.PaymentTerms != nil {
			paymentTerms = *body.PaymentTerms
		}
		if body.DeliveryTerms != nil {
			deliveryTerms = *body.DeliveryTerms
		}
		if body.Notes != nil {
			notes = *body.Notes
		}

		var leadTimeDays, moq *int
		if body.LeadTimeDays != nil {
			v := int(*body.LeadTimeDays)
			leadTimeDays = &v
		}
		if body.Moq != nil {
			v := int(*body.Moq)
			moq = &v
		}

		sup, err := h.svc.UpdateSupplier(r.Context(), uuid.UUID(supplierId), name, nameEn, orgNumber, vatNumber, country, address, city, postalCode, website, contactEmail, contactPhone, role, paymentTerms, deliveryTerms, notes, leadTimeDays, moq, version)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, sup)
	})).ServeHTTP(w, r)
}

func (h *Handler) ArchiveSupplier(w http.ResponseWriter, r *http.Request, supplierId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sup, err := h.svc.GetSupplier(r.Context(), uuid.UUID(supplierId))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		archived, err := h.svc.ArchiveSupplier(r.Context(), uuid.UUID(supplierId), sup.Version)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, archived)
	})).ServeHTTP(w, r)
}

// ─── Contacts ─────────────────────────────────────────────────────────────────

func (h *Handler) ListSupplierContacts(w http.ResponseWriter, r *http.Request, supplierId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contacts, err := h.svc.ListContacts(r.Context(), uuid.UUID(supplierId))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, contacts)
	})).ServeHTTP(w, r)
}

func (h *Handler) CreateSupplierContact(w http.ResponseWriter, r *http.Request, supplierId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body api.CreateSupplierContactJSONBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, httpx.Problem{Code: "invalid-request", Title: "Invalid request body", Status: http.StatusBadRequest})
			return
		}

		contact, err := h.svc.CreateContact(r.Context(), uuid.UUID(supplierId), uuid.New(),
			body.Name, stringOr(body.NameEn, ""),
			stringOr(body.Email, ""), stringOr(body.Phone, ""), stringOr(body.Role, ""))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.Header().Set("Location", "/api/v1/suppliers/"+supplierId.String()+"/contacts/"+contact.ID.String())
		writeJSON(w, http.StatusCreated, contact)
	})).ServeHTTP(w, r)
}

func (h *Handler) UpdateSupplierContact(w http.ResponseWriter, r *http.Request, supplierId openapi_types.UUID, contactId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body api.UpdateSupplierContactJSONBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, httpx.Problem{Code: "invalid-request", Title: "Invalid request body", Status: http.StatusBadRequest})
			return
		}

		var version int
		if body.Version != 0 {
			version = body.Version
		} else {
			c, err := h.svc.ListContacts(r.Context(), uuid.UUID(supplierId))
			if err != nil {
				httpx.Error(w, r, err)
				return
			}
			for _, ci := range c {
				if ci.ID == uuid.UUID(contactId) {
					version = ci.Version
					break
				}
			}
			if version == 0 {
				httpx.WriteProblem(w, r, httpx.Problem{Code: "not-found", Title: "Contact not found", Status: http.StatusNotFound})
				return
			}
		}

		var name, nameEn, email, phone, role string
		if body.Name != nil {
			name = *body.Name
		}
		if body.NameEn != nil {
			nameEn = *body.NameEn
		}
		if body.Email != nil {
			email = *body.Email
		}
		if body.Phone != nil {
			phone = *body.Phone
		}
		if body.Role != nil {
			role = *body.Role
		}

		contact, err := h.svc.UpdateContact(r.Context(), uuid.UUID(contactId), name, nameEn, email, phone, role, version)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, contact)
	})).ServeHTTP(w, r)
}

func (h *Handler) ArchiveSupplierContact(w http.ResponseWriter, r *http.Request, supplierId openapi_types.UUID, contactId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contacts, err := h.svc.ListContacts(r.Context(), uuid.UUID(supplierId))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		var version int
		for _, c := range contacts {
			if c.ID == uuid.UUID(contactId) {
				version = c.Version
				break
			}
		}
		if version == 0 {
			httpx.WriteProblem(w, r, httpx.Problem{Code: "not-found", Title: "Contact not found", Status: http.StatusNotFound})
			return
		}
		archived, err := h.svc.ArchiveContact(r.Context(), uuid.UUID(contactId), version)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, archived)
	})).ServeHTTP(w, r)
}

// ─── Supplier Products ────────────────────────────────────────────────────────

func (h *Handler) ListSupplierProducts(w http.ResponseWriter, r *http.Request, supplierId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		products, err := h.svc.ListSupplierProducts(r.Context(), uuid.UUID(supplierId))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, products)
	})).ServeHTTP(w, r)
}

func (h *Handler) CreateSupplierProduct(w http.ResponseWriter, r *http.Request, supplierId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body api.CreateSupplierProductJSONBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, httpx.Problem{Code: "invalid-request", Title: "Invalid request body", Status: http.StatusBadRequest})
			return
		}

		var purchaseCurrency string
		if body.PurchaseCurrency != nil {
			purchaseCurrency = string(*body.PurchaseCurrency)
		} else {
			purchaseCurrency = "SEK"
		}

		var purchasePrice int64
		if body.PurchasePrice != nil {
			purchasePrice = int64(*body.PurchasePrice)
		}

		prod, err := h.svc.CreateSupplierProduct(r.Context(), uuid.UUID(supplierId), uuid.UUID(body.ProductId), purchasePrice, purchaseCurrency)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.Header().Set("Location", "/api/v1/suppliers/"+supplierId.String()+"/products/"+prod.ID.String())
		writeJSON(w, http.StatusCreated, prod)
	})).ServeHTTP(w, r)
}

func (h *Handler) ArchiveSupplierProduct(w http.ResponseWriter, r *http.Request, supplierId openapi_types.UUID, spId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		products, err := h.svc.ListSupplierProducts(r.Context(), uuid.UUID(supplierId))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		var version int
		for _, p := range products {
			if p.ID == uuid.UUID(spId) {
				version = p.Version
				break
			}
		}
		if version == 0 {
			httpx.WriteProblem(w, r, httpx.Problem{Code: "not-found", Title: "Supplier product not found", Status: http.StatusNotFound})
			return
		}
		archived, err := h.svc.ArchiveSupplierProduct(r.Context(), uuid.UUID(spId), version)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, archived)
	})).ServeHTTP(w, r)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func stringOr(s *string, def string) string {
	if s != nil {
		return *s
	}
	return def
}
