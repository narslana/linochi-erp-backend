-- name: ListICalSources :many
SELECT * FROM ical_sources WHERE archived = false ORDER BY name;

-- name: GetICalSource :one
SELECT * FROM ical_sources WHERE id = $1 AND archived = false;

-- name: CreateICalSource :one
INSERT INTO ical_sources (
    id, name, name_en, ical_url, color, sync_interval
) VALUES (
    $1, $2, $3, $4, $5, $6
) RETURNING *;

-- name: UpdateICalSource :one
UPDATE ical_sources SET
    name = COALESCE(sqlc.arg(name), name),
    name_en = COALESCE(sqlc.arg(name_en), name_en),
    ical_url = COALESCE(sqlc.arg(ical_url), ical_url),
    color = COALESCE(sqlc.arg(color), color),
    sync_interval = COALESCE(sqlc.arg(sync_interval), sync_interval),
    enabled = COALESCE(sqlc.arg(enabled), enabled),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2 AND archived = false
RETURNING *;

-- name: ArchiveICalSource :one
UPDATE ical_sources SET
    archived = true,
    archived_at = now(),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2
RETURNING *;

-- name: ListCalendarEvents :many
SELECT * FROM calendar_events
WHERE archived = false
  AND start_time < @end_time
  AND end_time > @start_time
ORDER BY start_time;

-- name: GetCalendarEvent :one
SELECT * FROM calendar_events WHERE id = $1 AND archived = false;

-- name: UpsertCalendarEvent :one
INSERT INTO calendar_events (
    id, source_type, source_id, ical_uid, title, description,
    start_time, end_time, all_day, color,
    erp_link_type, erp_link_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)
ON CONFLICT (ical_uid) DO UPDATE SET
    title = EXCLUDED.title,
    description = EXCLUDED.description,
    start_time = EXCLUDED.start_time,
    end_time = EXCLUDED.end_time,
    all_day = EXCLUDED.all_day,
    color = EXCLUDED.color,
    version = calendar_events.version + 1,
    updated_at = now()
RETURNING *;

-- name: DeleteCalendarEventByICalUID :exec
DELETE FROM calendar_events
WHERE ical_uid = $1 AND source_type = 'ical';

-- name: ListKioskPlaylists :many
SELECT * FROM kiosk_playlists WHERE archived = false ORDER BY name;

-- name: GetKioskPlaylist :one
SELECT * FROM kiosk_playlists WHERE id = $1 AND archived = false;

-- name: CreateKioskPlaylist :one
INSERT INTO kiosk_playlists (id, name, name_en) VALUES ($1, $2, $3) RETURNING *;

-- name: UpdateKioskPlaylist :one
UPDATE kiosk_playlists SET
    name = COALESCE(sqlc.arg(name), name),
    name_en = COALESCE(sqlc.arg(name_en), name_en),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2 AND archived = false
RETURNING *;

-- name: ArchiveKioskPlaylist :one
UPDATE kiosk_playlists SET
    archived = true,
    archived_at = now(),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2
RETURNING *;

-- name: ListPlaylistViews :many
SELECT * FROM kiosk_playlist_views
WHERE playlist_id = $1 AND archived = false
ORDER BY sort_order;

-- name: CreatePlaylistView :one
INSERT INTO kiosk_playlist_views (
    id, playlist_id, view_type, sort_order, duration_sec, settings
) VALUES ($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: UpdatePlaylistView :one
UPDATE kiosk_playlist_views SET
    view_type = COALESCE(sqlc.arg(view_type), view_type),
    sort_order = COALESCE(sqlc.arg(sort_order), sort_order),
    duration_sec = COALESCE(sqlc.arg(duration_sec), duration_sec),
    settings = COALESCE(sqlc.arg(settings), settings),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2 AND archived = false
RETURNING *;

-- name: ArchivePlaylistView :one
UPDATE kiosk_playlist_views SET
    archived = true,
    archived_at = now(),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2
RETURNING *;

-- name: UpdatePlaylistViewSortOrder :one
UPDATE kiosk_playlist_views SET
    sort_order = sqlc.arg(sort_order),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2 AND archived = false
RETURNING *;

-- name: ListKioskDevices :many
SELECT * FROM kiosk_devices WHERE archived = false ORDER BY name;

-- name: GetKioskDevice :one
SELECT * FROM kiosk_devices WHERE id = $1 AND archived = false;

-- name: GetKioskDeviceByPairingCode :one
SELECT * FROM kiosk_devices WHERE pairing_code = $1 AND archived = false;

-- name: GetKioskDeviceByAPIKeyHash :one
SELECT * FROM kiosk_devices WHERE api_key_hash = $1 AND approved = true AND archived = false;

-- name: CreateKioskDevice :one
INSERT INTO kiosk_devices (
    id, name, pairing_code, api_key_hash, approved, playlist_id, expires_at, lan_only
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *;

-- name: UpdateKioskDevice :one
UPDATE kiosk_devices SET
    name = COALESCE(sqlc.arg(name), name),
    approved = COALESCE(sqlc.arg(approved), approved),
    playlist_id = COALESCE(sqlc.arg(playlist_id), playlist_id),
    expires_at = COALESCE(sqlc.arg(expires_at), expires_at),
    api_key_hash = COALESCE(sqlc.arg(api_key_hash), api_key_hash),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2 AND archived = false
RETURNING *;

-- name: ArchiveKioskDevice :one
UPDATE kiosk_devices SET
    archived = true,
    archived_at = now(),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2
RETURNING *;
