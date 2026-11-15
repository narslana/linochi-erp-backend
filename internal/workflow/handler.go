package workflow

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

// Handler provides the HTTP handlers for the workflow engine (WF-01 to WF-07).
type Handler struct {
	svc    *Service
	mw     *auth.SessionMiddleware
	logger *slog.Logger
}

// NewHandler creates a new workflow handler.
func NewHandler(svc *Service, mw *auth.SessionMiddleware, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, mw: mw, logger: logger}
}

// ─── Steps ────────────────────────────────────────────────────────────────────

// ListWorkflowSteps returns all active workflow steps (WF-01).
func (h *Handler) ListWorkflowSteps(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.listWorkflowSteps)).ServeHTTP(w, r)
}

func (h *Handler) listWorkflowSteps(w http.ResponseWriter, r *http.Request) {
	steps, err := h.svc.ListWorkflowSteps(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, steps)
}

// CreateWorkflowStep creates a new workflow step (WF-01).
func (h *Handler) CreateWorkflowStep(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.createWorkflowStep)).ServeHTTP(w, r)
}

func (h *Handler) createWorkflowStep(w http.ResponseWriter, r *http.Request) {
	var body api.WorkflowStep
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	var nameEn string
	if body.NameEn != nil {
		nameEn = *body.NameEn
	}

	isStart := false
	if body.IsStart != nil {
		isStart = *body.IsStart
	}

	isEnd := false
	if body.IsEnd != nil {
		isEnd = *body.IsEnd
	}

	step, err := h.svc.CreateWorkflowStep(r.Context(), body.Key, body.Name, nameEn, body.SortOrder, isStart, isEnd)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "CreateWorkflowStep failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, step)
}

// GetWorkflowStep returns a workflow step by ID.
func (h *Handler) GetWorkflowStep(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.getWorkflowStep(w, r, id)
	})).ServeHTTP(w, r)
}

func (h *Handler) getWorkflowStep(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	step, err := h.svc.GetWorkflowStep(r.Context(), id)
	if err != nil {
		if errors.Is(err, newNotFound("workflow step")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "GetWorkflowStep failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, step)
}

// UpdateWorkflowStep updates a workflow step (WF-01).
func (h *Handler) UpdateWorkflowStep(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.updateWorkflowStep(w, r, id)
	})).ServeHTTP(w, r)
}

func (h *Handler) updateWorkflowStep(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	var body api.WorkflowStep
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	var name, nameEn *string
	if body.Name != "" {
		name = &body.Name
	}
	if body.NameEn != nil {
		nameEn = body.NameEn
	}

	var sortOrder *int
	if body.SortOrder != 0 {
		sortOrder = &body.SortOrder
	}

	var isStart, isEnd *bool
	if body.IsStart != nil {
		isStart = body.IsStart
	}
	if body.IsEnd != nil {
		isEnd = body.IsEnd
	}

	step, err := h.svc.UpdateWorkflowStep(r.Context(), name, nameEn, sortOrder, isStart, isEnd, id, int(body.Version))
	if err != nil {
		if errors.Is(err, newNotFound("workflow step")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "UpdateWorkflowStep failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, step)
}

// ArchiveWorkflowStep archives a workflow step (NFK-09).
func (h *Handler) ArchiveWorkflowStep(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.archiveWorkflowStep(w, r, id)
	})).ServeHTTP(w, r)
}

