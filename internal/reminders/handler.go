package reminders

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

// Handler provides the HTTP handlers for reminders (REG-05).
type Handler struct {
	svc    *Service
	mw     *auth.SessionMiddleware
	logger *slog.Logger
}

// NewHandler creates a new reminders handler.
func NewHandler(svc *Service, mw *auth.SessionMiddleware, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, mw: mw, logger: logger}
}

// ListReminders returns all active reminders (REG-05).
func (h *Handler) ListReminders(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.listReminders)).ServeHTTP(w, r)
}

func (h *Handler) listReminders(w http.ResponseWriter, r *http.Request) {
	reminders, err := h.svc.ListReminders(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, reminders)
}

// CreateReminder creates a new reminder (REG-05).
func (h *Handler) CreateReminder(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.createReminder)).ServeHTTP(w, r)
}

func (h *Handler) createReminder(w http.ResponseWriter, r *http.Request) {
	var body api.Reminder
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	var idsEntryID, externalRefID uuid.UUID
	if body.IdsEntryId != nil {
		idsEntryID = uuid.UUID(*body.IdsEntryId)
	}
	if body.ExternalRefId != nil {
		externalRefID = uuid.UUID(*body.ExternalRefId)
	}

	var externalRefType, titleEn, description string
	if body.ExternalRefType != nil {
		externalRefType = *body.ExternalRefType
	}
	if body.TitleEn != nil {
		titleEn = *body.TitleEn
	}
	if body.Description != nil {
		description = *body.Description
	}

	dueDate := body.DueDate.Time.Format("2006-01-02")

	reminder, err := h.svc.CreateReminder(r.Context(), idsEntryID, externalRefID, string(body.ReminderType), externalRefType, body.Title, titleEn, description, dueDate)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "CreateReminder failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, reminder)
}

// GetReminder returns a reminder by ID.
func (h *Handler) GetReminder(w http.ResponseWriter, r *http.Request, reminderId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.getReminder(w, r, reminderId)
	})).ServeHTTP(w, r)
}

func (h *Handler) getReminder(w http.ResponseWriter, r *http.Request, reminderId openapi_types.UUID) {
	reminder, err := h.svc.GetReminder(r.Context(), uuid.UUID(reminderId))
	if err != nil {
		if errors.Is(err, newNotFound("reminder")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "GetReminder failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, reminder)
}

// UpdateReminder updates a reminder (REG-05).
func (h *Handler) UpdateReminder(w http.ResponseWriter, r *http.Request, reminderId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.updateReminder(w, r, reminderId)
	})).ServeHTTP(w, r)
}

func (h *Handler) updateReminder(w http.ResponseWriter, r *http.Request, reminderId openapi_types.UUID) {
	var body api.Reminder
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	var title, titleEn, description, dueDate *string
	if body.Title != "" {
		title = &body.Title
	}
	if body.TitleEn != nil {
		titleEn = body.TitleEn
	}
	if body.Description != nil {
		description = body.Description
	}
	if body.DueDate.Time.Year() > 0 {
		d := body.DueDate.Time.Format("2006-01-02")
		dueDate = &d
	}

	reminder, err := h.svc.UpdateReminder(r.Context(), title, titleEn, description, dueDate, uuid.UUID(reminderId), int(body.Version))
	if err != nil {
		if errors.Is(err, newNotFound("reminder")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "UpdateReminder failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, reminder)
}

// ArchiveReminder archives a reminder (NFK-09).
func (h *Handler) ArchiveReminder(w http.ResponseWriter, r *http.Request, reminderId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.archiveReminder(w, r, reminderId)
	})).ServeHTTP(w, r)
}

func (h *Handler) archiveReminder(w http.ResponseWriter, r *http.Request, reminderId openapi_types.UUID) {
	var body struct {
		Version int `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	reminder, err := h.svc.ArchiveReminder(r.Context(), uuid.UUID(reminderId), body.Version)
	if err != nil {
		if errors.Is(err, newNotFound("reminder")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "ArchiveReminder failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, reminder)
}

// MarkReminderNotified marks a reminder as notified (REG-05).
func (h *Handler) MarkReminderNotified(w http.ResponseWriter, r *http.Request, reminderId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.markReminderNotified(w, r, reminderId)
	})).ServeHTTP(w, r)
}

func (h *Handler) markReminderNotified(w http.ResponseWriter, r *http.Request, reminderId openapi_types.UUID) {
	var body struct {
		Version int `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	reminder, err := h.svc.MarkReminderNotified(r.Context(), uuid.UUID(reminderId), body.Version)
	if err != nil {
		if errors.Is(err, newNotFound("reminder")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "MarkReminderNotified failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, reminder)
}

// writeJSON encodes data as JSON and writes it to the response writer.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
