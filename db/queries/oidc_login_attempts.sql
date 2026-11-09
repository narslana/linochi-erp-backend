-- name: InsertLoginAttempt :one
-- Starts a login. The WHERE is the cap on open attempts: /auth/login needs no
-- session, so without it anyone could fill the table. No row back means the cap
-- was reached. Two requests at the same moment may overshoot it by a few, which
-- is fine: it is there to stop a flood, not to count exactly.
INSERT INTO oidc_login_attempts (id, state_hash, binding_hash, nonce, code_verifier, return_to, created_at, expires_at)
SELECT $1, $2, $3, $4, $5, $6, sqlc.arg(created_at)::timestamptz, sqlc.arg(expires_at)::timestamptz
WHERE (SELECT count(*) FROM oidc_login_attempts WHERE expires_at > sqlc.arg(created_at)::timestamptz) < sqlc.arg(max_open)::bigint
RETURNING *;

-- name: ConsumeLoginAttempt :one
-- Finishes a login: the row is deleted and returned in one statement, so the
-- state works once, also across replicas. An attempt that has expired is
-- deleted too (the caller sees expires_at and refuses it).
DELETE FROM oidc_login_attempts WHERE state_hash = $1 RETURNING *;

-- name: DeleteExpiredLoginAttempts :execrows
-- The cleanup. Idempotent, so any number of pods may run it at the same time.
DELETE FROM oidc_login_attempts WHERE expires_at <= $1;
