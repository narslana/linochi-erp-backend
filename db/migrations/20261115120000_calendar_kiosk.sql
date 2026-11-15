-- +goose Up
-- +goose StatementBegin
-- KAL-01: iCal sources for Google calendars
CREATE TABLE ical_sources (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name            text NOT NULL,
    name_en         text NOT NULL DEFAULT '',
    ical_url        text NOT NULL,
    color           text NOT NULL DEFAULT '#3b82f6',
    sync_interval   int NOT NULL DEFAULT 15,
    enabled         boolean NOT NULL DEFAULT true,
    archived        boolean NOT NULL DEFAULT false,
    archived_at     timestamptz,
    version         int NOT NULL DEFAULT 1,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

-- KAL-01, KAL-03: calendar events from iCal and ERP internal sources
CREATE TABLE calendar_events (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source_type     text NOT NULL CHECK (source_type IN ('ical', 'internal')),
    source_id       uuid,
    ical_uid        text,
    title           text NOT NULL,
    description     text NOT NULL DEFAULT '',
    start_time      timestamptz NOT NULL,
    end_time        timestamptz NOT NULL,
    all_day         boolean NOT NULL DEFAULT false,
    color           text NOT NULL DEFAULT '#6366f1',
    erp_link_type   text,
    erp_link_id     uuid,
    archived        boolean NOT NULL DEFAULT false,
    archived_at     timestamptz,
    version         int NOT NULL DEFAULT 1,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

-- KAL-03: index for querying events by time range
CREATE INDEX idx_calendarevents_time ON calendar_events(start_time, end_time);
CREATE INDEX idx_calendarevents_source ON calendar_events(source_type, source_id) WHERE source_id IS NOT NULL;
CREATE INDEX idx_calendarevents_erp ON calendar_events(erp_link_type, erp_link_id) WHERE erp_link_id IS NOT NULL;
CREATE INDEX idx_calendarevents_ical_uid ON calendar_events(ical_uid) WHERE ical_uid IS NOT NULL;

-- KAL-01: grant permissions
GRANT SELECT, INSERT, UPDATE ON ical_sources TO linochi_erp_app;
GRANT SELECT, INSERT, UPDATE ON calendar_events TO linochi_erp_app;
-- +goose StatementEnd

-- +goose Up
-- +goose StatementBegin
-- KIO-01: kiosk playlists
CREATE TABLE kiosk_playlists (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name            text NOT NULL,
    name_en         text NOT NULL DEFAULT '',
    archived        boolean NOT NULL DEFAULT false,
    archived_at     timestamptz,
    version         int NOT NULL DEFAULT 1,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

-- KIO-01, KIO-04: views within a playlist
CREATE TABLE kiosk_playlist_views (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    playlist_id     uuid NOT NULL REFERENCES kiosk_playlists(id),
    view_type       text NOT NULL CHECK (view_type IN ('map', 'graph', 'inventory', 'orders', 'calendar')),
    sort_order      int NOT NULL DEFAULT 0,
    duration_sec    int NOT NULL DEFAULT 30,
    settings        jsonb NOT NULL DEFAULT '{}',
    archived        boolean NOT NULL DEFAULT false,
    archived_at     timestamptz,
    version         int NOT NULL DEFAULT 1,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

-- KIO-06: kiosk devices for fixed screens
CREATE TABLE kiosk_devices (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name            text NOT NULL,
    pairing_code    text NOT NULL DEFAULT '',
    api_key_hash    text NOT NULL DEFAULT '',
    approved        boolean NOT NULL DEFAULT false,
    playlist_id     uuid REFERENCES kiosk_playlists(id),
    expires_at      timestamptz,
    lan_only        boolean NOT NULL DEFAULT true,
    archived        boolean NOT NULL DEFAULT false,
    archived_at     timestamptz,
    version         int NOT NULL DEFAULT 1,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

-- KIO-06: index for pairing code lookups
CREATE INDEX idx_kioskdevices_pairing ON kiosk_devices(pairing_code) WHERE pairing_code != '';
CREATE INDEX idx_kioskdevices_key ON kiosk_devices(api_key_hash) WHERE api_key_hash != '';

-- KIO: grant permissions
GRANT SELECT, INSERT, UPDATE ON kiosk_playlists TO linochi_erp_app;
GRANT SELECT, INSERT, UPDATE ON kiosk_playlist_views TO linochi_erp_app;
GRANT SELECT, INSERT, UPDATE ON kiosk_devices TO linochi_erp_app;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS kiosk_devices;
DROP TABLE IF EXISTS kiosk_playlist_views;
DROP TABLE IF EXISTS kiosk_playlists;
DROP TABLE IF EXISTS calendar_events;
DROP TABLE IF EXISTS ical_sources;
-- +goose StatementEnd
