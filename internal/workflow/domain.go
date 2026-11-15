package workflow

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
)

// Domain errors.
var (
	ErrStaleStep       = db.ErrStaleVersion
	ErrStaleTransition = db.ErrStaleVersion
	ErrStaleCondition  = db.ErrStaleVersion
	ErrStaleEffect     = db.ErrStaleVersion
)

// NotFound is a domain error for missing resources.
type NotFound struct {
	resource string
}

func (e NotFound) Error() string { return e.resource + " not found" }
func (e NotFound) ProblemError() {}

func newNotFound(resource string) NotFound { return NotFound{resource: resource} }

// ProblemError returns a Problem for the domain error.
func ProblemError(err error) error {
	switch {
	case errors.Is(err, newNotFound("step")):
		return fmt.Errorf("%w: %w", db.ErrNotFound, errors.New("workflow step not found"))
	case errors.Is(err, newNotFound("transition")):
		return fmt.Errorf("%w: %w", db.ErrNotFound, errors.New("workflow transition not found"))
	case errors.Is(err, newNotFound("condition")):
		return fmt.Errorf("%w: %w", db.ErrNotFound, errors.New("workflow condition not found"))
	case errors.Is(err, newNotFound("effect")):
		return fmt.Errorf("%w: %w", db.ErrNotFound, errors.New("workflow effect not found"))
	default:
		return err
	}
}

// Step is a stage in the order workflow (WF-01).
type Step struct {
	ID         uuid.UUID `json:"id"`
	Key        string    `json:"key"`
	Name       string    `json:"name"`
	NameEn     string    `json:"name_en"`
	SortOrder  int       `json:"sort_order"`
	IsStart    bool      `json:"is_start"`
	IsEnd      bool      `json:"is_end"`
	Archived   bool      `json:"-"`
	ArchivedAt *string   `json:"archived_at,omitempty"`
	Version    int       `json:"version"`
	CreatedAt  string    `json:"created_at"`
	UpdatedAt  string    `json:"updated_at"`
}

// Transition defines allowed moves between steps (WF-01).
type Transition struct {
	ID         uuid.UUID `json:"id"`
	StepKey    string    `json:"step_key"`
	FromKey    string    `json:"from_key"`
	ToKey      string    `json:"to_key"`
	Label      string    `json:"label"`
	LabelEn    string    `json:"label_en"`
	SortOrder  int       `json:"sort_order"`
	Archived   bool      `json:"-"`
	ArchivedAt *string   `json:"archived_at,omitempty"`
	Version    int       `json:"version"`
	CreatedAt  string    `json:"created_at"`
	UpdatedAt  string    `json:"updated_at"`
}

// Condition gates a transition (WF-03).
type Condition struct {
	ID           uuid.UUID       `json:"id"`
	TransitionID uuid.UUID       `json:"transition_id"`
	Kind         string          `json:"kind"`
	Config       json.RawMessage `json:"config"`
	Archived     bool            `json:"-"`
	ArchivedAt   *string         `json:"archived_at,omitempty"`
	Version      int             `json:"version"`
	CreatedAt    string          `json:"created_at"`
	UpdatedAt    string          `json:"updated_at"`
}

// Effect triggers on a transition (WF-04).
type Effect struct {
	ID           uuid.UUID       `json:"id"`
	TransitionID uuid.UUID       `json:"transition_id"`
	Kind         string          `json:"kind"`
	Config       json.RawMessage `json:"config"`
	Archived     bool            `json:"-"`
	ArchivedAt   *string         `json:"archived_at,omitempty"`
	Version      int             `json:"version"`
	CreatedAt    string          `json:"created_at"`
	UpdatedAt    string          `json:"updated_at"`
}