func (h *Handler) archiveWorkflowStep(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	var body struct {
		Version int `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	step, err := h.svc.ArchiveWorkflowStep(r.Context(), id, body.Version)
	if err != nil {
		if errors.Is(err, newNotFound("workflow step")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "ArchiveWorkflowStep failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, step)
}

// ─── Transitions ──────────────────────────────────────────────────────────────

// ListWorkflowTransitions returns all workflow transitions (WF-02).
func (h *Handler) ListWorkflowTransitions(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.listWorkflowTransitions)).ServeHTTP(w, r)
}

func (h *Handler) listWorkflowTransitions(w http.ResponseWriter, r *http.Request) {
	transitions, err := h.svc.ListWorkflowTransitions(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, transitions)
}

// GetWorkflowTransition returns a transition by ID.
func (h *Handler) GetWorkflowTransition(w http.ResponseWriter, r *http.Request, transitionId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.getWorkflowTransition(w, r, transitionId)
	})).ServeHTTP(w, r)
}

func (h *Handler) getWorkflowTransition(w http.ResponseWriter, r *http.Request, transitionId openapi_types.UUID) {
	transition, err := h.svc.GetWorkflowTransition(r.Context(), uuid.UUID(transitionId))
	if err != nil {
		if errors.Is(err, newNotFound("workflow transition")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "GetWorkflowTransition failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, transition)
}

// CreateWorkflowTransition creates a new workflow transition (WF-02).
func (h *Handler) CreateWorkflowTransition(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.createWorkflowTransition)).ServeHTTP(w, r)
}

func (h *Handler) createWorkflowTransition(w http.ResponseWriter, r *http.Request) {
	var body api.WorkflowTransition
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	var labelEn string
	if body.LabelEn != nil {
		labelEn = *body.LabelEn
	}

	transition, err := h.svc.CreateWorkflowTransition(r.Context(), uuid.Nil, uuid.Nil, uuid.Nil, body.Label, labelEn, body.SortOrder)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "CreateWorkflowTransition failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, transition)
}

// UpdateWorkflowTransition updates a workflow transition (WF-02).
func (h *Handler) UpdateWorkflowTransition(w http.ResponseWriter, r *http.Request, transitionId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.updateWorkflowTransition(w, r, transitionId)
	})).ServeHTTP(w, r)
}

func (h *Handler) updateWorkflowTransition(w http.ResponseWriter, r *http.Request, transitionId openapi_types.UUID) {
	var body api.WorkflowTransition
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	var fromKey, toKey, label, labelEn *string
	if body.FromKey != "" {
		fromKey = &body.FromKey
	}
	if body.ToKey != "" {
		toKey = &body.ToKey
	}
	if body.Label != "" {
		label = &body.Label
	}
	if body.LabelEn != nil {
		labelEn = body.LabelEn
	}

	var sortOrder *int
	if body.SortOrder != 0 {
		sortOrder = &body.SortOrder
	}

	transition, err := h.svc.UpdateWorkflowTransition(r.Context(), fromKey, toKey, label, labelEn, sortOrder, uuid.UUID(transitionId), int(body.Version))
	if err != nil {
		if errors.Is(err, newNotFound("workflow transition")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "UpdateWorkflowTransition failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, transition)
}

// ArchiveWorkflowTransition archives a workflow transition (NFK-09).
func (h *Handler) ArchiveWorkflowTransition(w http.ResponseWriter, r *http.Request, transitionId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.archiveWorkflowTransition(w, r, transitionId)
	})).ServeHTTP(w, r)
}

func (h *Handler) archiveWorkflowTransition(w http.ResponseWriter, r *http.Request, transitionId openapi_types.UUID) {
	var body struct {
		Version int `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	transition, err := h.svc.ArchiveWorkflowTransition(r.Context(), uuid.UUID(transitionId), body.Version)
	if err != nil {
		if errors.Is(err, newNotFound("workflow transition")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "ArchiveWorkflowTransition failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, transition)
}

// ─── Conditions ───────────────────────────────────────────────────────────────

// ListWorkflowConditions returns all active conditions for a transition (WF-03).
func (h *Handler) ListWorkflowConditions(w http.ResponseWriter, r *http.Request, transitionId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.listWorkflowConditions(w, r, transitionId)
	})).ServeHTTP(w, r)
}

func (h *Handler) listWorkflowConditions(w http.ResponseWriter, r *http.Request, transitionId openapi_types.UUID) {
	conditions, err := h.svc.ListWorkflowConditions(r.Context(), uuid.UUID(transitionId))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, conditions)
}

// CreateWorkflowCondition creates a new workflow condition (WF-03).
func (h *Handler) CreateWorkflowCondition(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.createWorkflowCondition)).ServeHTTP(w, r)
}

func (h *Handler) createWorkflowCondition(w http.ResponseWriter, r *http.Request) {
	var body api.WorkflowCondition
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	configBytes, _ := json.Marshal(body.Config)

	condition, err := h.svc.CreateWorkflowCondition(r.Context(), uuid.UUID(body.TransitionId), body.Kind, configBytes)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "CreateWorkflowCondition failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, condition)
}

// ArchiveWorkflowCondition archives a workflow condition (NFK-09).
func (h *Handler) ArchiveWorkflowCondition(w http.ResponseWriter, r *http.Request, transitionId openapi_types.UUID, conditionId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.archiveWorkflowCondition(w, r, transitionId, conditionId)
	})).ServeHTTP(w, r)
}

