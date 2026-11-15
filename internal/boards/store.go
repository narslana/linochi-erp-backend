package boards

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db/dbgen"
)

// Store wraps sqlc queries for the boards module.
type Store struct {
	q *dbgen.Queries
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{q: dbgen.New(pool)}
}

// ─── Boards ─────────────────────────────────────────────────────────────────

func (s *Store) ListBoards(ctx context.Context) ([]Board, error) {
	rows, err := s.q.ListBoards(ctx)
	if err != nil {
		return nil, err
	}
	boards := make([]Board, 0, len(rows))
	for _, r := range rows {
		boards = append(boards, dbRowToBoard(r))
	}
	return boards, nil
}

func (s *Store) GetBoard(ctx context.Context, id uuid.UUID) (Board, error) {
	row, err := s.q.GetBoard(ctx, id)
	if err != nil {
		return Board{}, err
	}
	return dbRowToBoard(row), nil
}

func (s *Store) CreateBoard(ctx context.Context, id uuid.UUID, name string) (Board, error) {
	row, err := s.q.CreateBoard(ctx, dbgen.CreateBoardParams{ID: id, Name: name})
	if err != nil {
		return Board{}, err
	}
	return dbRowToBoard(row), nil
}

func (s *Store) UpdateBoard(ctx context.Context, name string, id uuid.UUID, version int) (Board, error) {
	row, err := s.q.UpdateBoard(ctx, dbgen.UpdateBoardParams{Name: name, ID: id, Version: int32(version)}) //nolint:gosec // version is bounded by optimistic locking
	if err != nil {
		return Board{}, err
	}
	return dbRowToBoard(row), nil
}

func (s *Store) ArchiveBoard(ctx context.Context, id uuid.UUID) (Board, error) {
	row, err := s.q.ArchiveBoard(ctx, id)
	if err != nil {
		return Board{}, err
	}
	return dbRowToBoard(row), nil
}

// ─── Columns ────────────────────────────────────────────────────────────────

func (s *Store) ListColumnsForBoard(ctx context.Context, boardID uuid.UUID) ([]Column, error) {
	rows, err := s.q.ListColumnsForBoard(ctx, boardID)
	if err != nil {
		return nil, err
	}
	cols := make([]Column, 0, len(rows))
	for _, r := range rows {
		cols = append(cols, dbRowToColumn(r))
	}
	return cols, nil
}

func (s *Store) GetColumn(ctx context.Context, id uuid.UUID) (Column, error) {
	row, err := s.q.GetColumn(ctx, id)
	if err != nil {
		return Column{}, err
	}
	return dbRowToColumn(row), nil
}

func (s *Store) CreateColumn(ctx context.Context, id, boardID uuid.UUID, name string, sortOrder int32) (Column, error) {
	row, err := s.q.CreateColumn(ctx, dbgen.CreateColumnParams{ID: id, BoardID: boardID, Name: name, SortOrder: sortOrder})
	if err != nil {
		return Column{}, err
	}
	return dbRowToColumn(row), nil
}

func (s *Store) UpdateColumn(ctx context.Context, name string, sortOrder int32, id uuid.UUID, version int) (Column, error) {
	row, err := s.q.UpdateColumn(ctx, dbgen.UpdateColumnParams{Name: name, SortOrder: sortOrder, ID: id, Version: int32(version)}) //nolint:gosec // version is bounded by optimistic locking
	if err != nil {
		return Column{}, err
	}
	return dbRowToColumn(row), nil
}

func (s *Store) ArchiveColumn(ctx context.Context, id uuid.UUID) (Column, error) {
	row, err := s.q.ArchiveColumn(ctx, id)
	if err != nil {
		return Column{}, err
	}
	return dbRowToColumn(row), nil
}

// ─── Cards ──────────────────────────────────────────────────────────────────

func (s *Store) ListCardsForBoard(ctx context.Context, boardID uuid.UUID) ([]Card, error) {
	rows, err := s.q.ListCardsForBoard(ctx, boardID)
	if err != nil {
		return nil, err
	}
	cards := make([]Card, 0, len(rows))
	for _, r := range rows {
		cards = append(cards, dbRowToCard(r))
	}
	return cards, nil
}

