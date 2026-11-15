package products

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

// Handler implements api.ServerInterface for the products module.
type Handler struct {
	svc    *Service
	mw     *auth.SessionMiddleware
	logger *slog.Logger
}

// NewHandler creates a products handler.
func NewHandler(svc *Service, mw *auth.SessionMiddleware, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, mw: mw, logger: logger}
}

// ─── Field Definitions ──────────────────────────────────────────────────────

func (h *Handler) ListFieldDefinitions(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.listFieldDefinitions)).ServeHTTP(w, r)
}

func (h *Handler) listFieldDefinitions(w http.ResponseWriter, r *http.Request) {
	fds, err := h.svc.ListFieldDefinitions(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, fds)
}

func (h *Handler) CreateFieldDefinition(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.createFieldDefinition)).ServeHTTP(w, r)
}

func (h *Handler) createFieldDefinition(w http.ResponseWriter, r *http.Request) {
	var body api.CreateFieldDefinitionJSONBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteProblem(w, r, httpx.Problem{Code: "invalid-request", Title: "Invalid request body", Status: http.StatusBadRequest})
		return
	}

	var choices json.RawMessage
	if body.Choices != nil {
		choices, _ = json.Marshal(body.Choices)
	}

	var helpText, unit string
	if body.HelpText != nil {
		helpText = *body.HelpText
	}
	if body.Unit != nil {
		unit = *body.Unit
	}

	fd, err := h.svc.CreateFieldDefinition(r.Context(), body.Name, body.NameEn, string(body.FieldType), body.GroupName, helpText, ptrOrBool(body.Mandatory, false), choices, unit, ptrOrBool(body.ShowOnCard, false))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/field-definitions/"+fd.ID.String())
	writeJSON(w, http.StatusCreated, fd)
}

func (h *Handler) GetFieldDefinition(w http.ResponseWriter, r *http.Request, fieldDefinitionId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fd, err := h.svc.GetFieldDefinition(r.Context(), uuid.UUID(fieldDefinitionId))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, fd)
	})).ServeHTTP(w, r)
}

func (h *Handler) UpdateFieldDefinition(w http.ResponseWriter, r *http.Request, fieldDefinitionId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body api.UpdateFieldDefinitionJSONBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, httpx.Problem{Code: "invalid-request", Title: "Invalid request body", Status: http.StatusBadRequest})
			return
		}

		var choices json.RawMessage
		if body.Choices != nil {
			choices, _ = json.Marshal(body.Choices)
		}

		name := ""
		if body.Name != nil {
			name = *body.Name
		}
		nameEn := ""
		if body.NameEn != nil {
			nameEn = *body.NameEn
		}
		groupName := ""
		if body.GroupName != nil {
			groupName = *body.GroupName
		}
		helpText := ""
		if body.HelpText != nil {
			helpText = *body.HelpText
		}
		unit := ""
		if body.Unit != nil {
			unit = *body.Unit
		}

		var fieldType string
		if body.FieldType != nil {
			fieldType = string(*body.FieldType)
		}

		fd, err := h.svc.UpdateFieldDefinition(r.Context(), uuid.UUID(fieldDefinitionId), name, nameEn, fieldType, groupName, helpText, unit, ptrOrBool(body.Mandatory, false), choices, ptrOrBool(body.ShowOnCard, false), body.Version)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, fd)
	})).ServeHTTP(w, r)
}

func (h *Handler) ArchiveFieldDefinition(w http.ResponseWriter, r *http.Request, fieldDefinitionId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fd, err := h.svc.ArchiveFieldDefinition(r.Context(), uuid.UUID(fieldDefinitionId), 0)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, fd)
	})).ServeHTTP(w, r)
}

// ─── Product Types ──────────────────────────────────────────────────────────

func (h *Handler) ListProductTypes(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.listProductTypes)).ServeHTTP(w, r)
}

func (h *Handler) listProductTypes(w http.ResponseWriter, r *http.Request) {
	pts, err := h.svc.ListProductTypes(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, pts)
}

