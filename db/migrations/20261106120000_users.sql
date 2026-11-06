-- +goose Up

-- Shared by every table that has updated_at. Optimistic locking uses the
-- version column and is done in the queries, not here: a trigger cannot know
-- which version the caller read.
-- +goose StatementBegin
CREATE FUNCTION set_updated_at() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

-- Users come from OIDC (Authelia). Roles are not stored: they are computed from
-- the groups at every login and kept in the session.
CREATE TABLE users (
    id            uuid        PRIMARY KEY DEFAULT uuidv7(),
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    version       integer     NOT NULL DEFAULT 1,
    oidc_issuer   text        NOT NULL,
    oidc_subject  text        NOT NULL,
    email         text        NOT NULL,
    display_name  text        NOT NULL DEFAULT '',
    last_login_at timestamptz,
    archived_at   timestamptz,
    CONSTRAINT users_oidc_identity_key UNIQUE (oidc_issuer, oidc_subject)
);

CREATE TRIGGER users_set_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE users;
DROP FUNCTION set_updated_at();
