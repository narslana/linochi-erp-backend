package inventory

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

// Handler provides the HTTP handlers for inventory (LAG-01 to LAG-05).
type Handler struct {
	svc    *Service
	mw     *auth.SessionMiddleware
	logger *slog.Logger
}

// NewHandler creates a new inventory handler.
func NewHandler(svc *Service, mw *auth.SessionMiddleware, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, mw: mw, logger: logger}
}

// ─── Locations ────────────────────────────────────────────────────────────────

// ListLocations returns all active locations (LAG-01).
func (h *Handler) ListLocations(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.listLocations)).ServeHTTP(w, r)
}

func (h *Handler) listLocations(w http.ResponseWriter, r *http.Request) {
	locations, err := h.svc.ListLocations(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, locations)
}

// CreateLocation creates a new location (LAG-01).
func (h *Handler) CreateLocation(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.createLocation)).ServeHTTP(w, r)
}

func (h *Handler) createLocation(w http.ResponseWriter, r *http.Request) {
	var body api.Location
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	var nameEn, address, city, country string
	if body.NameEn != nil {
		nameEn = *body.NameEn
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

	loc, err := h.svc.CreateLocation(r.Context(), body.Name, nameEn, address, city, country)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "CreateLocation failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, loc)
}

// GetLocation returns a location by ID.
func (h *Handler) GetLocation(w http.ResponseWriter, r *http.Request, locationId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.getLocation(w, r, locationId)
	})).ServeHTTP(w, r)
}

func (h *Handler) getLocation(w http.ResponseWriter, r *http.Request, locationId openapi_types.UUID) {
	loc, err := h.svc.GetLocation(r.Context(), uuid.UUID(locationId))
	if err != nil {
		if errors.Is(err, newNotFound("location")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "GetLocation failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, loc)
}

// UpdateLocation updates a location (LAG-01).
func (h *Handler) UpdateLocation(w http.ResponseWriter, r *http.Request, locationId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.updateLocation(w, r, locationId)
	})).ServeHTTP(w, r)
}

func (h *Handler) updateLocation(w http.ResponseWriter, r *http.Request, locationId openapi_types.UUID) {
	var body api.Location
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	var name, nameEn, address, city, country *string
	if body.Name != "" {
		name = &body.Name
	}
	if body.NameEn != nil {
		nameEn = body.NameEn
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

	loc, err := h.svc.UpdateLocation(r.Context(), uuid.UUID(locationId), name, nameEn, address, city, country, int(body.Version))
	if err != nil {
		if errors.Is(err, newNotFound("location")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "UpdateLocation failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, loc)
}

// ArchiveLocation archives a location (NFK-09).
func (h *Handler) ArchiveLocation(w http.ResponseWriter, r *http.Request, locationId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.archiveLocation(w, r, locationId)
	})).ServeHTTP(w, r)
}