func (s *Store) ListCardsForColumn(ctx context.Context, columnID uuid.UUID) ([]Card, error) {
	rows, err := s.q.ListCardsForColumn(ctx, columnID)
	if err != nil {
		return nil, err
	}
	cards := make([]Card, 0, len(rows))
	for _, r := range rows {
		cards = append(cards, dbRowToCard(r))
	}
	return cards, nil
}

func (s *Store) GetCard(ctx context.Context, id uuid.UUID) (Card, error) {
	row, err := s.q.GetCard(ctx, id)
	if err != nil {
		return Card{}, err
	}
	return dbRowToCard(row), nil
}

func (s *Store) CreateCard(ctx context.Context, id, boardID, columnID uuid.UUID, title string, descMd *string, assigneeID *uuid.UUID, dueDate *time.Time, labels json.RawMessage) (Card, error) {
	var due pgtype.Date
	if dueDate != nil {
		due = pgtype.Date{Time: *dueDate, Valid: true}
	}
	row, err := s.q.CreateCard(ctx, dbgen.CreateCardParams{
		ID:            id,
		BoardID:       boardID,
		ColumnID:      columnID,
		Title:         title,
		DescriptionMd: descMd,
		AssigneeID:    assigneeID,
		DueDate:       due,
		Labels:        labels,
	})
	if err != nil {
		return Card{}, err
	}
	return dbRowToCard(row), nil
}

func (s *Store) UpdateCard(ctx context.Context, title string, columnID uuid.UUID, descMd *string, assigneeID *uuid.UUID, dueDate *time.Time, labels json.RawMessage, id uuid.UUID, version int) (Card, error) {
	var due pgtype.Date
	if dueDate != nil {
		due = pgtype.Date{Time: *dueDate, Valid: true}
	}
	row, err := s.q.UpdateCard(ctx, dbgen.UpdateCardParams{
		Title:         title,
		ColumnID:      columnID,
		DescriptionMd: descMd,
		AssigneeID:    assigneeID,
		DueDate:       due,
		Labels:        labels,
		ID:            id,
		Version:       int32(version), //nolint:gosec // version is bounded by optimistic locking
	})
	if err != nil {
		return Card{}, err
	}
	return dbRowToCard(row), nil
}

func (s *Store) ArchiveCard(ctx context.Context, id uuid.UUID) (Card, error) {
	row, err := s.q.ArchiveCard(ctx, id)
	if err != nil {
		return Card{}, err
	}
	return dbRowToCard(row), nil
}

// ─── Checklist Items ────────────────────────────────────────────────────────

func (s *Store) GetChecklistItem(ctx context.Context, id uuid.UUID) (ChecklistItem, error) {
	row, err := s.q.GetChecklistItem(ctx, id)
	if err != nil {
		return ChecklistItem{}, err
	}
	return dbRowToChecklistItem(row), nil
}

func (s *Store) ListChecklistItemsForCard(ctx context.Context, cardID uuid.UUID) ([]ChecklistItem, error) {
	rows, err := s.q.ListChecklistItemsForCard(ctx, cardID)
	if err != nil {
		return nil, err
	}
	items := make([]ChecklistItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, dbRowToChecklistItem(r))
	}
	return items, nil
}

func (s *Store) CreateChecklistItem(ctx context.Context, id, cardID uuid.UUID, title string, completed bool, sortOrder int32) (ChecklistItem, error) {
	row, err := s.q.CreateChecklistItem(ctx, dbgen.CreateChecklistItemParams{ID: id, CardID: cardID, Title: title, Completed: completed, SortOrder: sortOrder})
	if err != nil {
		return ChecklistItem{}, err
	}
	return dbRowToChecklistItem(row), nil
}

func (s *Store) UpdateChecklistItem(ctx context.Context, title string, completed bool, sortOrder int32, id uuid.UUID, version int) (ChecklistItem, error) {
	row, err := s.q.UpdateChecklistItem(ctx, dbgen.UpdateChecklistItemParams{Title: title, Completed: completed, SortOrder: sortOrder, ID: id, Version: int32(version)}) //nolint:gosec // version is bounded by optimistic locking
	if err != nil {
		return ChecklistItem{}, err
	}
	return dbRowToChecklistItem(row), nil
}

func (s *Store) ArchiveChecklistItem(ctx context.Context, id uuid.UUID) (ChecklistItem, error) {
	row, err := s.q.ArchiveChecklistItem(ctx, id)
	if err != nil {
		return ChecklistItem{}, err
	}
	return dbRowToChecklistItem(row), nil
}

