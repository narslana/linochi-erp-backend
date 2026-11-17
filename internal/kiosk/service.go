package kiosk

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/audit"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
)

// Service holds the business logic for the kiosk module (KIO-01 to KIO-07).
type Service struct {
	store *Store
	rec   *audit.Recorder
	pool  *pgxpool.Pool
}

func NewService(store *Store, rec *audit.Recorder, pool *pgxpool.Pool) *Service {
	return &Service{store: store, rec: rec, pool: pool}
}

// ─── Playlists ────────────────────────────────────────────────────────────────

// ListKioskPlaylists returns all active playlists (KIO-01).
func (s *Service) ListKioskPlaylists(ctx context.Context) ([]Playlist, error) {
	return s.store.ListKioskPlaylists(ctx)
}

// GetKioskPlaylist returns a playlist by ID (KIO-01).
func (s *Service) GetKioskPlaylist(ctx context.Context, id uuid.UUID) (Playlist, error) {
	p, err := s.store.GetKioskPlaylist(ctx, id)
	if err != nil {
		return p, ProblemError(err)
	}
	return p, nil
}

// CreateKioskPlaylist creates a new playlist (KIO-01).
func (s *Service) CreateKioskPlaylist(ctx context.Context, name, nameEn string) (Playlist, error) {
	var p Playlist
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		id := uuid.New()
		var err error
		p, err = st.CreateKioskPlaylist(ctx, id, name, nameEn)
		if err != nil {
			return err
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "kiosk",
			EntityType: "playlist",
			EntityID:   p.ID,
			Action:     "Create",
			After:      map[string]any{"name": name},
		})
	})
	return p, err
}

// UpdateKioskPlaylist updates a playlist (KIO-01).
func (s *Service) UpdateKioskPlaylist(ctx context.Context, id uuid.UUID, name, nameEn *string, version int) (Playlist, error) {
	var updated Playlist
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		p, err := st.GetKioskPlaylist(ctx, id)
		if err != nil {
			return ProblemError(err)
		}
		updated, err = st.UpdateKioskPlaylist(ctx, name, nameEn, id, version)
		if err != nil {
			return ProblemError(err)
		}

		after := map[string]any{}
		if name != nil {
			after["name"] = *name
		}
		if nameEn != nil {
			after["name_en"] = *nameEn
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "kiosk",
			EntityType: "playlist",
			EntityID:   id,
			Action:     "Update",
			Before:     map[string]any{"name": p.Name, "name_en": p.NameEn},
			After:      after,
		})
	})
	return updated, err
}

// ArchiveKioskPlaylist archives a playlist (NFK-09).
func (s *Service) ArchiveKioskPlaylist(ctx context.Context, id uuid.UUID, version int) (Playlist, error) {
	var updated Playlist
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		p, err := st.GetKioskPlaylist(ctx, id)
		if err != nil {
			return ProblemError(err)
		}
		updated, err = st.ArchiveKioskPlaylist(ctx, id, version)
		if err != nil {
			return ProblemError(err)
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "kiosk",
			EntityType: "playlist",
			EntityID:   id,
			Action:     "Archive",
			Before:     map[string]any{"name": p.Name},
		})
	})
	return updated, err
}

// ─── Playlist Views ───────────────────────────────────────────────────────────

// ListPlaylistViews returns all views in a playlist (KIO-01).
func (s *Service) ListPlaylistViews(ctx context.Context, playlistID uuid.UUID) ([]PlaylistView, error) {
	return s.store.ListPlaylistViews(ctx, playlistID)
}

