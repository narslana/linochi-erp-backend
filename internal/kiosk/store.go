package kiosk

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db/dbgen"
)

// Store wraps sqlc queries for the kiosk module.
type Store struct {
	q *dbgen.Queries
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{q: dbgen.New(pool)}
}

// WithTx returns a store that runs all queries in the given transaction.
func (s *Store) WithTx(tx *db.Tx) *Store {
	return &Store{q: tx.Queries()}
}

// ─── Playlists ────────────────────────────────────────────────────────────────

func (s *Store) ListKioskPlaylists(ctx context.Context) ([]Playlist, error) {
	rows, err := s.q.ListKioskPlaylists(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Playlist, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToPlaylist(r))
	}
	return out, nil
}

func (s *Store) GetKioskPlaylist(ctx context.Context, id uuid.UUID) (Playlist, error) {
	row, err := s.q.GetKioskPlaylist(ctx, id)
	if err != nil {
		return Playlist{}, err
	}
	return dbRowToPlaylist(row), nil
}

func (s *Store) CreateKioskPlaylist(ctx context.Context, id uuid.UUID, name, nameEn string) (Playlist, error) {
	row, err := s.q.CreateKioskPlaylist(ctx, dbgen.CreateKioskPlaylistParams{
		ID:     id,
		Name:   name,
		NameEn: nameEn,
	})
	if err != nil {
		return Playlist{}, err
	}
	return dbRowToPlaylist(row), nil
}

func (s *Store) UpdateKioskPlaylist(ctx context.Context, name, nameEn *string, id uuid.UUID, version int) (Playlist, error) {
	var params dbgen.UpdateKioskPlaylistParams
	params.ID = id
	params.Version = int32(version) //nolint:gosec
	if name != nil {
		params.Name = *name
	}
	if nameEn != nil {
		params.NameEn = *nameEn
	}
	row, err := s.q.UpdateKioskPlaylist(ctx, params)
	if err != nil {
		return Playlist{}, err
	}
	return dbRowToPlaylist(row), nil
}

func (s *Store) ArchiveKioskPlaylist(ctx context.Context, id uuid.UUID, version int) (Playlist, error) {
	row, err := s.q.ArchiveKioskPlaylist(ctx, dbgen.ArchiveKioskPlaylistParams{ID: id, Version: int32(version)}) //nolint:gosec
	if err != nil {
		return Playlist{}, err
	}
	return dbRowToPlaylist(row), nil
}

// ─── Playlist Views ───────────────────────────────────────────────────────────

func (s *Store) ListPlaylistViews(ctx context.Context, playlistID uuid.UUID) ([]PlaylistView, error) {
	rows, err := s.q.ListPlaylistViews(ctx, playlistID)
	if err != nil {
		return nil, err
	}
	out := make([]PlaylistView, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToPlaylistView(r))
	}
	return out, nil
}

func (s *Store) GetPlaylistView(ctx context.Context, id uuid.UUID) (PlaylistView, error) {
	row, err := s.q.GetPlaylistView(ctx, id)
	if err != nil {
		return PlaylistView{}, err
	}
	return dbRowToPlaylistView(row), nil
}

func (s *Store) CreatePlaylistView(ctx context.Context, id, playlistID uuid.UUID, viewType string, sortOrder, durationSec int32, settings json.RawMessage) (PlaylistView, error) {
	row, err := s.q.CreatePlaylistView(ctx, dbgen.CreatePlaylistViewParams{
		ID:          id,
		PlaylistID:  playlistID,
		ViewType:    viewType,
		SortOrder:   sortOrder,
		DurationSec: durationSec,
		Settings:    settings,
	})
	if err != nil {
		return PlaylistView{}, err
	}
	return dbRowToPlaylistView(row), nil
}

func (s *Store) UpdatePlaylistView(ctx context.Context, viewType *string, sortOrder, durationSec *int32, settings *json.RawMessage, id uuid.UUID, version int) (PlaylistView, error) {
	var params dbgen.UpdatePlaylistViewParams
	params.ID = id
	params.Version = int32(version) //nolint:gosec
	if viewType != nil {
		params.ViewType = *viewType
	}
	if sortOrder != nil {
		params.SortOrder = *sortOrder
	}
	if durationSec != nil {
		params.DurationSec = *durationSec
	}
	if settings != nil {
		params.Settings = *settings
	}
	row, err := s.q.UpdatePlaylistView(ctx, params)
	if err != nil {
		return PlaylistView{}, err
	}
	return dbRowToPlaylistView(row), nil
}

func (s *Store) ArchivePlaylistView(ctx context.Context, id uuid.UUID, version int) (PlaylistView, error) {
	row, err := s.q.ArchivePlaylistView(ctx, dbgen.ArchivePlaylistViewParams{ID: id, Version: int32(version)}) //nolint:gosec
	if err != nil {
		return PlaylistView{}, err
	}
	return dbRowToPlaylistView(row), nil
}

func (s *Store) UpdatePlaylistViewSortOrder(ctx context.Context, sortOrder int32, id uuid.UUID, version int) (PlaylistView, error) {
	row, err := s.q.UpdatePlaylistViewSortOrder(ctx, dbgen.UpdatePlaylistViewSortOrderParams{
		ID:        id,
		SortOrder: sortOrder,
		Version:   int32(version), //nolint:gosec
	})
	if err != nil {
		return PlaylistView{}, err
	}
	return dbRowToPlaylistView(row), nil
}

// ─── Devices ──────────────────────────────────────────────────────────────────

