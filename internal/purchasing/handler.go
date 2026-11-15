package purchasing

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

// Handler implements api.ServerInterface for the purchasing module.
type Handler struct {
	svc    *Service
	mw     *auth.SessionMiddleware
	logger *slog.Logger
}

// NewHandler creates a purchasing handler.
func NewHandler(svc *Service, mw *auth.SessionMiddleware, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, mw: mw, logger: logger}
}

// ─── Purchase Orders ──────────────────────────────────────────────────────────

func (h *Handler) ListPurchaseOrders(w http.ResponseWriter, r *http.Request, params api.ListPurchaseOrdersParams) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var status *string
		if params.Status != nil {
			v := string(*params.Status)
			status = &v
		}
		var supplierID *uuid.UUID
		if params.SupplierId != nil {
			v := uuid.UUID(*params.SupplierId)
			supplierID = &v
		}
		orders, err := h.svc.ListPurchaseOrders(r.Context(), status, supplierID)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, orders)
	})).ServeHTTP(w, r)
}

func (h *Handler) CreatePurchaseOrder(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.createPurchaseOrder)).ServeHTTP(w, r)
}

func (h *Handler) createPurchaseOrder(w http.ResponseWriter, r *http.Request) {
	var body api.CreatePurchaseOrderJSONBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteProblem(w, r, httpx.Problem{Code: "invalid-request", Title: "Invalid request body", Status: http.StatusBadRequest})
		return
	}

	var currency, orderNumber, notes string
	if body.Currency != nil {
		currency = string(*body.Currency)
	} else {
		currency = "SEK"
	}
	if body.OrderNumber != nil {
		orderNumber = *body.OrderNumber
	}
	if body.Notes != nil {
		notes = *body.Notes
	}

	var orderDate, expectedDate *time.Time
	if body.OrderDate != nil {
		t, _ := time.Parse("2006-01-02", body.OrderDate.Format("2006-01-02"))
		orderDate = &t
	}
	if body.ExpectedDate != nil {
		t, _ := time.Parse("2006-01-02", body.ExpectedDate.Format("2006-01-02"))
		expectedDate = &t
	}

	po, err := h.svc.CreatePurchaseOrder(r.Context(), uuid.UUID(body.SupplierId), orderNumber, currency, orderDate, expectedDate, notes)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/purchase-orders/"+po.ID.String())
	writeJSON(w, http.StatusCreated, po)
}

func (h *Handler) GetPurchaseOrder(w http.ResponseWriter, r *http.Request, poId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		po, err := h.svc.GetPurchaseOrder(r.Context(), uuid.UUID(poId))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, po)
	})).ServeHTTP(w, r)
}

func (h *Handler) UpdatePurchaseOrder(w http.ResponseWriter, r *http.Request, poId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body api.UpdatePurchaseOrderJSONBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, httpx.Problem{Code: "invalid-request", Title: "Invalid request body", Status: http.StatusBadRequest})
			return
		}

		version := body.Version

		var status, notes *string
		if body.Status != nil {
			v := string(*body.Status)
			status = &v
		}
		if body.Notes != nil {
			notes = body.Notes
		}

		var expectedDate *time.Time
		if body.ExpectedDate != nil {
			t, _ := time.Parse("2006-01-02", body.ExpectedDate.Format("2006-01-02"))
			expectedDate = &t
		}

		var totalAmount *int64
		if body.TotalAmount != nil {
			v := int64(*body.TotalAmount)
			totalAmount = &v
		}

		po, err := h.svc.UpdatePurchaseOrder(r.Context(), uuid.UUID(poId), status, expectedDate, totalAmount, notes, version)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, po)
	})).ServeHTTP(w, r)
}

func (h *Handler) ArchivePurchaseOrder(w http.ResponseWriter, r *http.Request, poId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		po, err := h.svc.GetPurchaseOrder(r.Context(), uuid.UUID(poId))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		archived, err := h.svc.ArchivePurchaseOrder(r.Context(), uuid.UUID(poId), po.Version)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, archived)
	})).ServeHTTP(w, r)
}

// ─── Purchase Order Lines ─────────────────────────────────────────────────────

func (h *Handler) ListPurchaseOrderLines(w http.ResponseWriter, r *http.Request, poId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lines, err := h.svc.ListLines(r.Context(), uuid.UUID(poId))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, lines)
	})).ServeHTTP(w, r)
}

func (h *Handler) CreatePurchaseOrderLine(w http.ResponseWriter, r *http.Request, poId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body api.CreatePurchaseOrderLineJSONBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, httpx.Problem{Code: "invalid-request", Title: "Invalid request body", Status: http.StatusBadRequest})
			return
		}

		var unitCurrency string
		if body.UnitCurrency != nil {
			unitCurrency = string(*body.UnitCurrency)
		} else {
			unitCurrency = "SEK"
		}

		var unitPrice int64
		if body.UnitPrice != nil {
			unitPrice = int64(*body.UnitPrice)
		}
		lineTotal := int64(body.Quantity) * unitPrice
		line, err := h.svc.CreateLine(r.Context(), uuid.UUID(poId), uuid.UUID(body.ProductId), body.Quantity, unitPrice, lineTotal, unitCurrency)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.Header().Set("Location", "/api/v1/purchase-orders/"+poId.String()+"/lines/"+line.ID.String())
		writeJSON(w, http.StatusCreated, line)
	})).ServeHTTP(w, r)
}

func (h *Handler) UpdatePurchaseOrderLine(w http.ResponseWriter, r *http.Request, poId openapi_types.UUID, lineId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body api.UpdatePurchaseOrderLineJSONBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, httpx.Problem{Code: "invalid-request", Title: "Invalid request body", Status: http.StatusBadRequest})
			return
		}

		version := body.Version

		var quantity, lineTotal *int
		if body.Quantity != nil {
			v := int(*body.Quantity)
			quantity = &v
		}
		if body.LineTotal != nil {
			v := int(*body.LineTotal)
			lineTotal = &v
		}

		line, err := h.svc.UpdateLine(r.Context(), uuid.UUID(lineId), quantity, nil, lineTotal, version)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, line)
	})).ServeHTTP(w, r)
}

func (h *Handler) ArchivePurchaseOrderLine(w http.ResponseWriter, r *http.Request, poId openapi_types.UUID, lineId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lines, err := h.svc.ListLines(r.Context(), uuid.UUID(poId))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		var version int
		for _, l := range lines {
			if l.ID == uuid.UUID(lineId) {
				version = l.Version
				break
			}
		}
		if version == 0 {
			httpx.WriteProblem(w, r, httpx.Problem{Code: "not-found", Title: "Line not found", Status: http.StatusNotFound})
			return
		}
		archived, err := h.svc.ArchiveLine(r.Context(), uuid.UUID(lineId), version)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, archived)
	})).ServeHTTP(w, r)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
