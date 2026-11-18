-- name: DefaultWorkflowForUpdate :one
-- WF-07: locks the default flow so that two publishes cannot take the same version number.
SELECT * FROM workflows WHERE channel IS NULL FOR UPDATE;

-- name: GetActiveWorkflowVersion :one
-- WF-07: the version new orders start in.
SELECT v.* FROM workflow_versions v JOIN workflows w ON w.id = v.workflow_id
WHERE w.channel IS NULL AND v.is_active;

-- name: GetWorkflowVersion :one
SELECT * FROM workflow_versions WHERE id = $1;

-- name: ListWorkflowVersions :many
SELECT * FROM workflow_versions ORDER BY version DESC;

-- name: NextWorkflowVersionNumber :one
SELECT (COALESCE(MAX(version), 0) + 1)::int AS next FROM workflow_versions WHERE workflow_id = $1;

-- name: DeactivateWorkflowVersions :exec
UPDATE workflow_versions SET is_active = false, updated_at = now() WHERE workflow_id = $1 AND is_active;

-- name: InsertWorkflowVersion :one
INSERT INTO workflow_versions (workflow_id, version, is_active) VALUES ($1, $2, true) RETURNING *;

-- name: InsertWorkflowStep :one
INSERT INTO workflow_steps (version_id, key, sort_order, is_start, is_final, is_locked, stock_phase)
VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING *;

-- name: InsertWorkflowTransition :exec
INSERT INTO workflow_transitions (version_id, from_step_id, to_step_id, effect) VALUES ($1, $2, $3, $4);

-- name: ListWorkflowSteps :many
SELECT * FROM workflow_steps WHERE version_id = $1 ORDER BY sort_order, key;

-- name: ListWorkflowTransitions :many
SELECT t.id, t.version_id, f.key AS from_key, n.key AS to_key, t.effect
FROM workflow_transitions t
JOIN workflow_steps f ON f.id = t.from_step_id
JOIN workflow_steps n ON n.id = t.to_step_id
WHERE t.version_id = $1
ORDER BY f.sort_order, n.sort_order;

-- name: LockOrderForTransition :one
-- WF-05: the order row is locked for the transaction, and its real version is
-- what the optimistic lock checks (ORD-04: never version 0).
SELECT id, status, version, workflow_version_id FROM orders WHERE id = $1 AND archived = false FOR UPDATE;

-- name: InsertOrderStatusHistory :exec
INSERT INTO order_status_history (order_id, from_key, to_key, actor_id, reason, is_override)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: ListOrderStatusHistory :many
SELECT * FROM order_status_history WHERE order_id = $1 ORDER BY created_at, id;

-- name: ListOrderLinesForEngine :many
SELECT id, product_id, quantity FROM order_lines WHERE order_id = $1 AND archived = false ORDER BY id;

-- name: ReservedByLocationForLine :many
-- WF-03: where an order line still holds a reservation.
SELECT location_id, (-SUM(quantity))::bigint AS reserved
FROM stockmovements
WHERE reference_type = 'order_line' AND reference_id = $1 AND movement_type IN ('reservation', 'release')
GROUP BY location_id
HAVING -SUM(quantity) > 0
ORDER BY location_id;

-- name: SoldByBatchForLine :many
-- WF-05: what an order line has taken out of stock and not had returned.
SELECT location_id, batch_number, (-SUM(quantity))::bigint AS sold
FROM stockmovements
WHERE reference_type = 'order_line' AND reference_id = $1 AND movement_type IN ('sale', 'return')
GROUP BY location_id, batch_number
HAVING -SUM(quantity) > 0
ORDER BY location_id, batch_number;

-- name: GetOrderForTransition :one
-- WF-05 preview: the same as LockOrderForTransition, without the lock.
SELECT id, status, version, workflow_version_id FROM orders WHERE id = $1 AND archived = false;
