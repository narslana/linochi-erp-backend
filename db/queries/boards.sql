-- name: ListBoards :many
SELECT id, name, archived_at, version, created_at, updated_at
FROM boards
WHERE archived_at IS NULL
ORDER BY created_at;

-- name: GetBoard :one
SELECT id, name, archived_at, version, created_at, updated_at
FROM boards
WHERE id = $1 AND archived_at IS NULL;

-- name: CreateBoard :one
INSERT INTO boards (id, name)
VALUES ($1, $2)
RETURNING id, name, archived_at, version, created_at, updated_at;

-- name: UpdateBoard :one
UPDATE boards
SET name = $1, version = version + 1, updated_at = now()
WHERE id = $2 AND version = $3
RETURNING id, name, archived_at, version, created_at, updated_at;

-- name: ArchiveBoard :one
UPDATE boards
SET archived_at = now(), version = version + 1, updated_at = now()
WHERE id = $1
RETURNING id, name, archived_at, version, created_at, updated_at;

-- name: ListColumnsForBoard :many
SELECT id, board_id, name, sort_order, archived_at, version, created_at, updated_at
FROM board_columns
WHERE board_id = $1 AND archived_at IS NULL
ORDER BY sort_order;

-- name: GetColumn :one
SELECT id, board_id, name, sort_order, archived_at, version, created_at, updated_at
FROM board_columns
WHERE id = $1 AND archived_at IS NULL;

-- name: CreateColumn :one
INSERT INTO board_columns (id, board_id, name, sort_order)
VALUES ($1, $2, $3, $4)
RETURNING id, board_id, name, sort_order, archived_at, version, created_at, updated_at;

-- name: UpdateColumn :one
UPDATE board_columns
SET name = $1, sort_order = $2, version = version + 1, updated_at = now()
WHERE id = $3 AND version = $4
RETURNING id, board_id, name, sort_order, archived_at, version, created_at, updated_at;

-- name: ArchiveColumn :one
UPDATE board_columns
SET archived_at = now(), version = version + 1, updated_at = now()
WHERE id = $1
RETURNING id, board_id, name, sort_order, archived_at, version, created_at, updated_at;

-- name: ListCardsForBoard :many
SELECT id, board_id, column_id, title, description_md, assignee_id, due_date, labels, archived_at, version, created_at, updated_at
FROM cards
WHERE board_id = $1 AND archived_at IS NULL
ORDER BY created_at;

-- name: ListCardsForColumn :many
SELECT id, board_id, column_id, title, description_md, assignee_id, due_date, labels, archived_at, version, created_at, updated_at
FROM cards
WHERE column_id = $1 AND archived_at IS NULL
ORDER BY created_at;

-- name: GetCard :one
SELECT id, board_id, column_id, title, description_md, assignee_id, due_date, labels, archived_at, version, created_at, updated_at
FROM cards
WHERE id = $1 AND archived_at IS NULL;

-- name: CreateCard :one
INSERT INTO cards (id, board_id, column_id, title, description_md, assignee_id, due_date, labels)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, board_id, column_id, title, description_md, assignee_id, due_date, labels, archived_at, version, created_at, updated_at;

-- name: UpdateCard :one
UPDATE cards
SET title = $1, column_id = $2, description_md = $3, assignee_id = $4, due_date = $5, labels = $6, version = version + 1, updated_at = now()
WHERE id = $7 AND version = $8
RETURNING id, board_id, column_id, title, description_md, assignee_id, due_date, labels, archived_at, version, created_at, updated_at;

-- name: ArchiveCard :one
UPDATE cards
SET archived_at = now(), version = version + 1, updated_at = now()
WHERE id = $1
RETURNING id, board_id, column_id, title, description_md, assignee_id, due_date, labels, archived_at, version, created_at, updated_at;

-- name: ListChecklistItemsForCard :many
SELECT id, card_id, title, completed, sort_order, archived_at, version, created_at, updated_at
FROM card_checklist_items
WHERE card_id = $1 AND archived_at IS NULL
ORDER BY sort_order;

-- name: GetChecklistItem :one
SELECT id, card_id, title, completed, sort_order, archived_at, version, created_at, updated_at
FROM card_checklist_items
WHERE id = $1 AND archived_at IS NULL;

-- name: CreateChecklistItem :one
INSERT INTO card_checklist_items (id, card_id, title, completed, sort_order)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, card_id, title, completed, sort_order, archived_at, version, created_at, updated_at;

-- name: UpdateChecklistItem :one
UPDATE card_checklist_items
SET title = $1, completed = $2, sort_order = $3, version = version + 1, updated_at = now()
WHERE id = $4 AND version = $5
RETURNING id, card_id, title, completed, sort_order, archived_at, version, created_at, updated_at;

-- name: ArchiveChecklistItem :one
UPDATE card_checklist_items
SET archived_at = now(), version = version + 1, updated_at = now()
WHERE id = $1
RETURNING id, card_id, title, completed, sort_order, archived_at, version, created_at, updated_at;

-- name: ListCommentsForCard :many
SELECT id, card_id, body_md, author_id, archived_at, version, created_at, updated_at
FROM card_comments
WHERE card_id = $1 AND archived_at IS NULL
ORDER BY created_at;

-- name: CreateComment :one
INSERT INTO card_comments (id, card_id, body_md, author_id)
VALUES ($1, $2, $3, $4)
RETURNING id, card_id, body_md, author_id, archived_at, version, created_at, updated_at;
