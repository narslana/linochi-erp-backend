package shipments

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

// Handler implements api.ServerInterface for the shipments module.
type Handler struct {
	svc    *Service
	mw     *auth.SessionMiddleware
	logger *slog.Logger
}

// NewHandler creates a shipments handler.
func NewHandler(svc *Service, mw *auth.SessionMiddleware, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, mw: mw, logger: logger}
}

// ─── Shipments ────────────────────────────────────────────────────────────────

func (h *Handler) ListShipments(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		shipments, err := h.svc.ListShipments(r.Context(), nil, nil)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, shipments)
	})).ServeHTTP(w, r)
}

func (h *Handler) CreateShipment(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.createShipment)).ServeHTTP(w, r)
}

func (h *Handler) createShipment(w http.ResponseWriter, r *http.Request) {
	var body api.CreateShipmentJSONBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteProblem(w, r, httpx.Problem{Code: "invalid-request", Title: "Invalid request body", Status: http.StatusBadRequest})
		return
	}

	var status string
	if body.Status != nil {
		status = string(*body.Status)
	} else {
		status = "planned"
	}

	var shippedAt, estimatedArrival, actualArrival *time.Time
	if body.ShippedAt != nil {
		shippedAt = body.ShippedAt
	}
	if body.EstimatedArrival != nil {
		t, _ := time.Parse("2006-01-02", body.EstimatedArrival.Format("2006-01-02"))
		estimatedArrival = &t
	}
	if body.ActualArrival != nil {
		t, _ := time.Parse("2006-01-02", body.ActualArrival.Format("2006-01-02"))
		actualArrival = &t
	}

	batchNumber := stringOr(body.BatchNumber, "")
	fromLocation := stringOr(body.FromLocation, "")
	toLocation := stringOr(body.ToLocation, "")
	carrier := stringOr(body.Carrier, "")
	trackingNumber := stringOr(body.TrackingNumber, "")

	sh, err := h.svc.CreateShipment(r.Context(), uuid.UUID(body.PurchaseOrderId),
		fromLocation, toLocation, carrier, trackingNumber, batchNumber, status,
		shippedAt, estimatedArrival, actualArrival, nil)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/shipments/"+sh.ID.String())
	writeJSON(w, http.StatusCreated, sh)
}

func (h *Handler) GetShipment(w http.ResponseWriter, r *http.Request, shipmentId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sh, err := h.svc.GetShipment(r.Context(), uuid.UUID(shipmentId))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, sh)
	})).ServeHTTP(w, r)
}

func (h *Handler) UpdateShipment(w http.ResponseWriter, r *http.Request, shipmentId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body api.UpdateShipmentJSONBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, httpx.Problem{Code: "invalid-request", Title: "Invalid request body", Status: http.StatusBadRequest})
			return
		}

		version := body.Version

		var fromLocation, toLocation, carrier, trackingNumber, batchNumber, status *string
		if body.FromLocation != nil {
			v := *body.FromLocation
			fromLocation = &v
		}
		if body.ToLocation != nil {
			v := *body.ToLocation
			toLocation = &v
		}
		if body.Carrier != nil {
			v := *body.Carrier
			carrier = &v
		}
		if body.TrackingNumber != nil {
			v := *body.TrackingNumber
			trackingNumber = &v
		}
		if body.BatchNumber != nil {
			v := *body.BatchNumber
			batchNumber = &v
		}
		if body.Status != nil {
			v := string(*body.Status)
			status = &v
		}

		var shippedAt, estimatedArrival, actualArrival *time.Time
		if body.ShippedAt != nil {
			shippedAt = body.ShippedAt
		}
		if body.EstimatedArrival != nil {
			t, _ := time.Parse("2006-01-02", body.EstimatedArrival.Format("2006-01-02"))
			estimatedArrival = &t
		}
		if body.ActualArrival != nil {
			t, _ := time.Parse("2006-01-02", body.ActualArrival.Format("2006-01-02"))
			actualArrival = &t
		}

		sh, err := h.svc.UpdateShipment(r.Context(), uuid.UUID(shipmentId),
			fromLocation, toLocation, carrier, trackingNumber, batchNumber, status,
			shippedAt, estimatedArrival, actualArrival, nil, version)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, sh)
	})).ServeHTTP(w, r)
}

func (h *Handler) ArchiveShipment(w http.ResponseWriter, r *http.Request, shipmentId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sh, err := h.svc.GetShipment(r.Context(), uuid.UUID(shipmentId))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		archived, err := h.svc.ArchiveShipment(r.Context(), uuid.UUID(shipmentId), sh.Version)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, archived)
	})).ServeHTTP(w, r)
}

func (h *Handler) ListShipmentsByPurchaseOrder(w http.ResponseWriter, r *http.Request, poId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		poUUID := uuid.UUID(poId)
		shipments, err := h.svc.ListShipments(r.Context(), nil, &poUUID)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, shipments)
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
