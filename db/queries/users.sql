-- name: InsertUserIfNew :one
-- The first login of a user. No rows back means the user already exists, which
-- makes concurrent first logins safe: exactly one of them gets the row, and that
-- one is audited as the create. The id is made by the app (UUIDv7).
INSERT INTO users (id, oidc_issuer, oidc_subject, email, display_name, last_login_at)
VALUES ($1, $2, $3, $4, $5, now())
ON CONFLICT (oidc_issuer, oidc_subject) DO NOTHING
RETURNING *;

-- name: LockUserByOIDC :one
-- The row as it is before a login changes it, locked so that the audit entry's
-- before values are the ones that were really overwritten.
SELECT * FROM users WHERE oidc_issuer = $1 AND oidc_subject = $2 FOR UPDATE;

-- name: UpdateUserOnLogin :one
-- Later logins refresh the profile and the time of the last login.
UPDATE users
SET email         = $2,
    display_name  = $3,
    last_login_at = now(),
    version       = version + 1
WHERE id = $1
RETURNING *;

-- name: GetUser :one
SELECT * FROM users WHERE id = $1;

-- name: LockUser :one
SELECT * FROM users WHERE id = $1 FOR UPDATE;

-- name: ArchiveUser :one
-- Optimistic locking: only the caller who read the current version wins.
-- Archived users are hidden, never deleted (NFK-09).
UPDATE users
SET archived_at = coalesce(archived_at, now()),
    version     = version + 1
WHERE id = $1 AND version = $2
RETURNING *;
