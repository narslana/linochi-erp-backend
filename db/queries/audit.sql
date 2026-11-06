-- name: InsertAuditEntry :one
INSERT INTO audit_log (
    id, actor_type, actor_id, actor_label, module, entity_type, entity_id,
    action, before, after, reason, trace_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7,
    $8, $9, $10, $11, $12
)
RETURNING *;

-- name: ListAuditEntriesForEntity :many
-- The history of one record, oldest first. The filters of INS-05 (user, module,
-- date) get queries of their own when the view is built.
SELECT * FROM audit_log
WHERE entity_type = $1 AND entity_id = $2
ORDER BY created_at, id;

-- name: RedactAuditPersonalData :one
-- GDPR (KUN-07). Calls the SECURITY DEFINER function, the only way to change old
-- audit rows. Returns how many rows were redacted.
SELECT audit_redact_personal_data(sqlc.arg(entity_type)::text, sqlc.arg(entity_id)::uuid, sqlc.arg(fields)::text[])::integer AS redacted;
