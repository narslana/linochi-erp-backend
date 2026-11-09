-- name: InsertSession :one
-- created_at comes from the app, like last_seen_at and expires_at, so that all
-- three come from one clock reading (and one injected clock in tests).
INSERT INTO sessions (id, user_id, token_hash, roles, created_at, last_seen_at, expires_at)
VALUES ($1, $2, $3, $4, $5, $5, $6)
RETURNING *;

-- name: GetSessionByTokenHash :one
-- The login check on every request. The user comes along so that an archived
-- user is refused in the same query, with no second round trip.
SELECT s.*, u.archived_at AS user_archived_at
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1;

-- name: TouchSession :execrows
-- Moves last_seen_at forward and nothing else. Guarded on the value that was
-- read, so two requests on two pods writing at once do not move it backwards,
-- and the session limit (expires_at) is never part of this statement.
UPDATE sessions
SET last_seen_at = $2,
    version      = version + 1
WHERE id = $1 AND last_seen_at < $2;

-- name: DeleteSessionByTokenHash :one
-- Logout. The row is returned so that the audit entry can say whose session it
-- was; no row back means the session was already gone, which is fine.
DELETE FROM sessions WHERE token_hash = $1 RETURNING *;

-- name: DeleteSessionsForUser :many
-- Ends every session of one user at once (access removed in Authelia, a lost
-- device, an archived user). Returns them so that the caller can count and audit.
DELETE FROM sessions WHERE user_id = $1 RETURNING *;

-- name: DeleteEndedSessions :execrows
-- The cleanup. Idempotent, so any number of pods may run it at the same time.
-- The two conditions are the two limits of NFK-11, with the same boundaries as
-- Session.Status: a session whose limit is exactly now has ended. idle_cutoff is
-- now minus IdleTimeout, worked out by the caller, so the limit lives in one
-- place (session.go) and not also in SQL.
DELETE FROM sessions
WHERE expires_at   <= sqlc.arg(now)::timestamptz
   OR last_seen_at <= sqlc.arg(idle_cutoff)::timestamptz;

-- name: LockActiveUserForSession :one
-- Taken before a session is created, so that archiving the user and logging in
-- at the same time cannot leave a session behind for an archived user: either
-- the archive commits first and this returns an archived user, or this lock is
-- held and the archive waits and then ends the session it finds.
SELECT * FROM users WHERE id = $1 FOR SHARE;
