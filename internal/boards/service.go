package boards

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/audit"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
)

// Service holds the business logic for boards, columns and cards.
type Service struct {
	store *Store
	rec   *audit.Recorder
	pool  *pgxpool.Pool
}

func NewService(store *Store, rec *audit.Recorder, pool *pgxpool.Pool) *Service {
	return &Service{store: store, rec: rec, pool: pool}
}

// ListBoards returns all active boards.
func (s *Service) ListBoards(ctx context.Context) ([]Board, error) {
	return s.store.ListBoards(ctx)
}

// CreateBoard creates a new board with three default columns (TAV-02).
func (s *Service) CreateBoard(ctx context.Context, name string) (Board, error) {
	var b Board
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		id := uuid.New()
		var err error
		b, err = s.store.CreateBoard(ctx, id, name)
		if err != nil {
			return err
		}

		// Default columns: To do, In progress, Done (TAV-02).
		defaultCols := []struct {
			name  string
			order int32
		}{
			{"To do", 0},
			{"In progress", 1},
			{"Done", 2},
		}
		for _, dc := range defaultCols {
			colID := uuid.New()
			_, err = s.store.CreateColumn(ctx, colID, id, dc.name, dc.order)
			if err != nil {
				return err
			}
		}

		actor, _ := audit.ActorFrom(ctx)
		_ = s.rec.Record(ctx, tx, audit.Entry{
			Module:     "boards",
			EntityType: "board",
			EntityID:   b.ID,
			Action:     "Create",
			After:      map[string]any{"name": name},
			Reason:     actor.ID.String(),
		})
		return nil
	})
	return b, err
}

// GetBoard returns a board by ID.
func (s *Service) GetBoard(ctx context.Context, id uuid.UUID) (Board, error) {
	b, err := s.store.GetBoard(ctx, id)
	if err != nil {
		return b, ProblemError(err)
	}
	return b, nil
}

// UpdateBoard updates the board name.
func (s *Service) UpdateBoard(ctx context.Context, id uuid.UUID, name string, version int) (Board, error) {
	b, err := s.store.UpdateBoard(ctx, name, id, version)
	if err != nil {
		return b, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "boards",
		EntityType: "board",
		EntityID:   id,
		Action:     "Update",
		After:      map[string]any{"name": name},
		Reason:     actor.ID.String(),
	})
	return b, nil
}

// ArchiveBoard archives a board (NFK-09).
func (s *Service) ArchiveBoard(ctx context.Context, id uuid.UUID) (Board, error) {
	b, err := s.store.ArchiveBoard(ctx, id)
	if err != nil {
		return b, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "boards",
		EntityType: "board",
		EntityID:   id,
		Action:     "Archive",
		Before:     map[string]any{"name": b.Name},
		Reason:     actor.ID.String(),
	})
	return b, nil
}

// ListColumns returns the columns for a board, sorted by sort_order.
func (s *Service) ListColumns(ctx context.Context, boardID uuid.UUID) ([]Column, error) {
	return s.store.ListColumnsForBoard(ctx, boardID)
}

// GetColumn returns a column by ID.
func (s *Service) GetColumn(ctx context.Context, id uuid.UUID) (Column, error) {
	return s.store.GetColumn(ctx, id)
}

// CreateColumn adds a column to a board (TAV-02).
func (s *Service) CreateColumn(ctx context.Context, boardID uuid.UUID, name string, sortOrder *int32) (Column, error) {
	order := int32(0)
	if sortOrder != nil {
		order = *sortOrder
	} else {
		// Append last: get the max sort_order and add 1.
		cols, err := s.store.ListColumnsForBoard(ctx, boardID)
		if err != nil {
			return Column{}, err
		}
		for _, c := range cols {
			if c.SortOrder >= order {
				order = c.SortOrder + 1
			}
		}
	}

	colID := uuid.New()
	col, err := s.store.CreateColumn(ctx, colID, boardID, name, order)
	if err != nil {
		return col, err
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "board_columns",
		EntityType: "column",
		EntityID:   col.ID,
		Action:     "Create",
		After:      map[string]any{"name": name, "board_id": boardID},
		Reason:     actor.ID.String(),
	})
	return col, nil
}

