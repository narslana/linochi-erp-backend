-- name: ListOrders :many
SELECT * FROM orders WHERE archived = false
  AND (sqlc.arg(status)::text IS NULL OR status = sqlc.arg(status))
  AND (sqlc.arg(customer_id)::uuid IS NULL OR customer_id = sqlc.arg(customer_id))
  AND (sqlc.arg(from_date)::date IS NULL OR created_at >= sqlc.arg(from_date)::date)
  AND (sqlc.arg(to_date)::date IS NULL OR created_at < sqlc.arg(to_date)::date + interval '1 day')
ORDER BY created_at DESC;

-- name: GetOrder :one
SELECT * FROM orders WHERE id = $1 AND archived = false;

-- name: GetOrderByExternalId :one
SELECT * FROM orders WHERE external_id = $1 AND external_id != '' AND archived = false;

-- name: CreateOrder :one
INSERT INTO orders (
    id, external_id, channel, status, currency, exchange_rate_sek,
    subtotal_sek, tax_sek, freight_sek, commission_sek, total_sek,
    customer_id, shipping_name, shipping_address, shipping_city,
    shipping_postal_code, shipping_country, billing_name, billing_address,
    billing_city, billing_postal_code, billing_country,
    tracking_number, carrier, shipping_label_url, delivered_at, notes
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
    $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27
) RETURNING *;

-- name: UpdateOrder :one
UPDATE orders SET
    external_id = COALESCE(sqlc.narg(external_id), external_id),
    channel = COALESCE(sqlc.narg(channel), channel),
    status = COALESCE(sqlc.narg(status), status),
    currency = COALESCE(sqlc.narg(currency), currency),
    exchange_rate_sek = COALESCE(sqlc.narg(exchange_rate_sek), exchange_rate_sek),
    subtotal_sek = COALESCE(sqlc.narg(subtotal_sek), subtotal_sek),
    tax_sek = COALESCE(sqlc.narg(tax_sek), tax_sek),
    freight_sek = COALESCE(sqlc.narg(freight_sek), freight_sek),
    commission_sek = COALESCE(sqlc.narg(commission_sek), commission_sek),
    total_sek = COALESCE(sqlc.narg(total_sek), total_sek),
    customer_id = COALESCE(sqlc.narg(customer_id), customer_id),
    shipping_name = COALESCE(sqlc.narg(shipping_name), shipping_name),
    shipping_address = COALESCE(sqlc.narg(shipping_address), shipping_address),
    shipping_city = COALESCE(sqlc.narg(shipping_city), shipping_city),
    shipping_postal_code = COALESCE(sqlc.narg(shipping_postal_code), shipping_postal_code),
    shipping_country = COALESCE(sqlc.narg(shipping_country), shipping_country),
    billing_name = COALESCE(sqlc.narg(billing_name), billing_name),
    billing_address = COALESCE(sqlc.narg(billing_address), billing_address),
    billing_city = COALESCE(sqlc.narg(billing_city), billing_city),
    billing_postal_code = COALESCE(sqlc.narg(billing_postal_code), billing_postal_code),
    billing_country = COALESCE(sqlc.narg(billing_country), billing_country),
    tracking_number = COALESCE(sqlc.narg(tracking_number), tracking_number),
    carrier = COALESCE(sqlc.narg(carrier), carrier),
    shipping_label_url = COALESCE(sqlc.narg(shipping_label_url), shipping_label_url),
    notes = COALESCE(sqlc.narg(notes), notes),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2 AND archived = false
RETURNING *;

-- name: ArchiveOrder :one
UPDATE orders SET
    archived = true,
    archived_at = now(),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2
RETURNING *;

-- name: UpdateOrderStatus :one
UPDATE orders SET
    status = $2,
    shipped_at = COALESCE(sqlc.narg(shipped_at), shipped_at),
    delivered_at = COALESCE(sqlc.narg(delivered_at), delivered_at),
    tracking_number = COALESCE(sqlc.narg(tracking_number), tracking_number),
    carrier = COALESCE(sqlc.narg(carrier), carrier),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $3 AND archived = false
RETURNING *;

-- name: ListOrderLines :many
SELECT * FROM order_lines WHERE order_id = $1 AND archived = false ORDER BY id;

-- name: GetOrderLine :one
SELECT * FROM order_lines WHERE id = $1 AND archived = false;

-- name: CreateOrderLine :one
INSERT INTO order_lines (
    id, order_id, product_id, quantity, quantity_shipped,
    unit_price_sek, tax_rate, tax_amount_sek, total_sek
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
) RETURNING *;

-- name: UpdateOrderLine :one
UPDATE order_lines SET
    quantity = COALESCE(sqlc.narg(quantity), quantity),
    quantity_shipped = COALESCE(sqlc.narg(quantity_shipped), quantity_shipped),
    unit_price_sek = COALESCE(sqlc.narg(unit_price_sek), unit_price_sek),
    tax_rate = COALESCE(sqlc.narg(tax_rate), tax_rate),
    tax_amount_sek = COALESCE(sqlc.narg(tax_amount_sek), tax_amount_sek),
    total_sek = COALESCE(sqlc.narg(total_sek), total_sek),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2 AND archived = false
RETURNING *;

-- name: ArchiveOrderLine :one
UPDATE order_lines SET
    archived = true,
    archived_at = now(),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2
RETURNING *;
