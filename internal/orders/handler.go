package orders

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/api"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/auth"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/httpx"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/workflow"
)

// Handler provides the HTTP handlers for orders (ORD-01 to ORD-08).
type Handler struct {
	svc    *Service
	mw     *auth.SessionMiddleware
	logger *slog.Logger
}

// NewHandler creates a new orders handler.
func NewHandler(svc *Service, mw *auth.SessionMiddleware, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, mw: mw, logger: logger}
}

// ─── Orders ───────────────────────────────────────────────────────────────────

// ListOrders returns all active orders (ORD-01).
func (h *Handler) ListOrders(w http.ResponseWriter, r *http.Request, params api.ListOrdersParams) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.listOrders(w, r, params)
	})).ServeHTTP(w, r)
}

func (h *Handler) listOrders(w http.ResponseWriter, r *http.Request, params api.ListOrdersParams) {
	var status *string
	if params.Status != nil {
		s := string(*params.Status)
		status = &s
	}
	orders, err := h.svc.ListOrders(r.Context(), status, params.CustomerId, nil, nil)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, orders)
}

// CreateOrder creates a new order (ORD-01).
func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.createOrder)).ServeHTTP(w, r)
}

func (h *Handler) createOrder(w http.ResponseWriter, r *http.Request) {
	var body api.Order
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	var externalID string
	if body.ExternalId != nil {
		externalID = *body.ExternalId
	}
	var shippingName, shippingAddress, shippingCity, shippingPostalCode, shippingCountry string
	if body.ShippingName != nil {
		shippingName = *body.ShippingName
	}
	if body.ShippingAddress != nil {
		shippingAddress = *body.ShippingAddress
	}
	if body.ShippingCity != nil {
		shippingCity = *body.ShippingCity
	}
	if body.ShippingPostalCode != nil {
		shippingPostalCode = *body.ShippingPostalCode
	}
	if body.ShippingCountry != nil {
		shippingCountry = *body.ShippingCountry
	}

	var billingName, billingAddress, billingCity, billingPostalCode, billingCountry string
	if body.BillingName != nil {
		billingName = *body.BillingName
	}
	if body.BillingAddress != nil {
		billingAddress = *body.BillingAddress
	}
	if body.BillingCity != nil {
		billingCity = *body.BillingCity
	}
	if body.BillingPostalCode != nil {
		billingPostalCode = *body.BillingPostalCode
	}
	if body.BillingCountry != nil {
		billingCountry = *body.BillingCountry
	}

	var trackingNumber, carrier, shippingLabelUrl, notes string
	if body.TrackingNumber != nil {
		trackingNumber = *body.TrackingNumber
	}
	if body.Carrier != nil {
		carrier = *body.Carrier
	}
	if body.ShippingLabelUrl != nil {
		shippingLabelUrl = *body.ShippingLabelUrl
	}
	if body.Notes != nil {
		notes = *body.Notes
	}

	order, err := h.svc.CreateOrder(r.Context(), body.CustomerId, externalID, body.Channel, body.Currency,
		int64(body.ExchangeRateSek), int64(body.SubtotalSek), int64(body.TaxSek), int64(body.FreightSek),
		int64(body.CommissionSek), int64(body.TotalSek),
		shippingName, shippingAddress, shippingCity, shippingPostalCode, shippingCountry,
		billingName, billingAddress, billingCity, billingPostalCode, billingCountry,
		trackingNumber, carrier, shippingLabelUrl, notes)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "CreateOrder failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, order)
}

// GetOrder returns an order by ID.
func (h *Handler) GetOrder(w http.ResponseWriter, r *http.Request, orderId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.getOrder(w, r, orderId)
	})).ServeHTTP(w, r)
}

func (h *Handler) getOrder(w http.ResponseWriter, r *http.Request, orderId openapi_types.UUID) {
	order, err := h.svc.GetOrder(r.Context(), uuid.UUID(orderId))
	if err != nil {
		if errors.Is(err, newNotFound("order")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "GetOrder failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

// UpdateOrder updates an order (ORD-01).
func (h *Handler) UpdateOrder(w http.ResponseWriter, r *http.Request, orderId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.updateOrder(w, r, orderId)
	})).ServeHTTP(w, r)
}

