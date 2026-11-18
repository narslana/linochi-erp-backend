package workflow

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/auth"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/httpx"
)

// Handler provides the HTTP handlers for flow versions (WF-01, WF-06, WF-07).
// The status endpoints are in the orders handler: they are order routes.
type Handler struct {
	svc    *Service
	mw     *auth.SessionMiddleware
	logger *slog.Logger
}

func NewHandler(svc *Service, mw *auth.SessionMiddleware, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, mw: mw, logger: logger}
}

func (h *Handler) ListWorkflowVersions(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vs, err := h.svc.ListVersions(r.Context())
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, vs)
	})).ServeHTTP(w, r)
}

// PublishWorkflowVersion publishes a new version. Admin only: the flow decides
// what happens to stock for every order.
func (h *Handler) PublishWorkflowVersion(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sess, ok := auth.SessionFrom(r.Context()); !ok || !hasAdmin(sess.Roles) {
			httpx.Error(w, r, &ForbiddenError{})
			return
		}
		var body VersionDefinition
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.Error(w, r, err)
			return
		}
		v, err := h.svc.PublishVersion(r.Context(), body)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, v)
	})).ServeHTTP(w, r)
}

func (h *Handler) GetActiveWorkflowVersion(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v, err := h.svc.ActiveVersion(r.Context())
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, v)
	})).ServeHTTP(w, r)
}

func (h *Handler) GetWorkflowVersion(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v, err := h.svc.GetVersion(r.Context(), uuid.UUID(id))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, v)
	})).ServeHTTP(w, r)
}

func hasAdmin(roles []string) bool {
	for _, r := range roles {
		if r == auth.RoleAdmin {
			return true
		}
	}
	return false
}

// ActorFromRequest builds the engine's actor from the session the middleware
// put in the context. ok is false without a session.
func ActorFromRequest(r *http.Request) (Actor, bool) {
	sess, ok := auth.SessionFrom(r.Context())
	if !ok {
		return Actor{}, false
	}
	return Actor{UserID: sess.UserID, Admin: hasAdmin(sess.Roles)}, true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
