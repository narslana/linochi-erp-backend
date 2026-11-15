package ids

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/api"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/auth"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/httpx"
)

// Handler implements api.ServerInterface for the ids module.
type Handler struct {
	svc    *Service
	mw     *auth.SessionMiddleware
	logger *slog.Logger
}

// NewHandler creates an ids handler.
func NewHandler(svc *Service, mw *auth.SessionMiddleware, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, mw: mw, logger: logger}
}

func (h *Handler) ListIdsEntries(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.listIdsEntries)).ServeHTTP(w, r)
}

func (h *Handler) listIdsEntries(w http.ResponseWriter, r *http.Request) {
	entries, err := h.svc.ListIdsEntries(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (h *Handler) CreateIdsEntry(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.createIdsEntry)).ServeHTTP(w, r)
}

func (h *Handler) createIdsEntry(w http.ResponseWriter, r *http.Request) {
	var body api.CreateIdsEntryJSONBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteProblem(w, r, httpx.Problem{Code: "invalid-request", Title: "Invalid request body", Status: http.StatusBadRequest})
		return
	}

	var validFrom, validTo *time.Time
	if body.ValidFrom != nil {
		if t, err := time.Parse("2006-01-02", body.ValidFrom.Format("2006-01-02")); err == nil {
			validFrom = &t
		}
	}
	if body.ValidTo != nil {
		if t, err := time.Parse("2006-01-02", body.ValidTo.Format("2006-01-02")); err == nil {
			validTo = &t
		}
	}

	var documentURL, issuer, owner, note string
	if body.DocumentUrl != nil {
		documentURL = *body.DocumentUrl
	}
	if body.Issuer != nil {
		issuer = *body.Issuer
	}
	if body.Owner != nil {
		owner = *body.Owner
	}
	if body.Note != nil {
		note = *body.Note
	}

	e, err := h.svc.CreateIdsEntry(r.Context(), body.Category, body.Name, body.Value, issuer, owner, note, documentURL, validFrom, validTo)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/ids/"+e.ID.String())
	writeJSON(w, http.StatusCreated, e)
}

func (h *Handler) GetIdsEntry(w http.ResponseWriter, r *http.Request, entryId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e, err := h.svc.GetIdsEntry(r.Context(), uuid.UUID(entryId))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, e)
	})).ServeHTTP(w, r)
}

func (h *Handler) UpdateIdsEntry(w http.ResponseWriter, r *http.Request, entryId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body api.UpdateIdsEntryJSONBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, httpx.Problem{Code: "invalid-request", Title: "Invalid request body", Status: http.StatusBadRequest})
			return
		}

		var validFrom, validTo *time.Time
		if body.ValidFrom != nil {
			if t, err := time.Parse("2006-01-02", body.ValidFrom.Format("2006-01-02")); err == nil {
				validFrom = &t
			}
		}
		if body.ValidTo != nil {
			if t, err := time.Parse("2006-01-02", body.ValidTo.Format("2006-01-02")); err == nil {
				validTo = &t
			}
		}

		var documentURL, issuer, owner, note string
		if body.DocumentUrl != nil {
			documentURL = *body.DocumentUrl
		}
		if body.Issuer != nil {
			issuer = *body.Issuer
		}
		if body.Owner != nil {
			owner = *body.Owner
		}
		if body.Note != nil {
			note = *body.Note
		}

		category := ""
		if body.Category != nil {
			category = *body.Category
		}
		name := ""
		if body.Name != nil {
			name = *body.Name
		}
		value := ""
		if body.Value != nil {
			value = *body.Value
		}

		e, err := h.svc.UpdateIdsEntry(r.Context(), uuid.UUID(entryId), category, name, value, issuer, owner, note, documentURL, validFrom, validTo, body.Version)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, e)
	})).ServeHTTP(w, r)
}

func (h *Handler) ArchiveIdsEntry(w http.ResponseWriter, r *http.Request, entryId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e, err := h.svc.ArchiveIdsEntry(r.Context(), uuid.UUID(entryId), 0)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, e)
	})).ServeHTTP(w, r)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