func (h *Handler) updateOrder(w http.ResponseWriter, r *http.Request, orderId openapi_types.UUID) {
	var body api.Order
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	var channel, currency, notes *string
	if body.Channel != "" {
		channel = &body.Channel
	}
	if body.Currency != "" {
		currency = &body.Currency
	}
	if body.Notes != nil {
		notes = body.Notes
	}

	var externalID, trackingNumber, carrier, shippingLabelUrl *string
	if body.ExternalId != nil {
		externalID = body.ExternalId
	}
	if body.TrackingNumber != nil {
		trackingNumber = body.TrackingNumber
	}
	if body.Carrier != nil {
		carrier = body.Carrier
	}
	if body.ShippingLabelUrl != nil {
		shippingLabelUrl = body.ShippingLabelUrl
	}

	var exchangeRateSek, subtotalSek, taxSek, freightSek, commissionSek, totalSek *int64
	if body.ExchangeRateSek != 0 {
		v := int64(body.ExchangeRateSek)
		exchangeRateSek = &v
	}
	if body.SubtotalSek != 0 {
		v := int64(body.SubtotalSek)
		subtotalSek = &v
	}
	if body.TaxSek != 0 {
		v := int64(body.TaxSek)
		taxSek = &v
	}
	if body.FreightSek != 0 {
		v := int64(body.FreightSek)
		freightSek = &v
	}
	if body.CommissionSek != 0 {
		v := int64(body.CommissionSek)
		commissionSek = &v
	}
	if body.TotalSek != 0 {
		v := int64(body.TotalSek)
		totalSek = &v
	}

	var shippingName, shippingAddress, shippingCity, shippingPostalCode, shippingCountry *string
	if body.ShippingName != nil {
		shippingName = body.ShippingName
	}
	if body.ShippingAddress != nil {
		shippingAddress = body.ShippingAddress
	}
	if body.ShippingCity != nil {
		shippingCity = body.ShippingCity
	}
	if body.ShippingPostalCode != nil {
		shippingPostalCode = body.ShippingPostalCode
	}
	if body.ShippingCountry != nil {
		shippingCountry = body.ShippingCountry
	}

	var billingName, billingAddress, billingCity, billingPostalCode, billingCountry *string
	if body.BillingName != nil {
		billingName = body.BillingName
	}
	if body.BillingAddress != nil {
		billingAddress = body.BillingAddress
	}
	if body.BillingCity != nil {
		billingCity = body.BillingCity
	}
	if body.BillingPostalCode != nil {
		billingPostalCode = body.BillingPostalCode
	}
	if body.BillingCountry != nil {
		billingCountry = body.BillingCountry
	}

	order, err := h.svc.UpdateOrder(r.Context(), externalID, channel, currency, trackingNumber, carrier, shippingLabelUrl, notes, exchangeRateSek, subtotalSek, taxSek, freightSek, commissionSek, totalSek, nil, shippingName, shippingAddress, shippingCity, shippingPostalCode, shippingCountry, billingName, billingAddress, billingCity, billingPostalCode, billingCountry, uuid.UUID(orderId), int(body.Version))
	if err != nil {
		if errors.Is(err, newNotFound("order")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "UpdateOrder failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

// ArchiveOrder archives an order (NFK-09).
func (h *Handler) ArchiveOrder(w http.ResponseWriter, r *http.Request, orderId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.archiveOrder(w, r, orderId)
	})).ServeHTTP(w, r)
}

