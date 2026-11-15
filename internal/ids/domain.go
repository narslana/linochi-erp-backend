package ids

import (
	"errors"
	"fmt"

	"github.com/google/uuid"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
)

// Domain errors.
var (
	ErrStaleEntry = db.ErrStaleVersion
)

// NotFound is a domain error for missing resources.
type NotFound struct {
	resource string
}

func (e NotFound) Error() string { return e.resource + " not found" }
func (e NotFound) ProblemError() {}

func newNotFound(resource string) NotFound { return NotFound{resource: resource} }

// IdsEntry is a number or ID that needs to be at hand (REG-01, REG-02, REG-03).
type IdsEntry struct {
	ID          uuid.UUID `json:"id"`
	Category    string    `json:"category"`
	Name        string    `json:"name"`
	Value       string    `json:"value"`
	Issuer      string    `json:"issuer"`
	ValidFrom   *string   `json:"valid_from,omitempty"`
	ValidTo     *string   `json:"valid_to,omitempty"`
	Owner       string    `json:"owner"`
	Note        string    `json:"note"`
	DocumentURL *string   `json:"document_url,omitempty"`
	Version     int       `json:"version"`
	CreatedAt   string    `json:"created_at"`
	UpdatedAt   string    `json:"updated_at"`
}

// ProblemError maps domain errors to RFC 9457 problems.
func ProblemError(err error) error {
	switch {
	case errors.Is(err, newNotFound("ids_entry")):
		return fmt.Errorf("%w: %w", db.ErrNotFound, errors.New("ids entry not found"))
	default:
		return err
	}
}
