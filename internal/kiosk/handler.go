package kiosk

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

// Handler provides the HTTP handlers for the kiosk module (KIO-01 to KIO-07).
type Handler struct {
	svc    *Service
	mw     *auth.SessionMiddleware
	logger *slog.Logger
}

// NewHandler creates a new kiosk handler.
func NewHandler(svc *Service, mw *auth.SessionMiddleware, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, logger: logger, mw: mw}
}

// ─── Playlists ────────────────────────────────────────────────────────────────

func (h *Handler) ListKioskPlaylists(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.listKioskPlaylists)).ServeHTTP(w, r)
}

func (h *Handler) listKioskPlaylists(w http.ResponseWriter, r *http.Request) {
	playlists, err := h.svc.ListKioskPlaylists(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, playlists)
}

func (h *Handler) CreateKioskPlaylist(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.createKioskPlaylist)).ServeHTTP(w, r)
}

func (h *Handler) createKioskPlaylist(w http.ResponseWriter, r *http.Request) {
	var body api.CreateKioskPlaylistJSONBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	var nameEn string
	if body.NameEn != nil {
		nameEn = *body.NameEn
	}

	p, err := h.svc.CreateKioskPlaylist(r.Context(), body.Name, nameEn)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "CreateKioskPlaylist failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (h *Handler) GetKioskPlaylist(w http.ResponseWriter, r *http.Request, playlistId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.getKioskPlaylist(w, r, playlistId)
	})).ServeHTTP(w, r)
}

func (h *Handler) getKioskPlaylist(w http.ResponseWriter, r *http.Request, playlistId openapi_types.UUID) {
	p, err := h.svc.GetKioskPlaylist(r.Context(), uuid.UUID(playlistId))
	if err != nil {
		if errors.Is(err, newNotFound("playlist")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "GetKioskPlaylist failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *Handler) UpdateKioskPlaylist(w http.ResponseWriter, r *http.Request, playlistId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.updateKioskPlaylist(w, r, playlistId)
	})).ServeHTTP(w, r)
}

func (h *Handler) updateKioskPlaylist(w http.ResponseWriter, r *http.Request, playlistId openapi_types.UUID) {
	var body api.UpdateKioskPlaylistJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	var name, nameEn *string
	if body.Name != nil {
		name = body.Name
	}
	if body.NameEn != nil {
		nameEn = body.NameEn
	}

	p, err := h.svc.UpdateKioskPlaylist(r.Context(), uuid.UUID(playlistId), name, nameEn, *body.Version)
	if err != nil {
		if errors.Is(err, newNotFound("playlist")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "UpdateKioskPlaylist failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *Handler) ArchiveKioskPlaylist(w http.ResponseWriter, r *http.Request, playlistId openapi_types.UUID, params api.ArchiveKioskPlaylistParams) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.archiveKioskPlaylist(w, r, playlistId, params.Version)
	})).ServeHTTP(w, r)
}

func (h *Handler) archiveKioskPlaylist(w http.ResponseWriter, r *http.Request, playlistId openapi_types.UUID, version int) {
	p, err := h.svc.ArchiveKioskPlaylist(r.Context(), uuid.UUID(playlistId), version)
	if err != nil {
		if errors.Is(err, newNotFound("playlist")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "ArchiveKioskPlaylist failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// ─── Playlist Views ───────────────────────────────────────────────────────────

func (h *Handler) ListPlaylistViews(w http.ResponseWriter, r *http.Request, playlistId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.listPlaylistViews(w, r, playlistId)
	})).ServeHTTP(w, r)
}

func (h *Handler) listPlaylistViews(w http.ResponseWriter, r *http.Request, playlistId openapi_types.UUID) {
	views, err := h.svc.ListPlaylistViews(r.Context(), uuid.UUID(playlistId))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, views)
}

func (h *Handler) CreatePlaylistView(w http.ResponseWriter, r *http.Request, playlistId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.createPlaylistView(w, r, playlistId)
	})).ServeHTTP(w, r)
}

func (h *Handler) createPlaylistView(w http.ResponseWriter, r *http.Request, playlistId openapi_types.UUID) {
	var body api.CreatePlaylistViewJSONBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	var settings map[string]any
	if body.Settings != nil {
		settings = *body.Settings
	}

	sortOrder := 0
	if body.SortOrder != nil {
		sortOrder = int(*body.SortOrder)
	}

	durationSec := 30
	if body.DurationSec != nil {
		durationSec = int(*body.DurationSec)
	}

	v, err := h.svc.CreatePlaylistView(r.Context(), uuid.UUID(playlistId), string(body.ViewType), sortOrder, durationSec, settings)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "CreatePlaylistView failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (h *Handler) UpdatePlaylistView(w http.ResponseWriter, r *http.Request, playlistId openapi_types.UUID, viewId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.updatePlaylistView(w, r, viewId)
	})).ServeHTTP(w, r)
}

