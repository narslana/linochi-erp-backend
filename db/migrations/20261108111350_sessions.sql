-- +goose Up

-- Browser sessions (NFK-02, NFK-11). They live in Postgres so that every replica
-- sees the same session and a rolling update logs nobody out.
--
-- Only a hash of the token is stored, never the token (NFK-11): a copy of this
-- table, a backup or a read-only SQL injection cannot be turned into a login.
-- The token is 32 random bytes, so SHA-256 is enough.
--
-- Roles are not stored in users. They are computed from the Authelia groups at
-- every login and kept here, for the life of the session.
CREATE TABLE sessions (
    id           uuid        PRIMARY KEY DEFAULT uuidv7(),
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    version      integer     NOT NULL DEFAULT 1,
    -- No ON DELETE: users are never deleted (NFK-09), only archived.
    user_id      uuid        NOT NULL REFERENCES users (id),
    token_hash   bytea       NOT NULL,
    roles        text[]      NOT NULL,
    -- Moves at most once a minute while the session is used (the idle limit).
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    -- The absolute limit. Set once, never moved.
    expires_at   timestamptz NOT NULL,
    CONSTRAINT sessions_token_hash_key UNIQUE (token_hash),
    CONSTRAINT sessions_token_hash_length CHECK (octet_length(token_hash) = 32),
    CONSTRAINT sessions_expires_after_creation CHECK (expires_at > created_at)
);

-- "End every session of this user" (access removed, device lost) and the
-- cleanup both filter on these. last_seen_at has no index on purpose: it is
-- written often and the table is small, so the cleanup scans it.
CREATE INDEX sessions_user_idx ON sessions (user_id);

CREATE TRIGGER sessions_set_updated_at
    BEFORE UPDATE ON sessions
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- DELETE is granted here, and nowhere else but on this table: a session is not
-- a record of anything. Data is kept forever (NFK-09), but a session is a
-- credential, and a credential that is over has to be gone. What happened to it
-- is in audit_log (login, logout, all of a user's sessions ended).
--
-- No TRUNCATE, and no UPDATE on anything but last_seen_at: expires_at is the
-- absolute limit and must not be movable by the app, or the 7 days would mean
-- nothing.
GRANT SELECT, INSERT, DELETE ON sessions TO linochi_erp_app;
GRANT UPDATE (last_seen_at, updated_at, version) ON sessions TO linochi_erp_app;

-- +goose Down
REVOKE UPDATE (last_seen_at, updated_at, version) ON sessions FROM linochi_erp_app;
REVOKE SELECT, INSERT, DELETE ON sessions FROM linochi_erp_app;
DROP TABLE sessions;