func (h *Handler) archiveWorkflowCondition(w http.ResponseWriter, r *http.Request, transitionId openapi_types.UUID, conditionId openapi_types.UUID) {
	var body struct {
		Version int `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	condition, err := h.svc.ArchiveWorkflowCondition(r.Context(), uuid.UUID(conditionId), body.Version)
	if err != nil {
		if errors.Is(err, newNotFound("workflow condition")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "ArchiveWorkflowCondition failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, condition)
}

// ─── Effects ──────────────────────────────────────────────────────────────────

// ListWorkflowEffects returns all active effects for a transition (WF-04).
func (h *Handler) ListWorkflowEffects(w http.ResponseWriter, r *http.Request, transitionId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.listWorkflowEffects(w, r, transitionId)
	})).ServeHTTP(w, r)
}

func (h *Handler) listWorkflowEffects(w http.ResponseWriter, r *http.Request, transitionId openapi_types.UUID) {
	effects, err := h.svc.ListWorkflowEffects(r.Context(), uuid.UUID(transitionId))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, effects)
}

// CreateWorkflowEffect creates a new workflow effect (WF-04).
func (h *Handler) CreateWorkflowEffect(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.createWorkflowEffect)).ServeHTTP(w, r)
}

func (h *Handler) createWorkflowEffect(w http.ResponseWriter, r *http.Request) {
	var body api.WorkflowEffect
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	configBytes, _ := json.Marshal(body.Config)

	effect, err := h.svc.CreateWorkflowEffect(r.Context(), uuid.UUID(body.TransitionId), body.Kind, configBytes)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "CreateWorkflowEffect failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, effect)
}

// ArchiveWorkflowEffect archives a workflow effect (NFK-09).
func (h *Handler) ArchiveWorkflowEffect(w http.ResponseWriter, r *http.Request, transitionId openapi_types.UUID, effectId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.archiveWorkflowEffect(w, r, transitionId, effectId)
	})).ServeHTTP(w, r)
}

func (h *Handler) archiveWorkflowEffect(w http.ResponseWriter, r *http.Request, transitionId openapi_types.UUID, effectId openapi_types.UUID) {
	var body struct {
		Version int `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	effect, err := h.svc.ArchiveWorkflowEffect(r.Context(), uuid.UUID(effectId), body.Version)
	if err != nil {
		if errors.Is(err, newNotFound("workflow effect")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "ArchiveWorkflowEffect failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, effect)
}

// ─── Transition-scoped conditions and effects (WF-03, WF-04) ─────────────────

// ListWorkflowTransitionConditions returns all active conditions for a transition (WF-03).
func (h *Handler) ListWorkflowTransitionConditions(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.listWorkflowTransitionConditions(w, r, id)
	})).ServeHTTP(w, r)
}

func (h *Handler) listWorkflowTransitionConditions(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	conditions, err := h.svc.ListWorkflowConditions(r.Context(), uuid.UUID(id))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, conditions)
}

// CreateWorkflowTransitionCondition adds a condition to a transition (WF-03).
func (h *Handler) CreateWorkflowTransitionCondition(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.createWorkflowTransitionCondition(w, r, id)
	})).ServeHTTP(w, r)
}

func (h *Handler) createWorkflowTransitionCondition(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	var body api.CreateWorkflowCondition
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	configBytes, _ := json.Marshal(body.Config)

	condition, err := h.svc.CreateWorkflowCondition(r.Context(), uuid.UUID(id), body.Kind, configBytes)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "CreateWorkflowTransitionCondition failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, condition)
}

// ListWorkflowTransitionEffects returns all active effects for a transition (WF-04).
func (h *Handler) ListWorkflowTransitionEffects(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.listWorkflowTransitionEffects(w, r, id)
	})).ServeHTTP(w, r)
}

func (h *Handler) listWorkflowTransitionEffects(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	effects, err := h.svc.ListWorkflowEffects(r.Context(), uuid.UUID(id))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, effects)
}

// CreateWorkflowTransitionEffect adds an effect to a transition (WF-04).
func (h *Handler) CreateWorkflowTransitionEffect(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.createWorkflowTransitionEffect(w, r, id)
	})).ServeHTTP(w, r)
}

func (h *Handler) createWorkflowTransitionEffect(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	var body api.CreateWorkflowEffect
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	configBytes, _ := json.Marshal(body.Config)

	effect, err := h.svc.CreateWorkflowEffect(r.Context(), uuid.UUID(id), body.Kind, configBytes)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "CreateWorkflowTransitionEffect failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, effect)
}

// writeJSON encodes data as JSON and writes it to the response writer.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