// CreatePlaylistView adds a view to a playlist (KIO-01, KIO-04).
func (s *Service) CreatePlaylistView(ctx context.Context, playlistID uuid.UUID, viewType string, sortOrder, durationSec int, settings map[string]any) (PlaylistView, error) {
	validTypes := map[string]bool{
		"map": true, "graph": true, "inventory": true, "orders": true, "calendar": true,
	}
	if !validTypes[viewType] {
		return PlaylistView{}, fmt.Errorf("invalid view type %q: must be one of map, graph, inventory, orders, calendar", viewType)
	}

	if settings == nil {
		settings = map[string]any{}
	}
	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return PlaylistView{}, fmt.Errorf("marshal settings: %w", err)
	}

	if sortOrder == 0 {
		views, _ := s.store.ListPlaylistViews(ctx, playlistID)
		sortOrder = len(views)
	}
	if durationSec == 0 {
		durationSec = 30
	}

	var v PlaylistView
	err = db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		id := uuid.New()
		var err error
		v, err = st.CreatePlaylistView(ctx, id, playlistID, viewType, int32(sortOrder), int32(durationSec), settingsJSON) //nolint:gosec // sortOrder and durationSec are bounded by UI
		if err != nil {
			return err
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "kiosk",
			EntityType: "playlist_view",
			EntityID:   v.ID,
			Action:     "Create",
			After:      map[string]any{"view_type": viewType, "playlist_id": playlistID},
		})
	})
	return v, err
}

// UpdatePlaylistView updates a view (KIO-01).
func (s *Service) UpdatePlaylistView(ctx context.Context, id uuid.UUID, viewType *string, sortOrder, durationSec *int, settings *map[string]any, version int) (PlaylistView, error) {
	var vType *string
	var sOrder, dSec *int32
	var settingsJSON *json.RawMessage

	if viewType != nil {
		vType = viewType
	}
	if sortOrder != nil {
		v := int32(*sortOrder) //nolint:gosec // sortOrder is bounded by UI
		sOrder = &v
	}
	if durationSec != nil {
		v := int32(*durationSec) //nolint:gosec // durationSec is bounded by UI
		dSec = &v
	}
	if settings != nil {
		data, err := json.Marshal(*settings)
		if err != nil {
			return PlaylistView{}, fmt.Errorf("marshal settings: %w", err)
		}
		raw := json.RawMessage(data)
		settingsJSON = &raw
	}

	var updated PlaylistView
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		v, err := st.GetPlaylistView(ctx, id)
		if err != nil {
			return ProblemError(err)
		}
		updated, err = st.UpdatePlaylistView(ctx, vType, sOrder, dSec, settingsJSON, id, version)
		if err != nil {
			return ProblemError(err)
		}

		after := map[string]any{}
		if viewType != nil {
			after["view_type"] = *viewType
		}
		if sortOrder != nil {
			after["sort_order"] = *sortOrder
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "kiosk",
			EntityType: "playlist_view",
			EntityID:   id,
			Action:     "Update",
			Before:     map[string]any{"view_type": v.ViewType, "sort_order": v.SortOrder},
			After:      after,
		})
	})
	return updated, err
}

// ArchivePlaylistView removes a view from a playlist (KIO-01).
func (s *Service) ArchivePlaylistView(ctx context.Context, id uuid.UUID, version int) (PlaylistView, error) {
	var updated PlaylistView
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		v, err := st.GetPlaylistView(ctx, id)
		if err != nil {
			return ProblemError(err)
		}
		updated, err = st.ArchivePlaylistView(ctx, id, version)
		if err != nil {
			return ProblemError(err)
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "kiosk",
			EntityType: "playlist_view",
			EntityID:   id,
			Action:     "Archive",
			Before:     map[string]any{"view_type": v.ViewType},
		})
	})
	return updated, err
}

// ─── Devices ──────────────────────────────────────────────────────────────────

// ListKioskDevices returns all kiosk devices (KIO-06).
func (s *Service) ListKioskDevices(ctx context.Context) ([]KioskDevice, error) {
	return s.store.ListKioskDevices(ctx)
}

// GetKioskDevice returns a device by ID (KIO-06).
func (s *Service) GetKioskDevice(ctx context.Context, id uuid.UUID) (KioskDevice, error) {
	d, err := s.store.GetKioskDevice(ctx, id)
	if err != nil {
		return d, ProblemError(err)
	}
	return d, nil
}

