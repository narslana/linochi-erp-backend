-- name: ListLocations :many
SELECT * FROM locations WHERE archived = false ORDER BY name;

-- name: GetLocation :one
SELECT * FROM locations WHERE id = $1 AND archived = false;

-- name: CreateLocation :one
INSERT INTO locations (
    id, name, name_en, address, city, country, channel_buffer
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING *;

-- name: UpdateLocation :one
UPDATE locations SET
    name = COALESCE(sqlc.arg(name), name),
    name_en = COALESCE(sqlc.arg(name_en), name_en),
    address = COALESCE(sqlc.arg(address), address),
    city = COALESCE(sqlc.arg(city), city),
    country = COALESCE(sqlc.arg(country), country),
    channel_buffer = COALESCE(sqlc.arg(channel_buffer), channel_buffer),
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

-- name: BalanceByLocation :many
-- LAG-01: total balance per location
SELECT location_id,
       COALESCE(SUM(quantity), 0) AS in_stock
FROM stockmovements
WHERE product_id = $1
GROUP BY location_id;

-- name: BalanceByProduct :many
-- LAG-01, LAG-02: balance per product per location
SELECT product_id, location_id,
       COALESCE(SUM(quantity), 0) AS in_stock
FROM stockmovements
GROUP BY product_id, location_id;

-- name: AvailableBalance :many
-- LAG-02: in_stock - reserved = available
SELECT product_id, location_id,
       COALESCE(SUM(CASE WHEN movement_type = 'reservation' THEN quantity ELSE 0 END), 0) AS reserved,
       COALESCE(SUM(CASE WHEN movement_type NOT IN ('reservation') THEN quantity ELSE 0 END), 0) AS available
FROM stockmovements
WHERE product_id = $1
GROUP BY product_id, location_id;

-- name: MovementsForFEFO :many
-- LAG-03: stock available for a product at a location, ordered FEFO (oldest first)
SELECT product_id, location_id, batch_number,
       manufacturing_date, best_before_date,
       SUM(quantity) AS in_stock
FROM stockmovements
WHERE product_id = $1 AND location_id = $2
  AND movement_type != 'reservation'
  AND movement_type != 'release'
GROUP BY product_id, location_id, batch_number, manufacturing_date, best_before_date
HAVING SUM(quantity) > 0
ORDER BY best_before_date ASC NULLS LAST, manufacturing_date ASC NULLS LAST;

-- name: InventoryValueByLocation :many
-- LAG-05: inventory value = quantity * manufacturing cost per location
-- Manufacturing cost comes from the supplier_products.purchase_price
SELECT sm.location_id,
       p.id AS product_id,
       p.name AS product_name,
       COALESCE(SUM(sm.quantity), 0) AS in_stock,
       COALESCE(SUM(sm.quantity * sp.purchase_price), 0) AS value
FROM stockmovements sm
JOIN products p ON p.id = sm.product_id AND p.archived = false
LEFT JOIN supplier_products sp ON sp.product_id = p.id AND sp.archived = false
WHERE sm.movement_type != 'reservation' AND sm.movement_type != 'release'
GROUP BY sm.location_id, p.id, p.name;