// UpdateColumn updates the name and/or sort order of a column (TAV-02).
func (s *Service) UpdateColumn(ctx context.Context, id uuid.UUID, name *string, sortOrder *int32, version int) (Column, error) {
	col, err := s.store.GetColumn(ctx, id)
	if err != nil {
		return col, ProblemError(err)
	}

	newName := col.Name
	if name != nil {
		newName = *name
	}
	newOrder := col.SortOrder
	if sortOrder != nil {
		newOrder = *sortOrder
	}

	c, err := s.store.UpdateColumn(ctx, newName, int32(newOrder), id, version)
	if err != nil {
		return c, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "board_columns",
		EntityType: "column",
		EntityID:   id,
		Action:     "Update",
		After:      map[string]any{"name": newName, "sort_order": newOrder},
		Reason:     actor.ID.String(),
	})
	return c, nil
}

// ArchiveColumn archives a column (TAV-02).
func (s *Service) ArchiveColumn(ctx context.Context, id uuid.UUID) (Column, error) {
	col, err := s.store.ArchiveColumn(ctx, id)
	if err != nil {
		return col, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "board_columns",
		EntityType: "column",
		EntityID:   id,
		Action:     "Archive",
		Before:     map[string]any{"name": col.Name},
		Reason:     actor.ID.String(),
	})
	return col, nil
}

// ListCardsForBoard returns all cards for a board.
func (s *Service) ListCardsForBoard(ctx context.Context, boardID uuid.UUID) ([]Card, error) {
	return s.store.ListCardsForBoard(ctx, boardID)
}

// CreateCard creates a new card in a column (TAV-03).
func (s *Service) CreateCard(ctx context.Context, boardID, columnID uuid.UUID, title string, descriptionMD *string, assigneeID *uuid.UUID, dueDate *string, labels json.RawMessage) (Card, error) {
	cardID := uuid.New()

	var due *time.Time
	if dueDate != nil && *dueDate != "" {
		d, _ := parseDate(*dueDate)
		due = &d
	}

	if labels == nil {
		labels = json.RawMessage("[]")
	}

	c, err := s.store.CreateCard(ctx, cardID, boardID, columnID, title, descriptionMD, assigneeID, due, labels)
	if err != nil {
		return c, err
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "cards",
		EntityType: "card",
		EntityID:   c.ID,
		Action:     "Create",
		After:      map[string]any{"title": title, "board_id": boardID, "column_id": columnID},
		Reason:     actor.ID.String(),
	})
	return c, nil
}

// GetCard returns a card by ID.
func (s *Service) GetCard(ctx context.Context, id uuid.UUID) (Card, error) {
	c, err := s.store.GetCard(ctx, id)
	if err != nil {
		return c, ProblemError(err)
	}
	return c, nil
}

// UpdateCard updates a card: title, column (move), description, assignee, due date and labels (TAV-04).
func (s *Service) UpdateCard(ctx context.Context, id uuid.UUID, title *string, columnID *uuid.UUID, descriptionMD *string, assigneeID *uuid.UUID, dueDate *string, labels json.RawMessage, version int) (Card, error) {
	c, err := s.store.GetCard(ctx, id)
	if err != nil {
		return c, ProblemError(err)
	}

	newTitle := c.Title
	if title != nil {
		newTitle = *title
	}
	newCol := c.ColumnID
	if columnID != nil {
		newCol = *columnID
	}
	var newDue *time.Time
	if dueDate != nil && *dueDate != "" {
		d, _ := parseDate(*dueDate)
		newDue = &d
	}

	if labels == nil {
		labels = c.Labels
	}

	card, err := s.store.UpdateCard(ctx, newTitle, newCol, descriptionMD, assigneeID, newDue, labels, id, version)
	if err != nil {
		return card, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "cards",
		EntityType: "card",
		EntityID:   id,
		Action:     "Update",
		After:      map[string]any{"title": newTitle, "column_id": newCol},
		Reason:     actor.ID.String(),
	})
	return card, nil
}