// CreateKioskDevice registers a new kiosk device (KIO-06).
func (s *Service) CreateKioskDevice(ctx context.Context, name string) (KioskDevice, error) {
	id := uuid.New()
	pairingCode := generatePairingCode()
	apiKeyHash := hashAPIKey(generateAPIKey())

	var d KioskDevice
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		var err error
		d, err = st.CreateKioskDevice(ctx, id, name, pairingCode, apiKeyHash, false, nil, nil, true)
		if err != nil {
			return err
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "kiosk",
			EntityType: "device",
			EntityID:   d.ID,
			Action:     "Create",
			After:      map[string]any{"name": name, "pairing_code": pairingCode},
		})
	})
	return d, err
}

// ApproveKioskDevice approves a device (KIO-06).
func (s *Service) ApproveKioskDevice(ctx context.Context, id uuid.UUID, playlistID *string, version int) (KioskDevice, error) {
	var pid *uuid.UUID
	if playlistID != nil {
		parsed, err := uuid.Parse(*playlistID)
		if err == nil {
			pid = &parsed
		}
	}

	var updated KioskDevice
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		d, err := st.GetKioskDevice(ctx, id)
		if err != nil {
			return ProblemError(err)
		}
		updated, err = st.UpdateKioskDevice(ctx, nil, nil, pid, nil, nil, id, version)
		if err != nil {
			return ProblemError(err)
		}

		// Re-generate API key on approval
		newKey := generateAPIKey()
		newHash := hashAPIKey(newKey)
		_, err = st.UpdateKioskDevice(ctx, nil, nil, pid, nil, &newHash, id, version+1)
		if err != nil {
			return err
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "kiosk",
			EntityType: "device",
			EntityID:   id,
			Action:     "Approve",
			Before:     map[string]any{"approved": d.Approved},
			After:      map[string]any{"approved": true, "playlist_id": playlistID},
		})
	})
	return updated, err
}

// ArchiveKioskDevice revokes and archives a device (KIO-06).
func (s *Service) ArchiveKioskDevice(ctx context.Context, id uuid.UUID, version int) (KioskDevice, error) {
	var updated KioskDevice
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		st := s.store.WithTx(tx)
		d, err := st.GetKioskDevice(ctx, id)
		if err != nil {
			return ProblemError(err)
		}
		updated, err = st.ArchiveKioskDevice(ctx, id, version)
		if err != nil {
			return ProblemError(err)
		}

		return s.rec.Record(ctx, tx, audit.Entry{
			Module:     "kiosk",
			EntityType: "device",
			EntityID:   id,
			Action:     "Revoke",
			Before:     map[string]any{"approved": d.Approved},
		})
	})
	return updated, err
}

// ValidateKioskAPIKey validates a kiosk API key and returns the device (KIO-06).
func (s *Service) ValidateKioskAPIKey(ctx context.Context, apiKey, pairingCode string) (*KioskDevice, error) {
	// Try by API key first
	if apiKey != "" {
		d, err := s.store.GetKioskDeviceByAPIKeyHash(ctx, hashAPIKey(apiKey))
		if err == nil {
			return &d, nil
		}
	}
	// Try by pairing code (for initial pairing)
	if pairingCode != "" {
		d, err := s.store.GetKioskDeviceByPairingCode(ctx, pairingCode)
		if err == nil {
			return &d, nil
		}
	}
	return nil, fmt.Errorf("invalid pairing code or API key")
}

// GetKioskData returns aggregated data for kiosk views (KIO-02, KIO-03, KIO-05).
// ASSUMPTION: The actual data fetching from orders, inventory, customers etc.
// will be implemented by querying those modules' tables. For now we return empty.
func (s *Service) GetKioskData(ctx context.Context, _ int) (KioskData, error) {
	return KioskData{}, nil // ASSUMPTION: real data will be fetched from orders/inventory/customers tables
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func generatePairingCode() string {
	// 6-character alphanumeric code
	return uuid.New().String()[:6]
}

func generateAPIKey() string {
	return "lnc_kiosk_" + uuid.New().String()
}

func hashAPIKey(key string) string {
	// Simple hash for storage; in production use bcrypt or argon2
	// ASSUMPTION: We'll use a proper hashing library later
	return fmt.Sprintf("%x", key)
}
