package boards

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
)

// Domain errors.
var (
	ErrStaleBoard     = db.ErrStaleVersion
	ErrStaleColumn    = db.ErrStaleVersion
	ErrStaleCard      = db.ErrStaleVersion
	ErrStaleChecklist = db.ErrStaleVersion
)

// NotFound is a domain error for missing resources.
type NotFound struct {
	resource string
}

func (e NotFound) Error() string { return e.resource + " not found" }
func (e NotFound) ProblemError() {}

func newNotFound(resource string) NotFound { return NotFound{resource: resource} }

// Board is a kanban board shared between users.
type Board struct {
	ID         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	Archived   bool      `json:"-"`
	ArchivedAt *string   `json:"archived_at,omitempty"`
	Version    int       `json:"version"`
	CreatedAt  string    `json:"created_at"`
	UpdatedAt  string    `json:"updated_at"`
}

// Column belongs to a board and represents a swimlane (To do, In progress, …).
type Column struct {
	ID         uuid.UUID `json:"id"`
	BoardID    uuid.UUID `json:"board_id"`
	Name       string    `json:"name"`
	SortOrder  int32     `json:"sort_order"`
	Archived   bool      `json:"-"`
	ArchivedAt *string   `json:"archived_at,omitempty"`
	Version    int       `json:"version"`
	CreatedAt  string    `json:"created_at"`
	UpdatedAt  string    `json:"updated_at"`
}

// Card lives in a column. Labels is a JSON array of {name, color}.
type Card struct {
	ID            uuid.UUID       `json:"id"`
	BoardID       uuid.UUID       `json:"board_id"`
	ColumnID      uuid.UUID       `json:"column_id"`
	Title         string          `json:"title"`
	DescriptionMD *string         `json:"description_md,omitempty"`
	AssigneeID    *uuid.UUID      `json:"assignee_id,omitempty"`
	DueDate       *string         `json:"due_date,omitempty"`
	Labels        json.RawMessage `json:"labels"`
	Archived      bool            `json:"-"`
	ArchivedAt    *string         `json:"archived_at,omitempty"`
	Version       int             `json:"version"`
	CreatedAt     string          `json:"created_at"`
	UpdatedAt     string          `json:"updated_at"`
}

// ChecklistItem belongs to a card.
type ChecklistItem struct {
	ID         uuid.UUID `json:"id"`
	CardID     uuid.UUID `json:"card_id"`
	Title      string    `json:"title"`
	Completed  bool      `json:"completed"`
	SortOrder  int32     `json:"sort_order"`
	Archived   bool      `json:"-"`
	ArchivedAt *string   `json:"archived_at,omitempty"`
	Version    int       `json:"version"`
	CreatedAt  string    `json:"created_at"`
	UpdatedAt  string    `json:"updated_at"`
}

// Comment belongs to a card, with the author's user ID.
type Comment struct {
	ID         uuid.UUID `json:"id"`
	CardID     uuid.UUID `json:"card_id"`
	BodyMD     string    `json:"body_md"`
	AuthorID   uuid.UUID `json:"author_id"`
	Archived   bool      `json:"-"`
	ArchivedAt *string   `json:"archived_at,omitempty"`
	Version    int       `json:"version"`
	CreatedAt  string    `json:"created_at"`
	UpdatedAt  string    `json:"updated_at"`
}

// ProblemError returns a Problem for the domain error.
func ProblemError(err error) error {
	switch {
	case errors.Is(err, newNotFound("board")):
		return fmt.Errorf("%w: %w", db.ErrNotFound, errors.New("board not found"))
	case errors.Is(err, newNotFound("column")):
		return fmt.Errorf("%w: %w", db.ErrNotFound, errors.New("column not found"))
	case errors.Is(err, newNotFound("card")):
		return fmt.Errorf("%w: %w", db.ErrNotFound, errors.New("card not found"))
	default:
		return err
	}
}
