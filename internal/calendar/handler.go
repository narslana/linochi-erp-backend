package calendar

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

// Handler provides the HTTP handlers for the calendar module (KAL-01 to KAL-04).
type Handler struct {
	svc    *Service
	mw     *auth.SessionMiddleware
	logger *slog.Logger
}

// NewHandler creates a new calendar handler.
func NewHandler(svc *Service, mw *auth.SessionMiddleware, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, mw: mw, logger: logger}
}

// ─── iCal Sources ─────────────────────────────────────────────────────────────

// ListICalSources returns all active iCal sources (KAL-01).
func (h *Handler) ListICalSources(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.listICalSources)).ServeHTTP(w, r)
}

func (h *Handler) listICalSources(w http.ResponseWriter, r *http.Request) {
	sources, err := h.svc.ListICalSources(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, sources)
}

// CreateICalSource creates a new iCal source (KAL-01).
func (h *Handler) CreateICalSource(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.createICalSource)).ServeHTTP(w, r)
}

func (h *Handler) createICalSource(w http.ResponseWriter, r *http.Request) {
	var body api.CreateICalSourceJSONBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	var nameEn string
	if body.NameEn != nil {
		nameEn = *body.NameEn
	}
	color := ""
	if body.Color != nil {
		color = *body.Color
	}
	syncInterval := 15
	if body.SyncInterval != nil {
		syncInterval = int(*body.SyncInterval)
	}

	src, err := h.svc.CreateICalSource(r.Context(), body.Name, nameEn, body.IcalUrl, color, syncInterval)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "CreateICalSource failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, src)
}

// GetICalSource returns an iCal source by ID (KAL-01).
func (h *Handler) GetICalSource(w http.ResponseWriter, r *http.Request, sourceId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.getICalSource(w, r, sourceId)
	})).ServeHTTP(w, r)
}

func (h *Handler) getICalSource(w http.ResponseWriter, r *http.Request, sourceId openapi_types.UUID) {
	src, err := h.svc.GetICalSource(r.Context(), uuid.UUID(sourceId))
	if err != nil {
		if errors.Is(err, newNotFound("ical_source")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "GetICalSource failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, src)
}

// UpdateICalSource updates an iCal source (KAL-01).
func (h *Handler) UpdateICalSource(w http.ResponseWriter, r *http.Request, sourceId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.updateICalSource(w, r, sourceId)
	})).ServeHTTP(w, r)
}

func (h *Handler) updateICalSource(w http.ResponseWriter, r *http.Request, sourceId openapi_types.UUID) {
	var body api.UpdateICalSourceJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	var name, nameEn, icalURL, color *string
	if body.Name != nil {
		name = body.Name
	}
	if body.NameEn != nil {
		nameEn = body.NameEn
	}
	if body.IcalUrl != nil {
		icalURL = body.IcalUrl
	}
	if body.Color != nil {
		color = body.Color
	}

	var syncInterval *int
	if body.SyncInterval != nil {
		v := int(*body.SyncInterval)
		syncInterval = &v
	}

	var enabled *bool
	if body.Enabled != nil {
		enabled = body.Enabled
	}

	src, err := h.svc.UpdateICalSource(r.Context(), uuid.UUID(sourceId), name, nameEn, icalURL, color, syncInterval, enabled, *body.Version)
	if err != nil {
		if errors.Is(err, newNotFound("ical_source")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "UpdateICalSource failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, src)
}

// ArchiveICalSource archives an iCal source (NFK-09).
func (h *Handler) ArchiveICalSource(w http.ResponseWriter, r *http.Request, sourceId openapi_types.UUID, params api.ArchiveICalSourceParams) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.archiveICalSource(w, r, sourceId, params.Version)
	})).ServeHTTP(w, r)
}

func (h *Handler) archiveICalSource(w http.ResponseWriter, r *http.Request, sourceId openapi_types.UUID, version int) {
	src, err := h.svc.ArchiveICalSource(r.Context(), uuid.UUID(sourceId), version)
	if err != nil {
		if errors.Is(err, newNotFound("ical_source")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "ArchiveICalSource failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, src)
}

// ─── Calendar Events ──────────────────────────────────────────────────────────

// ListCalendarEvents returns events in a time range (KAL-02, KAL-03).
func (h *Handler) ListCalendarEvents(w http.ResponseWriter, r *http.Request, params api.ListCalendarEventsParams) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.listCalendarEvents(w, r, params)
	})).ServeHTTP(w, r)
}

func (h *Handler) listCalendarEvents(w http.ResponseWriter, r *http.Request, params api.ListCalendarEventsParams) {
	events, err := h.svc.ListCalendarEvents(r.Context(), params.StartTime, params.EndTime)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, events)
}

// GetCalendarEvent returns a single event (KAL-04).
func (h *Handler) GetCalendarEvent(w http.ResponseWriter, r *http.Request, eventId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.getCalendarEvent(w, r, eventId)
	})).ServeHTTP(w, r)
}

func (h *Handler) getCalendarEvent(w http.ResponseWriter, r *http.Request, eventId openapi_types.UUID) {
	ev, err := h.svc.GetCalendarEvent(r.Context(), uuid.UUID(eventId))
	if err != nil {
		if errors.Is(err, newNotFound("event")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "GetCalendarEvent failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ev)
}

// SyncCalendar triggers a sync of all enabled iCal sources (KAL-01).
func (h *Handler) SyncCalendar(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.syncCalendar)).ServeHTTP(w, r)
}

func (h *Handler) syncCalendar(w http.ResponseWriter, r *http.Request) {
	synced, errs := h.svc.SyncICalSources(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"synced": synced,
		"errors": errs,
	})
}

// writeJSON encodes data as JSON and writes it to the response writer.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