// ─── Comments ───────────────────────────────────────────────────────────────

func (s *Store) ListCommentsForCard(ctx context.Context, cardID uuid.UUID) ([]Comment, error) {
	rows, err := s.q.ListCommentsForCard(ctx, cardID)
	if err != nil {
		return nil, err
	}
	comments := make([]Comment, 0, len(rows))
	for _, r := range rows {
		comments = append(comments, dbRowToComment(r))
	}
	return comments, nil
}

func (s *Store) CreateComment(ctx context.Context, id, cardID, authorID uuid.UUID, bodyMd string) (Comment, error) {
	row, err := s.q.CreateComment(ctx, dbgen.CreateCommentParams{ID: id, CardID: cardID, BodyMd: bodyMd, AuthorID: authorID})
	if err != nil {
		return Comment{}, err
	}
	return dbRowToComment(row), nil
}

// ─── Row helpers ────────────────────────────────────────────────────────────

func dbRowToBoard(r dbgen.Board) Board {
	b := Board{
		ID:        r.ID,
		Name:      r.Name,
		Version:   int(r.Version),
		CreatedAt: r.CreatedAt.Format(time.RFC3339),
		UpdatedAt: r.UpdatedAt.Format(time.RFC3339),
	}
	if r.ArchivedAt != nil {
		s := r.ArchivedAt.Format(time.RFC3339)
		b.Archived = true
		b.ArchivedAt = &s
	}
	return b
}

func dbRowToColumn(r dbgen.BoardColumn) Column {
	c := Column{
		ID:        r.ID,
		BoardID:   r.BoardID,
		Name:      r.Name,
		SortOrder: r.SortOrder,
		Version:   int(r.Version),
		CreatedAt: r.CreatedAt.Format(time.RFC3339),
		UpdatedAt: r.UpdatedAt.Format(time.RFC3339),
	}
	if r.ArchivedAt != nil {
		s := r.ArchivedAt.Format(time.RFC3339)
		c.Archived = true
		c.ArchivedAt = &s
	}
	return c
}

func dbRowToCard(r dbgen.Card) Card {
	c := Card{
		ID:        r.ID,
		BoardID:   r.BoardID,
		ColumnID:  r.ColumnID,
		Title:     r.Title,
		Version:   int(r.Version),
		CreatedAt: r.CreatedAt.Format(time.RFC3339),
		UpdatedAt: r.UpdatedAt.Format(time.RFC3339),
	}
	if r.DescriptionMd != nil {
		c.DescriptionMD = r.DescriptionMd
	}
	if r.AssigneeID != nil {
		c.AssigneeID = r.AssigneeID
	}
	if r.DueDate.Valid {
		s := r.DueDate.Time.Format("2006-01-02")
		c.DueDate = &s
	}
	c.Labels = r.Labels
	if r.ArchivedAt != nil {
		s := r.ArchivedAt.Format(time.RFC3339)
		c.Archived = true
		c.ArchivedAt = &s
	}
	return c
}

func dbRowToChecklistItem(r dbgen.CardChecklistItem) ChecklistItem {
	c := ChecklistItem{
		ID:        r.ID,
		CardID:    r.CardID,
		Title:     r.Title,
		Completed: r.Completed,
		SortOrder: r.SortOrder,
		Version:   int(r.Version),
		CreatedAt: r.CreatedAt.Format(time.RFC3339),
		UpdatedAt: r.UpdatedAt.Format(time.RFC3339),
	}
	if r.ArchivedAt != nil {
		s := r.ArchivedAt.Format(time.RFC3339)
		c.Archived = true
		c.ArchivedAt = &s
	}
	return c
}

func dbRowToComment(r dbgen.CardComment) Comment {
	c := Comment{
		ID:        r.ID,
		CardID:    r.CardID,
		BodyMD:    r.BodyMd,
		AuthorID:  r.AuthorID,
		Version:   int(r.Version),
		CreatedAt: r.CreatedAt.Format(time.RFC3339),
		UpdatedAt: r.UpdatedAt.Format(time.RFC3339),
	}
	if r.ArchivedAt != nil {
		s := r.ArchivedAt.Format(time.RFC3339)
		c.Archived = true
		c.ArchivedAt = &s
	}
	return c
}