func (h *Handler) CreateProductType(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.createProductType)).ServeHTTP(w, r)
}

func (h *Handler) createProductType(w http.ResponseWriter, r *http.Request) {
	var body api.CreateProductTypeJSONBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteProblem(w, r, httpx.Problem{Code: "invalid-request", Title: "Invalid request body", Status: http.StatusBadRequest})
		return
	}

	pt, err := h.svc.CreateProductType(r.Context(), body.Name, body.NameEn, body.TypeCode)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/product-types/"+pt.ID.String())
	writeJSON(w, http.StatusCreated, pt)
}

func (h *Handler) GetProductType(w http.ResponseWriter, r *http.Request, productTypeId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pt, err := h.svc.GetProductType(r.Context(), uuid.UUID(productTypeId))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, pt)
	})).ServeHTTP(w, r)
}

func (h *Handler) UpdateProductType(w http.ResponseWriter, r *http.Request, productTypeId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body api.UpdateProductTypeJSONBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, httpx.Problem{Code: "invalid-request", Title: "Invalid request body", Status: http.StatusBadRequest})
			return
		}

		name := ""
		if body.Name != nil {
			name = *body.Name
		}
		nameEn := ""
		if body.NameEn != nil {
			nameEn = *body.NameEn
		}
		typeCode := ""
		if body.TypeCode != nil {
			typeCode = *body.TypeCode
		}

		pt, err := h.svc.UpdateProductType(r.Context(), uuid.UUID(productTypeId), name, nameEn, typeCode, body.Version)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, pt)
	})).ServeHTTP(w, r)
}

func (h *Handler) ArchiveProductType(w http.ResponseWriter, r *http.Request, productTypeId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pt, err := h.svc.ArchiveProductType(r.Context(), uuid.UUID(productTypeId), 0)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, pt)
	})).ServeHTTP(w, r)
}

// ─── Product Type Fields ────────────────────────────────────────────────────

func (h *Handler) ListProductTypeFields(w http.ResponseWriter, r *http.Request, productTypeId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fields, err := h.svc.ListProductTypeFields(r.Context(), uuid.UUID(productTypeId))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, fields)
	})).ServeHTTP(w, r)
}

func (h *Handler) AssignFieldToProductType(w http.ResponseWriter, r *http.Request, productTypeId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body api.AssignFieldToProductTypeJSONBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, httpx.Problem{Code: "invalid-request", Title: "Invalid request body", Status: http.StatusBadRequest})
			return
		}

		var sortOrder *int32
		if body.SortOrder != nil {
			v := int32(*body.SortOrder) //nolint:gosec // sort order is bounded
			sortOrder = &v
		}

		ptf, err := h.svc.AssignFieldToProductType(r.Context(), uuid.UUID(productTypeId), uuid.UUID(body.FieldDefinitionId), sortOrder, body.MandatoryOverride)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, ptf)
	})).ServeHTTP(w, r)
}

func (h *Handler) UpdateProductTypeField(w http.ResponseWriter, r *http.Request, productTypeId openapi_types.UUID, ptfId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteProblem(w, r, httpx.Problem{Code: "not-implemented", Title: "Update product type field not yet implemented", Status: http.StatusNotImplemented})
	})).ServeHTTP(w, r)
}

func (h *Handler) ArchiveProductTypeField(w http.ResponseWriter, r *http.Request, productTypeId openapi_types.UUID, ptfId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteProblem(w, r, httpx.Problem{Code: "not-implemented", Title: "Archive product type field not yet implemented", Status: http.StatusNotImplemented})
	})).ServeHTTP(w, r)
}

// ─── Products ───────────────────────────────────────────────────────────────

func (h *Handler) ListProducts(w http.ResponseWriter, r *http.Request, params api.ListProductsParams) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var searchQuery *string
		if params.Q != nil {
			searchQuery = params.Q
		}
		products, err := h.svc.ListProducts(r.Context(), searchQuery)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, products)
	})).ServeHTTP(w, r)
}

func (h *Handler) CreateProduct(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.createProduct)).ServeHTTP(w, r)
}

