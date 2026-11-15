package reminders

import (
	"errors"
	"fmt"

	"github.com/google/uuid"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
)

// Domain errors.
var (
	ErrStaleReminder = db.ErrStaleVersion
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
	case errors.Is(err, newNotFound("reminder")):
		return fmt.Errorf("%w: %w", db.ErrNotFound, errors.New("reminder not found"))
	default:
		return err
	}
}

// Reminder is a deadline or follow-up task (REG-05).
type Reminder struct {
	ID             uuid.UUID `json:"id"`
	ReminderType   string    `json:"reminder_type"`
	IdsEntryID     *uuid.UUID `json:"ids_entry_id,omitempty"`
	ExternalRefType string   `json:"external_ref_type,omitempty"`
	ExternalRefID  *uuid.UUID `json:"external_ref_id,omitempty"`
	Title          string    `json:"title"`
	TitleEn        string    `json:"title_en"`
	Description    string    `json:"description,omitempty"`
	DueDate        string    `json:"due_date"`
	Notified       bool      `json:"notified"`
	NotifiedAt     *string   `json:"notified_at,omitempty"`
	Archived       bool      `json:"-"`
	ArchivedAt     *string   `json:"archived_at,omitempty"`
	Version        int       `json:"version"`
	CreatedAt      string    `json:"created_at"`
	UpdatedAt      string    `json:"updated_at"`
}
