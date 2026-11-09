-- +goose Up

-- One row per login in progress (NFK-10): made when the browser is sent to
-- Authelia and consumed when it comes back. It lives in Postgres and not in a
-- cookie or in memory because the callback may land on another replica than the
-- one that started the login, and so that a state can be used exactly once.
--
-- state_hash is the SHA-256 of the "state" that goes to Authelia and back in the
-- URL. binding_hash is the SHA-256 of a random value that stays in a cookie of
-- the browser that started the login: a callback from any other browser is
-- refused, so a login cannot be started in one browser and finished in another
-- (login CSRF). The nonce and the PKCE verifier are needed in clear to finish the
-- login. They are worth nothing without the authorization code and the browser's
-- cookie, and a row lives ten minutes at most.
--
-- No updated_at trigger: a row is written once and deleted, never updated, and
-- the app role has no UPDATE privilege.
CREATE TABLE oidc_login_attempts (
    id            uuid        PRIMARY KEY DEFAULT uuidv7(),
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    version       integer     NOT NULL DEFAULT 1,
    state_hash    bytea       NOT NULL,
    binding_hash  bytea       NOT NULL,
    nonce         text        NOT NULL,
    code_verifier text        NOT NULL,
    -- Where to send the browser after the login. A path in this app, checked
    -- before it is stored and again before it is used.
    return_to     text        NOT NULL DEFAULT '/',
    expires_at    timestamptz NOT NULL,
    CONSTRAINT oidc_login_attempts_state_hash_key UNIQUE (state_hash),
    CONSTRAINT oidc_login_attempts_state_hash_length CHECK (octet_length(state_hash) = 32),
    CONSTRAINT oidc_login_attempts_binding_hash_length CHECK (octet_length(binding_hash) = 32),
    CONSTRAINT oidc_login_attempts_expires_after_creation CHECK (expires_at > created_at)
);

-- The cap on open attempts and the cleanup both filter on this.
CREATE INDEX oidc_login_attempts_expires_idx ON oidc_login_attempts (expires_at);

-- DELETE is granted here, as on sessions and nowhere else: a login attempt is a
-- credential in the making and not a record of anything. Consuming one is a
-- DELETE ... RETURNING, which is what makes a state usable exactly once even
-- when two replicas get the callback at the same moment. DELETE ... WHERE and
-- RETURNING also need SELECT. There is no UPDATE and no TRUNCATE.
GRANT SELECT, INSERT, DELETE ON oidc_login_attempts TO linochi_erp_app;

-- +goose Down
REVOKE SELECT, INSERT, DELETE ON oidc_login_attempts FROM linochi_erp_app;
DROP TABLE oidc_login_attempts;
