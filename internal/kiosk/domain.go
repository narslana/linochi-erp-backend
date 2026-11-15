package kiosk

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
)

// Domain errors.
var (
	ErrStalePlaylist     = db.ErrStaleVersion
	ErrStalePlaylistView = db.ErrStaleVersion
	ErrStaleDevice       = db.ErrStaleVersion
)

// NotFound is a domain error for missing resources.
type NotFound struct {
	resource string
}

func (e NotFound) Error() string { return e.resource + " not found" }
func (e NotFound) ProblemError() {}

func newNotFound(resource string) NotFound { return NotFound{resource: resource} }

// Playlist is a kiosk playlist (KIO-01).
type Playlist struct {
	ID         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	NameEn     string    `json:"name_en"`
	Archived   bool      `json:"-"`
	ArchivedAt *string   `json:"archived_at,omitempty"`
	Version    int       `json:"version"`
	CreatedAt  string    `json:"created_at"`
	UpdatedAt  string    `json:"updated_at"`
}

// PlaylistView is a view within a playlist (KIO-01, KIO-04).
type PlaylistView struct {
	ID          uuid.UUID       `json:"id"`
	PlaylistID  uuid.UUID       `json:"playlist_id"`
	ViewType    string          `json:"view_type"`
	SortOrder   int32           `json:"sort_order"`
	DurationSec int32           `json:"duration_sec"`
	Settings    json.RawMessage `json:"settings"`
	Archived    bool            `json:"-"`
	ArchivedAt  *string         `json:"archived_at,omitempty"`
	Version     int             `json:"version"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
}

// KioskDevice is a fixed kiosk device (KIO-06).
type KioskDevice struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	PairingCode string    `json:"pairing_code"`
	Approved    bool      `json:"approved"`
	PlaylistID  *string   `json:"playlist_id,omitempty"`
	ExpiresAt   *string   `json:"expires_at,omitempty"`
	LanOnly     bool      `json:"lan_only"`
	Archived    bool      `json:"-"`
	ArchivedAt  *string   `json:"archived_at,omitempty"`
	Version     int       `json:"version"`
	CreatedAt   string    `json:"created_at"`
	UpdatedAt   string    `json:"updated_at"`
}

// KioskData is aggregated data for kiosk views (KIO-02, KIO-03, KIO-05).
type KioskData struct {
	OrdersPerMonth   []OrderMonth       `json:"orders_per_month,omitempty"`
	RevenueByChannel []RevenueByChannel `json:"revenue_by_channel,omitempty"`
	KeyFigures       KeyFigures         `json:"key_figures,omitempty"`
	InventoryValue   int64              `json:"inventory_value,omitempty"`
	LowStockCount    int                `json:"low_stock_count,omitempty"`
}

// OrderMonth is monthly order data.
type OrderMonth struct {
	Month   string `json:"month"`
	Orders  int    `json:"orders"`
	Revenue int64  `json:"revenue"`
}

// RevenueByChannel is revenue per sales channel.
type RevenueByChannel struct {
	Channel string `json:"channel"`
	Revenue int64  `json:"revenue"`
}

// KeyFigures are the large key figures shown on the kiosk.
type KeyFigures struct {
	OrdersThisYear  int   `json:"orders_this_year"`
	RevenueThisYear int64 `json:"revenue_this_year"`
	Countries       int   `json:"countries"`
	Customers       int   `json:"customers"`
}

// ProblemError returns a Problem for the domain error.
func ProblemError(err error) error {
	switch {
	case errors.Is(err, newNotFound("playlist")):
		return fmt.Errorf("%w: %w", db.ErrNotFound, errors.New("playlist not found"))
	case errors.Is(err, newNotFound("playlist_view")):
		return fmt.Errorf("%w: %w", db.ErrNotFound, errors.New("playlist view not found"))
	case errors.Is(err, newNotFound("device")):
		return fmt.Errorf("%w: %w", db.ErrNotFound, errors.New("kiosk device not found"))
	default:
		return err
	}
}