func (h *Handler) createProduct(w http.ResponseWriter, r *http.Request) {
	var body api.CreateProductJSONBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteProblem(w, r, httpx.Problem{Code: "invalid-request", Title: "Invalid request body", Status: http.StatusBadRequest})
		return
	}

	var attributes json.RawMessage
	if body.Attributes != nil {
		attributes, _ = json.Marshal(body.Attributes)
	}

	var eanStr string
	if body.Ean != nil {
		eanStr = string(*body.Ean)
	}

	var wholesalePriceSEK, msrpSEK int64
	if body.WholesalePriceSek != nil {
		wholesalePriceSEK = int64(*body.WholesalePriceSek)
	}
	if body.MsrpSek != nil {
		msrpSEK = int64(*body.MsrpSek)
	}

	var wholesaleCurrency, msrpCurrency, status string
	if body.WholesaleCurrency != nil {
		wholesaleCurrency = string(*body.WholesaleCurrency)
	}
	if body.MsrpCurrency != nil {
		msrpCurrency = string(*body.MsrpCurrency)
	}
	if body.Status != nil {
		status = string(*body.Status)
	}

	shortDesc := ""
	if body.ShortDescription != nil {
		shortDesc = *body.ShortDescription
	}

	p, err := h.svc.CreateProduct(r.Context(), body.Name, body.Sku, eanStr, uuid.UUID(body.ProductTypeId), shortDesc, wholesalePriceSEK, msrpSEK, wholesaleCurrency, msrpCurrency, status, attributes)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/products/"+p.ID.String())
	writeJSON(w, http.StatusCreated, p)
}

func (h *Handler) GetProduct(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := h.svc.GetProduct(r.Context(), uuid.UUID(productId))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, p)
	})).ServeHTTP(w, r)
}

func (h *Handler) UpdateProduct(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body api.UpdateProductJSONBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, httpx.Problem{Code: "invalid-request", Title: "Invalid request body", Status: http.StatusBadRequest})
			return
		}

		var attributes json.RawMessage
		if body.Attributes != nil {
			attributes, _ = json.Marshal(body.Attributes)
		}

		var eanStr string
		if body.Ean != nil {
			eanStr = string(*body.Ean)
		}

		var wholesalePriceSEK, msrpSEK int64
		if body.WholesalePriceSek != nil {
			wholesalePriceSEK = int64(*body.WholesalePriceSek)
		}
		if body.MsrpSek != nil {
			msrpSEK = int64(*body.MsrpSek)
		}

		var wholesaleCurrency, msrpCurrency, status string
		if body.WholesaleCurrency != nil {
			wholesaleCurrency = string(*body.WholesaleCurrency)
		}
		if body.MsrpCurrency != nil {
			msrpCurrency = string(*body.MsrpCurrency)
		}
		if body.Status != nil {
			status = string(*body.Status)
		}

		name := ""
		if body.Name != nil {
			name = *body.Name
		}
		sku := ""
		if body.Sku != nil {
			sku = *body.Sku
		}
		shortDesc := ""
		if body.ShortDescription != nil {
			shortDesc = *body.ShortDescription
		}

		var productTypeID uuid.UUID
		if body.ProductTypeId != nil {
			productTypeID = uuid.UUID(*body.ProductTypeId)
		}

		p, err := h.svc.UpdateProduct(r.Context(), uuid.UUID(productId), name, sku, eanStr, productTypeID, shortDesc, wholesalePriceSEK, msrpSEK, wholesaleCurrency, msrpCurrency, status, attributes, body.Version)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, p)
	})).ServeHTTP(w, r)
}

func (h *Handler) ArchiveProduct(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := h.svc.ArchiveProduct(r.Context(), uuid.UUID(productId), 0)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, p)
	})).ServeHTTP(w, r)
}

func (h *Handler) LockProductSKU(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := h.svc.LockSKU(r.Context(), uuid.UUID(productId), 0)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, p)
	})).ServeHTTP(w, r)
}

// ─── Product Images ─────────────────────────────────────────────────────────

