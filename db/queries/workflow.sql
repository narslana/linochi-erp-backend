-- name: ListWorkflowSteps :many
SELECT * FROM workflow_steps WHERE archived = false ORDER BY sort_order, key;

-- name: GetWorkflowStep :one
SELECT * FROM workflow_steps WHERE key = $1 AND archived = false;

-- name: CreateWorkflowStep :one
INSERT INTO workflow_steps (
    id, key, name, name_en, sort_order, is_start, is_end
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING *;

-- name: UpdateWorkflowStep :one
UPDATE workflow_steps SET
    name = COALESCE(sqlc.arg(name), name),
    name_en = COALESCE(sqlc.arg(name_en), name_en),
    sort_order = COALESCE(sqlc.arg(sort_order), sort_order),
    is_start = COALESCE(sqlc.arg(is_start), is_start),
    is_end = COALESCE(sqlc.arg(is_end), is_end),
    version = version + 1,
    updated_at = now()
WHERE key = $1 AND version = $2 AND archived = false
RETURNING *;

-- name: ArchiveWorkflowStep :one
UPDATE workflow_steps SET
    archived = true,
    archived_at = now(),
    version = version + 1,
    updated_at = now()
WHERE key = $1 AND version = $2
RETURNING *;

-- name: ListWorkflowTransitions :many
SELECT * FROM workflow_transitions WHERE step_key = $1 AND archived = false ORDER BY sort_order;

-- name: GetWorkflowTransition :one
SELECT * FROM workflow_transitions WHERE id = $1 AND archived = false;

-- name: CreateWorkflowTransition :one
INSERT INTO workflow_transitions (
    id, step_key, from_key, to_key, label, label_en, sort_order
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING *;

-- name: UpdateWorkflowTransition :one
UPDATE workflow_transitions SET
    from_key = COALESCE(sqlc.arg(from_key), from_key),
    to_key = COALESCE(sqlc.arg(to_key), to_key),
    label = COALESCE(sqlc.arg(label), label),
    label_en = COALESCE(sqlc.arg(label_en), label_en),
    sort_order = COALESCE(sqlc.arg(sort_order), sort_order),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2 AND archived = false
RETURNING *;

-- name: ArchiveWorkflowTransition :one
UPDATE workflow_transitions SET
    archived = true,
    archived_at = now(),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2
RETURNING *;

-- name: ListWorkflowConditions :many
SELECT * FROM workflow_conditions WHERE transition_id = $1 AND archived = false;

-- name: CreateWorkflowCondition :one
INSERT INTO workflow_conditions (
    id, transition_id, kind, config
) VALUES (
    $1, $2, $3, $4
) RETURNING *;

-- name: ArchiveWorkflowCondition :one
UPDATE workflow_conditions SET
    archived = true,
    archived_at = now(),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2
RETURNING *;

-- name: ListWorkflowEffects :many
SELECT * FROM workflow_effects WHERE transition_id = $1 AND archived = false;

-- name: CreateWorkflowEffect :one
INSERT INTO workflow_effects (
    id, transition_id, kind, config
) VALUES (
    $1, $2, $3, $4
) RETURNING *;

-- name: ArchiveWorkflowEffect :one
UPDATE workflow_effects SET
    archived = true,
    archived_at = now(),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2
RETURNING *;