func (h *Handler) updatePlaylistView(w http.ResponseWriter, r *http.Request, viewId openapi_types.UUID) {
	var body api.UpdatePlaylistViewJSONBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	var vType *string
	if body.ViewType != nil {
		v := string(*body.ViewType)
		vType = &v
	}

	var sOrder, dSec *int
	if body.SortOrder != nil {
		v := int(*body.SortOrder)
		sOrder = &v
	}
	if body.DurationSec != nil {
		v := int(*body.DurationSec)
		dSec = &v
	}

	var settings *map[string]any
	if body.Settings != nil {
		settings = body.Settings
	}

	v, err := h.svc.UpdatePlaylistView(r.Context(), uuid.UUID(viewId), vType, sOrder, dSec, settings, *body.Version)
	if err != nil {
		if errors.Is(err, newNotFound("playlist_view")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "UpdatePlaylistView failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handler) ArchivePlaylistView(w http.ResponseWriter, r *http.Request, playlistId openapi_types.UUID, viewId openapi_types.UUID, params api.ArchivePlaylistViewParams) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.archivePlaylistView(w, r, viewId, params.Version)
	})).ServeHTTP(w, r)
}

func (h *Handler) archivePlaylistView(w http.ResponseWriter, r *http.Request, viewId openapi_types.UUID, version int) {
	v, err := h.svc.ArchivePlaylistView(r.Context(), uuid.UUID(viewId), version)
	if err != nil {
		if errors.Is(err, newNotFound("playlist_view")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "ArchivePlaylistView failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// ─── Devices ──────────────────────────────────────────────────────────────────

func (h *Handler) ListKioskDevices(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.listKioskDevices)).ServeHTTP(w, r)
}

func (h *Handler) listKioskDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := h.svc.ListKioskDevices(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, devices)
}

func (h *Handler) CreateKioskDevice(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.createKioskDevice)).ServeHTTP(w, r)
}

func (h *Handler) createKioskDevice(w http.ResponseWriter, r *http.Request) {
	var body api.CreateKioskDeviceJSONBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	d, err := h.svc.CreateKioskDevice(r.Context(), body.Name)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "CreateKioskDevice failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

func (h *Handler) GetKioskDevice(w http.ResponseWriter, r *http.Request, deviceId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.getKioskDevice(w, r, deviceId)
	})).ServeHTTP(w, r)
}

func (h *Handler) getKioskDevice(w http.ResponseWriter, r *http.Request, deviceId openapi_types.UUID) {
	d, err := h.svc.GetKioskDevice(r.Context(), uuid.UUID(deviceId))
	if err != nil {
		if errors.Is(err, newNotFound("device")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "GetKioskDevice failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *Handler) UpdateKioskDevice(w http.ResponseWriter, r *http.Request, deviceId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.updateKioskDevice(w, r, deviceId)
	})).ServeHTTP(w, r)
}

func (h *Handler) updateKioskDevice(w http.ResponseWriter, r *http.Request, deviceId openapi_types.UUID) {
	var body api.UpdateKioskDeviceJSONBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	var playlistID *string
	if body.PlaylistId != nil {
		s := uuid.UUID(*body.PlaylistId).String()
		playlistID = &s
	}

	d, err := h.svc.ApproveKioskDevice(r.Context(), uuid.UUID(deviceId), playlistID, *body.Version)
	if err != nil {
		if errors.Is(err, newNotFound("device")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "UpdateKioskDevice failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *Handler) ArchiveKioskDevice(w http.ResponseWriter, r *http.Request, deviceId openapi_types.UUID, params api.ArchiveKioskDeviceParams) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.archiveKioskDevice(w, r, deviceId, params.Version)
	})).ServeHTTP(w, r)
}

func (h *Handler) archiveKioskDevice(w http.ResponseWriter, r *http.Request, deviceId openapi_types.UUID, version int) {
	d, err := h.svc.ArchiveKioskDevice(r.Context(), uuid.UUID(deviceId), version)
	if err != nil {
		if errors.Is(err, newNotFound("device")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "ArchiveKioskDevice failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// ─── Kiosk Public Endpoints ───────────────────────────────────────────────────

// PairKioskDevice pairs a device with its pairing code (KIO-06).
func (h *Handler) PairKioskDevice(w http.ResponseWriter, r *http.Request, params api.PairKioskDeviceParams) {
	pairingCode := params.PairingCode

	var body api.PairKioskDeviceJSONBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	d, err := h.svc.ValidateKioskAPIKey(r.Context(), body.ApiKey, pairingCode)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}

	if !d.Approved {
		httpx.Error(w, r, errors.New("device not approved"))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"message": "paired"})
}

// GetKioskData returns aggregated data for kiosk views (KIO-02, KIO-03, KIO-05).
func (h *Handler) GetKioskData(w http.ResponseWriter, r *http.Request, params api.GetKioskDataParams) {
	periodMonths := 12
	if params.PeriodMonths != nil {
		periodMonths = int(*params.PeriodMonths)
	}

	data, err := h.svc.GetKioskData(r.Context(), periodMonths)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}

// writeJSON encodes data as JSON and writes it to the response writer.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
