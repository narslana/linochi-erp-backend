-- name: ListReminders :many
SELECT * FROM reminders WHERE archived = false ORDER BY due_date ASC;

-- name: GetReminder :one
SELECT * FROM reminders WHERE id = $1 AND archived = false;

-- name: CreateReminder :one
INSERT INTO reminders (
    id, reminder_type, ids_entry_id, external_ref_type, external_ref_id,
    title, title_en, description, due_date
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
) RETURNING *;

-- name: UpdateReminder :one
UPDATE reminders SET
    title = COALESCE(sqlc.arg(title), title),
    title_en = COALESCE(sqlc.arg(title_en), title_en),
    description = COALESCE(sqlc.arg(description), description),
    due_date = COALESCE(sqlc.arg(due_date), due_date),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2 AND archived = false
RETURNING *;

-- name: ArchiveReminder :one
UPDATE reminders SET
    archived = true,
    archived_at = now(),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2
RETURNING *;

-- name: MarkReminderNotified :one
UPDATE reminders SET
    notified = true,
    notified_at = now(),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2
RETURNING *;

-- name: ExpiredReminders :many
-- Find reminders that have passed their due date without being notified
SELECT * FROM reminders
WHERE archived = false
  AND due_date < current_date
  AND notified = false;
