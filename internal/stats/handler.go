package stats

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/auth"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/httpx"
)

// Handler provides the HTTP handlers for statistics (STA-01 to STA-04).
type Handler struct {
	svc    *Service
	mw     *auth.SessionMiddleware
	logger *slog.Logger
}

// NewHandler creates a new stats handler.
func NewHandler(svc *Service, mw *auth.SessionMiddleware) *Handler {
	return &Handler{svc: svc, mw: mw}
}

// KeyFigures returns summary statistics (STA-01, STA-02).
func (h *Handler) KeyFigures(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.keyFigures)).ServeHTTP(w, r)
}

func (h *Handler) keyFigures(w http.ResponseWriter, r *http.Request) {
	data, err := h.svc.KeyFigures(r.Context())
	if err != nil {
		h.logger.ErrorContext(r.Context(), "KeyFigures failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}

// OrdersPerMonth returns monthly order and revenue data (STA-02).
func (h *Handler) OrdersPerMonth(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.ordersPerMonth)).ServeHTTP(w, r)
}

func (h *Handler) ordersPerMonth(w http.ResponseWriter, r *http.Request) {
	data, err := h.svc.OrdersPerMonth(r.Context())
	if err != nil {
		h.logger.ErrorContext(r.Context(), "OrdersPerMonth failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}

// RevenueByChannel returns revenue per sales channel (STA-02).
func (h *Handler) RevenueByChannel(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.revenueByChannel)).ServeHTTP(w, r)
}

func (h *Handler) revenueByChannel(w http.ResponseWriter, r *http.Request) {
	data, err := h.svc.RevenueByChannel(r.Context())
	if err != nil {
		h.logger.ErrorContext(r.Context(), "RevenueByChannel failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}

// RevenueByCountry returns revenue per shipping country (STA-02).
func (h *Handler) RevenueByCountry(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.revenueByCountry)).ServeHTTP(w, r)
}

func (h *Handler) revenueByCountry(w http.ResponseWriter, r *http.Request) {
	data, err := h.svc.RevenueByCountry(r.Context())
	if err != nil {
		h.logger.ErrorContext(r.Context(), "RevenueByCountry failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}

// MarginPerProduct returns product margin data (STA-02).
func (h *Handler) MarginPerProduct(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.marginPerProduct)).ServeHTTP(w, r)
}

func (h *Handler) marginPerProduct(w http.ResponseWriter, r *http.Request) {
	data, err := h.svc.MarginPerProduct(r.Context())
	if err != nil {
		h.logger.ErrorContext(r.Context(), "MarginPerProduct failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}

// OrderToShippedTime returns average fulfillment time (STA-04).
func (h *Handler) OrderToShippedTime(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.orderToShippedTime)).ServeHTTP(w, r)
}

func (h *Handler) orderToShippedTime(w http.ResponseWriter, r *http.Request) {
	data, err := h.svc.OrderToShippedTime(r.Context())
	if err != nil {
		h.logger.ErrorContext(r.Context(), "OrderToShippedTime failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}

// RepurchaseRate returns customer retention rate (STA-04).
func (h *Handler) RepurchaseRate(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.repurchaseRate)).ServeHTTP(w, r)
}

func (h *Handler) repurchaseRate(w http.ResponseWriter, r *http.Request) {
	data, err := h.svc.RepurchaseRate(r.Context())
	if err != nil {
		h.logger.ErrorContext(r.Context(), "RepurchaseRate failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}

// SalesByCustomerType returns sales per customer type (STA-04).
func (h *Handler) SalesByCustomerType(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.salesByCustomerType)).ServeHTTP(w, r)
}

func (h *Handler) salesByCustomerType(w http.ResponseWriter, r *http.Request) {
	data, err := h.svc.SalesByCustomerType(r.Context())
	if err != nil {
		h.logger.ErrorContext(r.Context(), "SalesByCustomerType failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}

// InventoryTurnover returns inventory metrics (STA-03).
func (h *Handler) InventoryTurnover(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.inventoryTurnover)).ServeHTTP(w, r)
}

func (h *Handler) inventoryTurnover(w http.ResponseWriter, r *http.Request) {
	data, err := h.svc.InventoryTurnover(r.Context())
	if err != nil {
		h.logger.ErrorContext(r.Context(), "InventoryTurnover failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}

// LowStockProducts returns products with low inventory (STA-01).
func (h *Handler) LowStockProducts(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.lowStockProducts)).ServeHTTP(w, r)
}

func (h *Handler) lowStockProducts(w http.ResponseWriter, r *http.Request) {
	data, err := h.svc.LowStockProducts(r.Context())
	if err != nil {
		h.logger.ErrorContext(r.Context(), "LowStockProducts failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}

// OrdersByStatus returns orders grouped by status (STA-01).
func (h *Handler) OrdersByStatus(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.ordersByStatus)).ServeHTTP(w, r)
}

func (h *Handler) ordersByStatus(w http.ResponseWriter, r *http.Request) {
	data, err := h.svc.OrdersByStatus(r.Context())
	if err != nil {
		h.logger.ErrorContext(r.Context(), "OrdersByStatus failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}

// ShipmentsOnTheirWay returns active shipments (STA-01).
func (h *Handler) ShipmentsOnTheirWay(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.shipmentsOnTheirWay)).ServeHTTP(w, r)
}

func (h *Handler) shipmentsOnTheirWay(w http.ResponseWriter, r *http.Request) {
	data, err := h.svc.ShipmentsOnTheirWay(r.Context())
	if err != nil {
		h.logger.ErrorContext(r.Context(), "ShipmentsOnTheirWay failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}

// CardsDue returns boards with due cards (STA-01).
func (h *Handler) CardsDue(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.cardsDue)).ServeHTTP(w, r)
}

func (h *Handler) cardsDue(w http.ResponseWriter, r *http.Request) {
	data, err := h.svc.CardsDue(r.Context())
	if err != nil {
		h.logger.ErrorContext(r.Context(), "CardsDue failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}

// writeJSON encodes data as JSON and writes it to the response writer.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