func (h *Handler) archiveOrder(w http.ResponseWriter, r *http.Request, orderId openapi_types.UUID) {
	var body struct {
		Version int `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	version := body.Version
	if version == 0 {
		order, err := h.svc.GetOrder(r.Context(), uuid.UUID(orderId))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		version = order.Version
	}

	order, err := h.svc.ArchiveOrder(r.Context(), uuid.UUID(orderId), version)
	if err != nil {
		if errors.Is(err, newNotFound("order")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "ArchiveOrder failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

// UpdateOrderStatus moves an order to another step of its workflow, optionally
// as a manual override by an admin (ORD-03, WF-02, WF-05).
func (h *Handler) UpdateOrderStatus(w http.ResponseWriter, r *http.Request, orderId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body api.UpdateOrderStatus
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.Error(w, r, err)
			return
		}
		h.change(w, r, uuid.UUID(orderId), body.Status, body.Reason, body.Override)
	})).ServeHTTP(w, r)
}

// statusChange is the answer of every status endpoint (OrderStatusChange in the OpenAPI).
type statusChange struct {
	Order    Order                    `json:"order"`
	Effects  []workflow.PlannedEffect `json:"effects"`
	Warnings []string                 `json:"warnings"`
}

// change runs one status change for a handler and writes the result.
func (h *Handler) change(w http.ResponseWriter, r *http.Request, id uuid.UUID, toKey string, reason *string, override *bool) {
	actor, ok := workflow.ActorFromRequest(r)
	if !ok {
		httpx.Error(w, r, auth.ErrUnauthenticated)
		return
	}
	order, res, err := h.svc.ChangeOrderStatus(r.Context(), id, toKey, actor, reasonString(reason), override != nil && *override)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if res.Effects == nil {
		res.Effects = []workflow.PlannedEffect{}
	}
	writeJSON(w, http.StatusOK, statusChange{Order: order, Effects: res.Effects, Warnings: res.Warnings})
}

// PreviewOrderTransition shows the effects of a move without running them (WF-05).
func (h *Handler) PreviewOrderTransition(w http.ResponseWriter, r *http.Request, id openapi_types.UUID, toKey string, params api.PreviewOrderTransitionParams) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		effects, err := h.svc.PreviewTransition(r.Context(), uuid.UUID(id), toKey, params.Override != nil && *params.Override)
		if err != nil {
			httpx.Error(w, r, ProblemError(err))
			return
		}
		if effects == nil {
			effects = []workflow.PlannedEffect{}
		}
		writeJSON(w, http.StatusOK, effects)
	})).ServeHTTP(w, r)
}

// GetOrderHistory returns the status history and the time in each step (WF-10).
func (h *Handler) GetOrderHistory(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entries, times, err := h.svc.History(r.Context(), uuid.UUID(id), time.Now())
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		type stepTime struct {
			Key     string  `json:"key"`
			Seconds float64 `json:"seconds"`
		}
		tis := make([]stepTime, 0, len(times))
		for _, t := range times {
			tis = append(tis, stepTime{Key: t.Key, Seconds: t.Duration.Seconds()})
		}
		if entries == nil {
			entries = []workflow.HistoryEntry{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"entries": entries, "time_in_steps": tis})
	})).ServeHTTP(w, r)
}

// ─── Order Lines ──────────────────────────────────────────────────────────────

// ListOrderLines returns all lines for an order (ORD-02).
func (h *Handler) ListOrderLines(w http.ResponseWriter, r *http.Request, orderId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.listOrderLines(w, r, orderId)
	})).ServeHTTP(w, r)
}

func (h *Handler) listOrderLines(w http.ResponseWriter, r *http.Request, orderId openapi_types.UUID) {
	lines, err := h.svc.ListOrderLines(r.Context(), uuid.UUID(orderId))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, lines)
}

// CreateOrderLine creates a new order line (ORD-02).
func (h *Handler) CreateOrderLine(w http.ResponseWriter, r *http.Request, orderId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.createOrderLine(w, r, orderId)
	})).ServeHTTP(w, r)
}

func (h *Handler) createOrderLine(w http.ResponseWriter, r *http.Request, orderId openapi_types.UUID) {
	var body api.OrderLine
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	line, err := h.svc.CreateOrderLine(r.Context(), uuid.UUID(orderId), body.ProductId, int64(body.Quantity), int64(body.QuantityShipped), int64(body.UnitPriceSek), int64(body.TaxAmountSek), int64(body.TotalSek), float64(body.TaxRate))
	if err != nil {
		h.logger.ErrorContext(r.Context(), "CreateOrderLine failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, line)
}

// UpdateOrderLine updates an order line (ORD-02).
func (h *Handler) UpdateOrderLine(w http.ResponseWriter, r *http.Request, orderId openapi_types.UUID, orderLineId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.updateOrderLine(w, r, orderId, orderLineId)
	})).ServeHTTP(w, r)
}

func (h *Handler) updateOrderLine(w http.ResponseWriter, r *http.Request, orderId openapi_types.UUID, orderLineId openapi_types.UUID) {
	var body api.OrderLine
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	quantity := int64(body.Quantity)
	quantityShipped := int64(body.QuantityShipped)
	unitPriceSek := int64(body.UnitPriceSek)
	taxAmountSek := int64(body.TaxAmountSek)
	totalSek := int64(body.TotalSek)
	taxRate := float64(body.TaxRate)

	q := int(quantity)
	qs := int(quantityShipped)
	line, err := h.svc.UpdateOrderLine(r.Context(), &q, &qs, &unitPriceSek, &taxAmountSek, &totalSek, &taxRate, uuid.UUID(orderLineId), int(body.Version))
	if err != nil {
		if errors.Is(err, newNotFound("order line")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "UpdateOrderLine failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, line)
}

// ArchiveOrderLine archives an order line (NFK-09).
func (h *Handler) ArchiveOrderLine(w http.ResponseWriter, r *http.Request, orderId openapi_types.UUID, orderLineId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.archiveOrderLine(w, r, orderId, orderLineId)
	})).ServeHTTP(w, r)
}

func (h *Handler) archiveOrderLine(w http.ResponseWriter, r *http.Request, orderId openapi_types.UUID, orderLineId openapi_types.UUID) {
	var body struct {
		Version int `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	version := body.Version
	if version == 0 {
		line, err := h.svc.GetOrderLine(r.Context(), uuid.UUID(orderLineId))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		version = line.Version
	}

	line, err := h.svc.ArchiveOrderLine(r.Context(), uuid.UUID(orderLineId), version)
	if err != nil {
		if errors.Is(err, newNotFound("order line")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "ArchiveOrderLine failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, line)
}

// AcceptOrder accepts an order: reserves its stock (ORD-04, WF-05).
func (h *Handler) AcceptOrder(w http.ResponseWriter, r *http.Request, orderId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body api.AcceptOrder
		if !decodeOptional(w, r, &body) {
			return
		}
		h.change(w, r, uuid.UUID(orderId), targetAccept, body.Reason, body.Override)
	})).ServeHTTP(w, r)
}