func (h *Handler) archiveLocation(w http.ResponseWriter, r *http.Request, locationId openapi_types.UUID) {
	var body struct {
		Version int `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	version := body.Version
	if version == 0 {
		loc, err := h.svc.GetLocation(r.Context(), uuid.UUID(locationId))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		version = loc.Version
	}

	loc, err := h.svc.ArchiveLocation(r.Context(), uuid.UUID(locationId), version)
	if err != nil {
		if errors.Is(err, newNotFound("location")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "ArchiveLocation failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, loc)
}

// ─── Stock Movements ──────────────────────────────────────────────────────────

// ListStockMovements returns stock movements (LAG-01).
func (h *Handler) ListStockMovements(w http.ResponseWriter, r *http.Request, params api.ListStockMovementsParams) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.listStockMovements(w, r)
	})).ServeHTTP(w, r)
}

func (h *Handler) listStockMovements(w http.ResponseWriter, r *http.Request) {
	movements, err := h.svc.ListStockMovements(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, movements)
}

// CreateStockMovement creates a stock movement (LAG-01, LAG-04).
func (h *Handler) CreateStockMovement(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.createStockMovement)).ServeHTTP(w, r)
}

func (h *Handler) createStockMovement(w http.ResponseWriter, r *http.Request) {
	var body api.StockMovement
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	var manufacturingDate, bestBeforeDate *string
	if body.ManufacturingDate != nil {
		s := body.ManufacturingDate.String()
		manufacturingDate = &s
	}
	if body.BestBeforeDate != nil {
		s := body.BestBeforeDate.String()
		bestBeforeDate = &s
	}

	var referenceID *string
	if body.ReferenceId != nil {
		s := body.ReferenceId.String()
		referenceID = &s
	}

	var batchNumber, reason, referenceType string
	if body.BatchNumber != nil {
		batchNumber = *body.BatchNumber
	}
	if body.Reason != nil {
		reason = *body.Reason
	}
	if body.ReferenceType != nil {
		referenceType = *body.ReferenceType
	}

	movement, err := h.svc.CreateStockMovement(r.Context(), MovementInput{
		ProductID: uuid.UUID(body.ProductId), LocationID: uuid.UUID(body.LocationId),
		BatchNumber: batchNumber, ManufacturingDate: manufacturingDate, BestBeforeDate: bestBeforeDate,
		Type: string(body.MovementType), Quantity: body.Quantity, Reason: reason,
		ReferenceType: referenceType, ReferenceID: referenceID,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, movement)
}

// ─── Balance & FEFO ───────────────────────────────────────────────────────────

// GetBalance returns the balance for a product (LAG-01, LAG-02).
func (h *Handler) GetBalance(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.getBalance(w, r, productId)
	})).ServeHTTP(w, r)
}

func (h *Handler) getBalance(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID) {
	balance, err := h.svc.GetBalance(r.Context(), uuid.UUID(productId))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, balance)
}

// GetFEFOBatches returns batches ordered by FEFO (first-expiry-first-out) (LAG-03).
func (h *Handler) GetFEFOBatches(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID, locationId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.getFEFOBatches(w, r, productId, locationId)
	})).ServeHTTP(w, r)
}

func (h *Handler) getFEFOBatches(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID, locationId openapi_types.UUID) {
	batches, err := h.svc.GetFEFOBatches(r.Context(), uuid.UUID(productId), uuid.UUID(locationId))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, batches)
}

// GetInventoryValue returns the inventory value per product per location (LAG-05).
func (h *Handler) GetInventoryValue(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.getInventoryValue)).ServeHTTP(w, r)
}

func (h *Handler) getInventoryValue(w http.ResponseWriter, r *http.Request) {
	value, err := h.svc.GetInventoryValue(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

// ─── Reservation & Withdrawal ─────────────────────────────────────────────────

// ReserveStock reserves stock for an order (LAG-02).
func (h *Handler) ReserveStock(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.reserveStock)).ServeHTTP(w, r)
}

func (h *Handler) reserveStock(w http.ResponseWriter, r *http.Request) {
	var body api.ReserveStockJSONBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	var referenceType string
	if body.ReferenceType != nil {
		referenceType = *body.ReferenceType
	}

	err := h.svc.ReserveStock(r.Context(), StockRef{ProductID: uuid.UUID(body.ProductId), LocationID: uuid.UUID(body.LocationId), Quantity: body.Quantity, ReferenceType: referenceType, ReferenceID: uuid.UUID(body.ReferenceId)})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "reserved"})
}

// ReleaseStock releases a reservation (LAG-02).
func (h *Handler) ReleaseStock(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.releaseStock)).ServeHTTP(w, r)
}

func (h *Handler) releaseStock(w http.ResponseWriter, r *http.Request) {
	var body api.ReleaseStockJSONBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	var referenceType string
	if body.ReferenceType != nil {
		referenceType = *body.ReferenceType
	}

	err := h.svc.ReleaseStock(r.Context(), StockRef{ProductID: uuid.UUID(body.ProductId), LocationID: uuid.UUID(body.LocationId), Quantity: body.Quantity, ReferenceType: referenceType, ReferenceID: uuid.UUID(body.ReferenceId)})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "released"})
}

// WithdrawStock withdraws stock when shipped (LAG-02, LAG-03).
func (h *Handler) WithdrawStock(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.withdrawStock)).ServeHTTP(w, r)
}

func (h *Handler) withdrawStock(w http.ResponseWriter, r *http.Request) {
	var body api.WithdrawStockJSONBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	var referenceType string
	if body.ReferenceType != nil {
		referenceType = *body.ReferenceType
	}

	in := ShipInput{StockRef: StockRef{
		ProductID: uuid.UUID(body.ProductId), LocationID: uuid.UUID(body.LocationId), Quantity: body.Quantity,
		ReferenceType: referenceType, ReferenceID: uuid.UUID(body.ReferenceId),
	}}
	if body.OrderLineId != nil {
		in.OrderLineID = uuid.UUID(*body.OrderLineId)
	}
	if body.BatchNumber != nil {
		in.BatchNumber = *body.BatchNumber
	}
	batches, err := h.svc.WithdrawStock(r.Context(), in)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "withdrawn", "batches": batches})
}

// writeJSON encodes data as JSON and writes it to the response writer.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