func (h *Handler) ListProductImages(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		images, err := h.svc.ListProductImages(r.Context(), uuid.UUID(productId))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, images)
	})).ServeHTTP(w, r)
}

func (h *Handler) CreateProductImage(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body api.CreateProductImageJSONBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, httpx.Problem{Code: "invalid-request", Title: "Invalid request body", Status: http.StatusBadRequest})
			return
		}

		var sortOrder *int32
		if body.SortOrder != nil {
			v := int32(*body.SortOrder) //nolint:gosec // sort order is bounded
			sortOrder = &v
		}

		img, err := h.svc.CreateProductImage(r.Context(), uuid.UUID(productId), body.Url, sortOrder, ptrOrBool(body.IsMain, false))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, img)
	})).ServeHTTP(w, r)
}

func (h *Handler) UpdateProductImage(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID, imageId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body api.UpdateProductImageJSONBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, httpx.Problem{Code: "invalid-request", Title: "Invalid request body", Status: http.StatusBadRequest})
			return
		}

		var sortOrder *int32
		if body.SortOrder != nil {
			v := int32(*body.SortOrder) //nolint:gosec // sort order is bounded
			sortOrder = &v
		}

		isMain := ptrOrBool(body.IsMain, false)
		img, err := h.svc.UpdateProductImage(r.Context(), uuid.UUID(imageId), *body.Url, sortOrder, &isMain, body.Version)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, img)
	})).ServeHTTP(w, r)
}

func (h *Handler) ArchiveProductImage(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID, imageId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		img, err := h.svc.ArchiveProductImage(r.Context(), uuid.UUID(imageId), 0)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, img)
	})).ServeHTTP(w, r)
}

// ─── Product Documents ──────────────────────────────────────────────────────

func (h *Handler) ListProductDocuments(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		docs, err := h.svc.ListProductDocuments(r.Context(), uuid.UUID(productId))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, docs)
	})).ServeHTTP(w, r)
}

func (h *Handler) CreateProductDocument(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body api.CreateProductDocumentJSONBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, httpx.Problem{Code: "invalid-request", Title: "Invalid request body", Status: http.StatusBadRequest})
			return
		}

		var expiresAt *time.Time
		if body.ExpiresAt != nil {
			t, err := time.Parse(time.RFC3339, body.ExpiresAt.Format(time.RFC3339))
			if err == nil {
				expiresAt = &t
			}
		}

		titleEn := ""
		if body.TitleEn != nil {
			titleEn = *body.TitleEn
		}

		doc, err := h.svc.CreateProductDocument(r.Context(), uuid.UUID(productId), body.Title, titleEn, string(body.DocumentType), body.Url, expiresAt)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, doc)
	})).ServeHTTP(w, r)
}

func (h *Handler) UpdateProductDocument(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID, docId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body api.UpdateProductDocumentJSONBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, httpx.Problem{Code: "invalid-request", Title: "Invalid request body", Status: http.StatusBadRequest})
			return
		}

		var expiresAt *time.Time
		if body.ExpiresAt != nil {
			t, err := time.Parse(time.RFC3339, body.ExpiresAt.Format(time.RFC3339))
			if err == nil {
				expiresAt = &t
			}
		}

		title := ""
		if body.Title != nil {
			title = *body.Title
		}
		titleEn := ""
		if body.TitleEn != nil {
			titleEn = *body.TitleEn
		}
		var documentType, url string
		if body.DocumentType != nil {
			documentType = *body.DocumentType
		}
		if body.Url != nil {
			url = *body.Url
		}

		doc, err := h.svc.UpdateProductDocument(r.Context(), uuid.UUID(docId), title, titleEn, documentType, url, expiresAt, body.Version)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, doc)
	})).ServeHTTP(w, r)
}

func (h *Handler) ArchiveProductDocument(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID, docId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		doc, err := h.svc.ArchiveProductDocument(r.Context(), uuid.UUID(docId), 0)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, doc)
	})).ServeHTTP(w, r)
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func ptrOrBool(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}