// ShipOrder ships an order: withdraws its stock (ORD-06).
func (h *Handler) ShipOrder(w http.ResponseWriter, r *http.Request, orderId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body api.ShipOrder
		if !decodeOptional(w, r, &body) {
			return
		}
		h.change(w, r, uuid.UUID(orderId), targetShip, body.Reason, body.Override)
	})).ServeHTTP(w, r)
}

// CancelOrder cancels an order and releases its reservation (ORD-07).
func (h *Handler) CancelOrder(w http.ResponseWriter, r *http.Request, orderId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body api.CancelOrder
		if !decodeOptional(w, r, &body) {
			return
		}
		h.change(w, r, uuid.UUID(orderId), targetCancel, body.Reason, body.Override)
	})).ServeHTTP(w, r)
}

// decodeOptional reads a JSON body that may be empty.
func decodeOptional(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil && !errors.Is(err, io.EOF) {
		httpx.Error(w, r, err)
		return false
	}
	return true
}

// DeleteOrderLine deletes an order line (NFK-09).
func (h *Handler) DeleteOrderLine(w http.ResponseWriter, r *http.Request, orderId openapi_types.UUID, orderLineId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.deleteOrderLine(w, r, orderId, orderLineId)
	})).ServeHTTP(w, r)
}

func (h *Handler) deleteOrderLine(w http.ResponseWriter, r *http.Request, orderId openapi_types.UUID, orderLineId openapi_types.UUID) {
	_, err := h.svc.DeleteOrderLine(r.Context(), uuid.UUID(orderLineId))
	if err != nil {
		if errors.Is(err, newNotFound("order line")) {
			httpx.Error(w, r, err)
			return
		}
		h.logger.ErrorContext(r.Context(), "DeleteOrderLine failed", "error", err)
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BatchExportOrders exports orders as CSV (ORD-13).
func (h *Handler) BatchExportOrders(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.batchExportOrders(w, r)
	})).ServeHTTP(w, r)
}

func (h *Handler) batchExportOrders(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CustomerID *string `json:"customer_id,omitempty"`
		Status     *string `json:"status,omitempty"`
		FromDate   *string `json:"from_date,omitempty"`
		ToDate     *string `json:"to_date,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, r, err)
		return
	}

	var customerID *uuid.UUID
	if body.CustomerID != nil {
		if id, err := uuid.Parse(*body.CustomerID); err == nil {
			customerID = &id
		}
	}

	orders, err := h.svc.ListOrders(r.Context(), body.Status, customerID, body.FromDate, body.ToDate)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Cache-Control", "no-store")
	writer := csv.NewWriter(w)
	defer writer.Flush()

	if err := writer.Write([]string{"id", "status", "customer_id", "created_at"}); err != nil {
		httpx.Error(w, r, err)
		return
	}
	for _, o := range orders {
		if err := writer.Write([]string{
			o.ID.String(),
			o.Status,
			o.CustomerID.String(),
			o.CreatedAt,
		}); err != nil {
			httpx.Error(w, r, err)
			return
		}
	}
}

// ExportOrder exports an order as PDF (ORD-09).
func (h *Handler) ExportOrder(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.exportOrder(w, r, id)
	})).ServeHTTP(w, r)
}

func (h *Handler) exportOrder(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	if _, err := h.svc.GetOrder(r.Context(), id); err != nil {
		httpx.Error(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Cache-Control", "no-store")

	// Minimal valid PDF stub (ORD-09: full PDF generation requires a PDF library)
	pdf := fmt.Sprintf("%%PDF-1.4\n1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\nxref\n0 4\n0000000000 65535 f \n0000000009 00000 n \n0000000058 00000 n \n0000000115 00000 n \ntrailer\n<< /Size 4 /Root 1 0 R >>\nstartxref\n190\n%%EOF")
	_, _ = w.Write([]byte(pdf))
}

// writeJSON encodes data as JSON and writes it to the response writer.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