// ArchiveCard archives a card (NFK-09).
func (s *Service) ArchiveCard(ctx context.Context, id uuid.UUID) (Card, error) {
	c, err := s.store.ArchiveCard(ctx, id)
	if err != nil {
		return c, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "cards",
		EntityType: "card",
		EntityID:   id,
		Action:     "Archive",
		Before:     map[string]any{"title": c.Title},
		Reason:     actor.ID.String(),
	})
	return c, nil
}

// ListChecklistItems returns the checklist items for a card (TAV-05).
func (s *Service) ListChecklistItems(ctx context.Context, cardID uuid.UUID) ([]ChecklistItem, error) {
	return s.store.ListChecklistItemsForCard(ctx, cardID)
}

// CreateChecklistItem adds an item to a card's checklist (TAV-05).
func (s *Service) CreateChecklistItem(ctx context.Context, cardID uuid.UUID, title string, sortOrder *int32) (ChecklistItem, error) {
	order := int32(0)
	if sortOrder != nil {
		order = *sortOrder
	} else {
		items, err := s.store.ListChecklistItemsForCard(ctx, cardID)
		if err != nil {
			return ChecklistItem{}, err
		}
		for _, it := range items {
			if int32(it.SortOrder) >= order {
				order = int32(it.SortOrder) + 1
			}
		}
	}

	itemID := uuid.New()
	item, err := s.store.CreateChecklistItem(ctx, itemID, cardID, title, false, order)
	if err != nil {
		return item, err
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "card_checklist_items",
		EntityType: "checklist_item",
		EntityID:   item.ID,
		Action:     "Create",
		After:      map[string]any{"title": title, "card_id": cardID},
		Reason:     actor.ID.String(),
	})
	return item, nil
}

// UpdateChecklistItem updates a checklist item's title, completed state or order (TAV-05).
func (s *Service) UpdateChecklistItem(ctx context.Context, id uuid.UUID, title *string, completed *bool, sortOrder *int32, version int) (ChecklistItem, error) {
	item, err := s.store.GetChecklistItem(ctx, id)
	if err != nil {
		return ChecklistItem{}, ProblemError(err)
	}

	newTitle := item.Title
	if title != nil {
		newTitle = *title
	}
	newCompleted := item.Completed
	if completed != nil {
		newCompleted = *completed
	}
	newOrder := item.SortOrder
	if sortOrder != nil {
		newOrder = *sortOrder
	}

	it, err := s.store.UpdateChecklistItem(ctx, newTitle, newCompleted, int32(newOrder), id, version)
	if err != nil {
		return it, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "card_checklist_items",
		EntityType: "checklist_item",
		EntityID:   id,
		Action:     "Update",
		After:      map[string]any{"title": newTitle, "completed": newCompleted},
		Reason:     actor.ID.String(),
	})
	return it, nil
}

// ArchiveChecklistItem archives a checklist item (NFK-09).
func (s *Service) ArchiveChecklistItem(ctx context.Context, id uuid.UUID) (ChecklistItem, error) {
	it, err := s.store.ArchiveChecklistItem(ctx, id)
	if err != nil {
		return it, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "card_checklist_items",
		EntityType: "checklist_item",
		EntityID:   id,
		Action:     "Archive",
		Before:     map[string]any{"title": it.Title},
		Reason:     actor.ID.String(),
	})
	return it, nil
}

// CreateComment adds a comment to a card (TAV-06).
func (s *Service) CreateComment(ctx context.Context, cardID, authorID uuid.UUID, bodyMD string) (Comment, error) {
	commentID := uuid.New()
	c, err := s.store.CreateComment(ctx, commentID, cardID, authorID, bodyMD)
	if err != nil {
		return c, err
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "card_comments",
		EntityType: "comment",
		EntityID:   c.ID,
		Action:     "Create",
		After:      map[string]any{"card_id": cardID},
		Reason:     actor.ID.String(),
	})
	return c, nil
}

// ListComments returns all comments for a card (TAV-06).
func (s *Service) ListComments(ctx context.Context, cardID uuid.UUID) ([]Comment, error) {
	return s.store.ListCommentsForCard(ctx, cardID)
}

// parseDate parses a date string in YYYY-MM-DD format.
func parseDate(s string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse date %q: %w", s, err)
	}
	return t, nil
}
