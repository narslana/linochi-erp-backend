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

-- name: ListActiveUsersByEmail :many
-- For the operator command that ends a user's sessions. An e-mail address is
-- not unique (a user is an issuer and a subject), so this can return more than
-- one row, and the caller must not guess. Archived users are left out: they
-- have no sessions and must not make an address ambiguous. Compared without
-- regard to case, because nobody types an address the way it is stored. There
-- is no index on lower(email): the table is two people, not thousands.
SELECT * FROM users
WHERE lower(email) = lower($1) AND archived_at IS NULL
ORDER BY created_at, id;
