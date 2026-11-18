-- name: ListLocations :many
SELECT * FROM locations WHERE archived = false ORDER BY name;

-- name: GetLocation :one
SELECT * FROM locations WHERE id = $1 AND archived = false;

-- name: CreateLocation :one
INSERT INTO locations (
    id, name, name_en, address, city, country
) VALUES (
    $1, $2, $3, $4, $5, $6
) RETURNING *;

-- name: UpdateLocation :one
UPDATE locations SET
    name = COALESCE(sqlc.arg(name), name),
    name_en = COALESCE(sqlc.arg(name_en), name_en),
    address = COALESCE(sqlc.arg(address), address),
    city = COALESCE(sqlc.arg(city), city),
    country = COALESCE(sqlc.arg(country), country),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2 AND archived = false
RETURNING *;

-- name: ArchiveLocation :one
UPDATE locations SET
    archived = true,
    archived_at = now(),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2
RETURNING *;

-- name: ListStockMovements :many
SELECT * FROM stockmovements
ORDER BY created_at DESC;

-- name: CreateStockMovement :one
INSERT INTO stockmovements (
    id, product_id, location_id, batch_number,
    manufacturing_date, best_before_date,
    movement_type, quantity, reason,
    reference_type, reference_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
) RETURNING *;

-- name: LockProductStock :exec
-- LAG-02: serialises everything that reads and then writes the balance of one
-- product. The lock is released at commit or rollback.
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(product_id)::uuid::text, 0));

-- name: BalanceByLocation :many
-- LAG-01, LAG-02: in stock, reserved and available per location.
-- in_stock is every movement except reservation and release; reserved is the
-- negated sum of reservation and release; available = in_stock - reserved.
SELECT location_id,
       COALESCE(SUM(quantity) FILTER (WHERE movement_type NOT IN ('reservation', 'release')), 0)::bigint AS in_stock,
       (-COALESCE(SUM(quantity) FILTER (WHERE movement_type IN ('reservation', 'release')), 0))::bigint AS reserved
FROM stockmovements
WHERE product_id = $1
GROUP BY location_id
ORDER BY location_id;

-- name: LocationBalance :one
SELECT COALESCE(SUM(quantity) FILTER (WHERE movement_type NOT IN ('reservation', 'release')), 0)::bigint AS in_stock,
       (-COALESCE(SUM(quantity) FILTER (WHERE movement_type IN ('reservation', 'release')), 0))::bigint AS reserved
FROM stockmovements
WHERE product_id = $1 AND location_id = $2;

-- name: ReservedForReference :one
-- LAG-02: what is still reserved for one reference (an order line).
SELECT (-COALESCE(SUM(quantity), 0))::bigint AS reserved
FROM stockmovements
WHERE product_id = $1 AND location_id = $2
  AND reference_type = $3 AND reference_id = $4
  AND movement_type IN ('reservation', 'release');

-- name: BatchStock :many
-- LAG-03: physical stock per batch at a location, FEFO order (earliest
-- best-before first, no date last). A sale row carries only the batch number,
-- so the batch is grouped by number and the dates come from its receipt.
SELECT batch_number,
       MIN(manufacturing_date)::date AS manufacturing_date,
       MIN(best_before_date)::date AS best_before_date,
       SUM(quantity)::bigint AS in_stock
FROM stockmovements
WHERE product_id = $1 AND location_id = $2
  AND movement_type NOT IN ('reservation', 'release')
GROUP BY batch_number
HAVING SUM(quantity) > 0
ORDER BY MIN(best_before_date) ASC NULLS LAST, MIN(manufacturing_date) ASC NULLS LAST, batch_number;

-- name: InventoryValueByLocation :many
-- LAG-05: inventory value = quantity in stock x manufacturing cost, per product
-- per location, in öre. Supplier prices are not used, so several suppliers on a
-- product cannot multiply the rows. An unknown cost counts as 0.
SELECT sm.location_id,
       p.id AS product_id,
       p.name AS product_name,
       SUM(sm.quantity)::bigint AS in_stock,
       ROUND(SUM(sm.quantity) * COALESCE(p.manufacturing_cost, 0) * 100)::bigint AS value
FROM stockmovements sm
JOIN products p ON p.id = sm.product_id AND p.archived = false
WHERE sm.movement_type NOT IN ('reservation', 'release')
GROUP BY sm.location_id, p.id, p.name, p.manufacturing_cost
ORDER BY p.name, sm.location_id;

-- name: ProductChannelBuffer :one
SELECT channel_buffer FROM products WHERE id = $1;

-- name: InsertOrderLineBatch :exec
-- LAG-03: the shipped batch is recorded for recall traceability.
INSERT INTO order_line_batches (order_line_id, batch_number, quantity, movement_id)
VALUES ($1, $2, $3, $4);

-- name: AddOrderLineShipped :exec
UPDATE order_lines SET
    quantity_shipped = quantity_shipped + $2,
    version = version + 1,
    updated_at = now()
WHERE id = $1;
