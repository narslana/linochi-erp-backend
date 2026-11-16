package boards

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/api"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/audit"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/auth"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/httpx"
)

// Handler implements api.ServerInterface for the boards module.
type Handler struct {
	svc    *Service
	mw     *auth.SessionMiddleware
	logger *slog.Logger
}

// NewHandler creates a boards handler.
func NewHandler(svc *Service, mw *auth.SessionMiddleware, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, mw: mw, logger: logger}
}

// ─── Boards ─────────────────────────────────────────────────────────────────

func (h *Handler) ListBoards(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.listBoards)).ServeHTTP(w, r)
}

func (h *Handler) listBoards(w http.ResponseWriter, r *http.Request) {
	boards, err := h.svc.ListBoards(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, boards)
}

func (h *Handler) CreateBoard(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.createBoard)).ServeHTTP(w, r)
}

func (h *Handler) createBoard(w http.ResponseWriter, r *http.Request) {
	var body api.CreateBoardJSONBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteProblem(w, r, httpx.Problem{Code: "invalid-request", Title: "Invalid request body", Status: http.StatusBadRequest})
		return
	}

	ctx := r.Context()
	actor, _ := audit.ActorFrom(ctx)
	ctx = audit.WithActor(ctx, actor)

	board, err := h.svc.CreateBoard(ctx, body.Name)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}

	w.Header().Set("Location", "/api/v1/boards/"+board.ID.String())
	writeJSON(w, http.StatusCreated, board)
}

func (h *Handler) GetBoard(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		board, err := h.svc.GetBoard(r.Context(), uuid.UUID(boardId))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, board)
	})).ServeHTTP(w, r)
}

func (h *Handler) UpdateBoard(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body api.UpdateBoardJSONBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, httpx.Problem{Code: "invalid-request", Title: "Invalid request body", Status: http.StatusBadRequest})
			return
		}

		board, err := h.svc.UpdateBoard(r.Context(), uuid.UUID(boardId), body.Name, body.Version)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, board)
	})).ServeHTTP(w, r)
}

func (h *Handler) ArchiveBoard(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		board, err := h.svc.ArchiveBoard(r.Context(), uuid.UUID(boardId))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, board)
	})).ServeHTTP(w, r)
}

// ─── Columns ────────────────────────────────────────────────────────────────

func (h *Handler) ListColumns(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cols, err := h.svc.ListColumns(r.Context(), uuid.UUID(boardId))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, cols)
	})).ServeHTTP(w, r)
}

func (h *Handler) CreateColumn(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body api.CreateColumnJSONBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, httpx.Problem{Code: "invalid-request", Title: "Invalid request body", Status: http.StatusBadRequest})
			return
		}

		var sortOrder *int32
		if body.SortOrder != nil {
			v := int32(*body.SortOrder) //nolint:gosec // sort order is bounded
			sortOrder = &v
		}

		col, err := h.svc.CreateColumn(r.Context(), uuid.UUID(boardId), body.Name, sortOrder)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}

		w.Header().Set("Location", "/api/v1/boards/"+boardId.String()+"/columns/"+col.ID.String())
		writeJSON(w, http.StatusCreated, col)
	})).ServeHTTP(w, r)
}

func (h *Handler) UpdateColumn(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID, columnId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body api.UpdateColumnJSONBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, httpx.Problem{Code: "invalid-request", Title: "Invalid request body", Status: http.StatusBadRequest})
			return
		}

		var name *string
		if body.Name != nil {
			v := *body.Name
			name = &v
		}
		var sortOrder *int32
		if body.SortOrder != nil {
			v := int32(*body.SortOrder) //nolint:gosec // sort order is bounded
			sortOrder = &v
		}

		var version int
		if body.Version != nil {
			version = int(*body.Version)
		} else {
			col, err := h.svc.GetColumn(r.Context(), uuid.UUID(columnId))
			if err != nil {
				httpx.Error(w, r, err)
				return
			}
			version = col.Version
		}

		col, err := h.svc.UpdateColumn(r.Context(), uuid.UUID(columnId), name, sortOrder, version)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, col)
	})).ServeHTTP(w, r)
}

func (h *Handler) ArchiveColumn(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID, columnId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		col, err := h.svc.ArchiveColumn(r.Context(), uuid.UUID(columnId))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, col)
	})).ServeHTTP(w, r)
}

// ─── Cards ──────────────────────────────────────────────────────────────────

func (h *Handler) ListCards(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cards, err := h.svc.ListCardsForBoard(r.Context(), uuid.UUID(boardId))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, cards)
	})).ServeHTTP(w, r)
}

func (h *Handler) CreateCard(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body api.CreateCardJSONBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, httpx.Problem{Code: "invalid-request", Title: "Invalid request body", Status: http.StatusBadRequest})
			return
		}

		var descMd *string
		if body.DescriptionMd != nil {
			v := *body.DescriptionMd
			descMd = &v
		}

		var assigneeID *uuid.UUID
		if body.AssigneeId != nil {
			v := uuid.UUID(*body.AssigneeId)
			assigneeID = &v
		}

		var dueDate *string
		if body.DueDate != nil {
			v := body.DueDate.Format("2006-01-02")
			dueDate = &v
		}

		var labels json.RawMessage = []byte("[]")
		if body.Labels != nil {
			labels, _ = json.Marshal(body.Labels)
		}

		card, err := h.svc.CreateCard(r.Context(), uuid.UUID(boardId), uuid.UUID(body.ColumnId), body.Title, descMd, assigneeID, dueDate, labels)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}

		w.Header().Set("Location", "/api/v1/boards/"+boardId.String()+"/cards/"+card.ID.String())
		writeJSON(w, http.StatusCreated, card)
	})).ServeHTTP(w, r)
}