func (s *Store) ListKioskDevices(ctx context.Context) ([]KioskDevice, error) {
	rows, err := s.q.ListKioskDevices(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]KioskDevice, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToKioskDevice(r))
	}
	return out, nil
}

func (s *Store) GetKioskDevice(ctx context.Context, id uuid.UUID) (KioskDevice, error) {
	row, err := s.q.GetKioskDevice(ctx, id)
	if err != nil {
		return KioskDevice{}, err
	}
	return dbRowToKioskDevice(row), nil
}

func (s *Store) GetKioskDeviceByPairingCode(ctx context.Context, pairingCode string) (KioskDevice, error) {
	row, err := s.q.GetKioskDeviceByPairingCode(ctx, pairingCode)
	if err != nil {
		return KioskDevice{}, err
	}
	return dbRowToKioskDevice(row), nil
}

func (s *Store) GetKioskDeviceByAPIKeyHash(ctx context.Context, apiKeyHash string) (KioskDevice, error) {
	row, err := s.q.GetKioskDeviceByAPIKeyHash(ctx, apiKeyHash)
	if err != nil {
		return KioskDevice{}, err
	}
	return dbRowToKioskDevice(row), nil
}

func (s *Store) CreateKioskDevice(ctx context.Context, id uuid.UUID, name, pairingCode, apiKeyHash string, approved bool, playlistID *uuid.UUID, expiresAt *time.Time, lanOnly bool) (KioskDevice, error) {
	row, err := s.q.CreateKioskDevice(ctx, dbgen.CreateKioskDeviceParams{
		ID:          id,
		Name:        name,
		PairingCode: pairingCode,
		ApiKeyHash:  apiKeyHash,
		Approved:    approved,
		PlaylistID:  playlistID,
		ExpiresAt:   expiresAt,
		LanOnly:     lanOnly,
	})
	if err != nil {
		return KioskDevice{}, err
	}
	return dbRowToKioskDevice(row), nil
}

func (s *Store) UpdateKioskDevice(ctx context.Context, name *string, approved *bool, playlistID *uuid.UUID, expiresAt *time.Time, apiKeyHash *string, id uuid.UUID, version int) (KioskDevice, error) {
	var params dbgen.UpdateKioskDeviceParams
	params.ID = id
	params.Version = int32(version) //nolint:gosec
	// COALESCE: pass empty/zero values; only non-nil pointers update the DB
	if name != nil {
		params.Name = *name
	} else {
		params.Name = ""
	}
	if approved != nil {
		params.Approved = *approved
	} else {
		params.Approved = false
	}
	params.PlaylistID = playlistID
	params.ExpiresAt = expiresAt
	if apiKeyHash != nil {
		params.ApiKeyHash = *apiKeyHash
	} else {
		params.ApiKeyHash = ""
	}
	row, err := s.q.UpdateKioskDevice(ctx, params)
	if err != nil {
		return KioskDevice{}, err
	}
	return dbRowToKioskDevice(row), nil
}

func (s *Store) ArchiveKioskDevice(ctx context.Context, id uuid.UUID, version int) (KioskDevice, error) {
	row, err := s.q.ArchiveKioskDevice(ctx, dbgen.ArchiveKioskDeviceParams{ID: id, Version: int32(version)}) //nolint:gosec
	if err != nil {
		return KioskDevice{}, err
	}
	return dbRowToKioskDevice(row), nil
}

// ─── Row helpers ──────────────────────────────────────────────────────────────

func dbRowToPlaylist(r dbgen.KioskPlaylist) Playlist {
	p := Playlist{
		ID:        r.ID,
		Name:      r.Name,
		NameEn:    r.NameEn,
		Version:   int(r.Version),
		CreatedAt: r.CreatedAt.Format(time.RFC3339),
		UpdatedAt: r.UpdatedAt.Format(time.RFC3339),
	}
	if r.ArchivedAt != nil {
		p.Archived = true
		t := r.ArchivedAt.Format(time.RFC3339)
		p.ArchivedAt = &t
	}
	return p
}

func dbRowToPlaylistView(r dbgen.KioskPlaylistView) PlaylistView {
	settings := r.Settings
	if settings == nil {
		settings = json.RawMessage("{}")
	}
	v := PlaylistView{
		ID:          r.ID,
		PlaylistID:  r.PlaylistID,
		ViewType:    r.ViewType,
		SortOrder:   r.SortOrder,
		DurationSec: r.DurationSec,
		Settings:    settings,
		Version:     int(r.Version),
		CreatedAt:   r.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   r.UpdatedAt.Format(time.RFC3339),
	}
	if r.ArchivedAt != nil {
		v.Archived = true
		t := r.ArchivedAt.Format(time.RFC3339)
		v.ArchivedAt = &t
	}
	return v
}

func dbRowToKioskDevice(r dbgen.KioskDevice) KioskDevice {
	d := KioskDevice{
		ID:          r.ID,
		Name:        r.Name,
		PairingCode: r.PairingCode,
		Approved:    r.Approved,
		LanOnly:     r.LanOnly,
		Version:     int(r.Version),
		CreatedAt:   r.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   r.UpdatedAt.Format(time.RFC3339),
	}
	if r.PlaylistID != nil {
		s := r.PlaylistID.String()
		d.PlaylistID = &s
	}
	if r.ExpiresAt != nil {
		s := r.ExpiresAt.Format(time.RFC3339)
		d.ExpiresAt = &s
	}
	if r.ArchivedAt != nil {
		d.Archived = true
		t := r.ArchivedAt.Format(time.RFC3339)
		d.ArchivedAt = &t
	}
	return d
}
