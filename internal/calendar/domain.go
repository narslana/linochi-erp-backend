package calendar

import (
	"errors"
	"fmt"

	"github.com/google/uuid"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
)

// Domain errors.
var (
	ErrStaleICalSource = db.ErrStaleVersion
	ErrStaleEvent      = db.ErrStaleVersion
)

// NotFound is a domain error for missing resources.
type NotFound struct {
	resource string
}

func (e NotFound) Error() string { return e.resource + " not found" }
func (e NotFound) ProblemError() {}

func newNotFound(resource string) NotFound { return NotFound{resource: resource} }

// ICalSource represents a Google calendar iCal feed (KAL-01).
type ICalSource struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	NameEn       string    `json:"name_en"`
	ICalURL      string    `json:"ical_url"`
	Color        string    `json:"color"`
	SyncInterval int       `json:"sync_interval"`
	Enabled      bool      `json:"enabled"`
	Archived     bool      `json:"-"`
	ArchivedAt   *string   `json:"archived_at,omitempty"`
	Version      int       `json:"version"`
	CreatedAt    string    `json:"created_at"`
	UpdatedAt    string    `json:"updated_at"`
}

// CalendarEvent is an event from iCal or an internal ERP source (KAL-01, KAL-03).
type CalendarEvent struct {
	ID          uuid.UUID  `json:"id"`
	SourceType  string     `json:"source_type"`
	SourceID    *uuid.UUID `json:"source_id,omitempty"`
	ICalUID     *string    `json:"ical_uid,omitempty"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	StartTime   string     `json:"start_time"`
	EndTime     string     `json:"end_time"`
	AllDay      bool       `json:"all_day"`
	Color       string     `json:"color"`
	ERPLinkType *string    `json:"erp_link_type,omitempty"`
	ERPLinkID   *string    `json:"erp_link_id,omitempty"`
	Archived    bool       `json:"-"`
	ArchivedAt  *string    `json:"archived_at,omitempty"`
	Version     int        `json:"version"`
	CreatedAt   string     `json:"created_at"`
	UpdatedAt   string     `json:"updated_at"`
}

// ProblemError returns a Problem for the domain error.
func ProblemError(err error) error {
	switch {
	case errors.Is(err, newNotFound("ical_source")):
		return fmt.Errorf("%w: %w", db.ErrNotFound, errors.New("ical source not found"))
	case errors.Is(err, newNotFound("event")):
		return fmt.Errorf("%w: %w", db.ErrNotFound, errors.New("calendar event not found"))
	default:
		return err
	}
}