func (h *Handler) GetCard(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID, cardId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		card, err := h.svc.GetCard(r.Context(), uuid.UUID(cardId))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, card)
	})).ServeHTTP(w, r)
}

func (h *Handler) UpdateCard(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID, cardId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body api.UpdateCardJSONBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, httpx.Problem{Code: "invalid-request", Title: "Invalid request body", Status: http.StatusBadRequest})
			return
		}

		var version int
		if body.Version != nil {
			version = int(*body.Version)
		} else {
			card, err := h.svc.GetCard(r.Context(), uuid.UUID(cardId))
			if err != nil {
				httpx.Error(w, r, err)
				return
			}
			version = card.Version
		}

		var title *string
		if body.Title != nil {
			v := *body.Title
			title = &v
		}

		var columnID *uuid.UUID
		if body.ColumnId != nil {
			v := uuid.UUID(*body.ColumnId)
			columnID = &v
		}

		var descMd *string
		if body.DescriptionMd != nil {
			v := *body.DescriptionMd
			descMd = &v
		}

		var assigneeID *uuid.UUID
		if body.AssigneeId != nil {
			v := uuid.UUID(*body.AssigneeId)
			assigneeID = &v
		}

		var dueDate *string
		if body.DueDate != nil {
			v := body.DueDate.Format("2006-01-02")
			dueDate = &v
		}

		var labels json.RawMessage
		if body.Labels != nil {
			labels, _ = json.Marshal(body.Labels)
		} else {
			labels = []byte("[]")
		}

		card, err := h.svc.UpdateCard(r.Context(), uuid.UUID(cardId), title, columnID, descMd, assigneeID, dueDate, labels, version)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, card)
	})).ServeHTTP(w, r)
}

func (h *Handler) ArchiveCard(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID, cardId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		card, err := h.svc.ArchiveCard(r.Context(), uuid.UUID(cardId))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, card)
	})).ServeHTTP(w, r)
}

// ─── Checklist Items ────────────────────────────────────────────────────────

func (h *Handler) ListChecklistItems(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID, cardId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		items, err := h.svc.ListChecklistItems(r.Context(), uuid.UUID(cardId))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, items)
	})).ServeHTTP(w, r)
}

func (h *Handler) CreateChecklistItem(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID, cardId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body api.CreateChecklistItemJSONBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, httpx.Problem{Code: "invalid-request", Title: "Invalid request body", Status: http.StatusBadRequest})
			return
		}

		var sortOrder *int32
		if body.SortOrder != nil {
			v := int32(*body.SortOrder) //nolint:gosec // sort order is bounded
			sortOrder = &v
		}

		item, err := h.svc.CreateChecklistItem(r.Context(), uuid.UUID(cardId), body.Title, sortOrder)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}

		w.Header().Set("Location", "/api/v1/boards/"+boardId.String()+"/cards/"+cardId.String()+"/checklist/"+item.ID.String())
		writeJSON(w, http.StatusCreated, item)
	})).ServeHTTP(w, r)
}

func (h *Handler) UpdateChecklistItem(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID, cardId openapi_types.UUID, itemId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body api.UpdateChecklistItemJSONBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, httpx.Problem{Code: "invalid-request", Title: "Invalid request body", Status: http.StatusBadRequest})
			return
		}

		var title *string
		if body.Title != nil {
			v := *body.Title
			title = &v
		}

		var completed *bool
		if body.Completed != nil {
			v := bool(*body.Completed)
			completed = &v
		}

		var sortOrder *int32
		if body.SortOrder != nil {
			v := int32(*body.SortOrder) //nolint:gosec // sort order is bounded
			sortOrder = &v
		}

		var version int
		if body.Version != nil {
			version = int(*body.Version)
		} else {
			items, err := h.svc.ListChecklistItems(r.Context(), uuid.UUID(cardId))
			if err != nil {
				httpx.Error(w, r, err)
				return
			}
			for _, it := range items {
				if it.ID == uuid.UUID(itemId) {
					version = it.Version
					break
				}
			}
			if version == 0 {
				httpx.WriteProblem(w, r, httpx.Problem{Code: "not-found", Title: "Checklist item not found", Status: http.StatusNotFound})
				return
			}
		}

		item, err := h.svc.UpdateChecklistItem(r.Context(), uuid.UUID(itemId), title, completed, sortOrder, version)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	})).ServeHTTP(w, r)
}

func (h *Handler) ArchiveChecklistItem(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID, cardId openapi_types.UUID, itemId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		item, err := h.svc.ArchiveChecklistItem(r.Context(), uuid.UUID(itemId))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	})).ServeHTTP(w, r)
}

// ─── Comments ───────────────────────────────────────────────────────────────

func (h *Handler) ListComments(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID, cardId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		comments, err := h.svc.ListComments(r.Context(), uuid.UUID(cardId))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, comments)
	})).ServeHTTP(w, r)
}

func (h *Handler) CreateComment(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID, cardId openapi_types.UUID) {
	h.mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body api.CreateCommentJSONBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, httpx.Problem{Code: "invalid-request", Title: "Invalid request body", Status: http.StatusBadRequest})
			return
		}

		actor, _ := audit.ActorFrom(r.Context())
		comment, err := h.svc.CreateComment(r.Context(), uuid.UUID(cardId), uuid.UUID(actor.ID), body.BodyMd)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}

		w.Header().Set("Location", "/api/v1/boards/"+boardId.String()+"/cards/"+cardId.String()+"/comments/"+comment.ID.String())
		writeJSON(w, http.StatusCreated, comment)
	})).ServeHTTP(w, r)
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
