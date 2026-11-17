-- name: ListSuppliers :many
SELECT * FROM suppliers WHERE archived = false ORDER BY name;

-- name: GetSupplier :one
SELECT * FROM suppliers WHERE id = $1 AND archived = false;

-- name: CreateSupplier :one
INSERT INTO suppliers (
    id, name, name_en, org_number, vat_number, country, address, city,
    postal_code, website, contact_email, contact_phone, role, lead_time_days,
    moq, currency, payment_terms, delivery_terms, notes
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
    $15, $16, $17, $18, $19
) RETURNING *;

-- name: UpdateSupplier :one
UPDATE suppliers SET
    name = COALESCE(sqlc.narg(name), name),
    name_en = COALESCE(sqlc.narg(name_en), name_en),
    org_number = COALESCE(sqlc.narg(org_number), org_number),
    vat_number = COALESCE(sqlc.narg(vat_number), vat_number),
    country = COALESCE(sqlc.narg(country), country),
    address = COALESCE(sqlc.narg(address), address),
    city = COALESCE(sqlc.narg(city), city),
    postal_code = COALESCE(sqlc.narg(postal_code), postal_code),
    website = COALESCE(sqlc.narg(website), website),
    contact_email = COALESCE(sqlc.narg(contact_email), contact_email),
    contact_phone = COALESCE(sqlc.narg(contact_phone), contact_phone),
    role = COALESCE(sqlc.narg(role), role),
    lead_time_days = COALESCE(sqlc.narg(lead_time_days), lead_time_days),
    moq = COALESCE(sqlc.narg(moq), moq),
    currency = COALESCE(sqlc.narg(currency), currency),
    payment_terms = COALESCE(sqlc.narg(payment_terms), payment_terms),
    delivery_terms = COALESCE(sqlc.narg(delivery_terms), delivery_terms),
    notes = COALESCE(sqlc.narg(notes), notes),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2 AND archived = false
RETURNING *;

-- name: ArchiveSupplier :one
UPDATE suppliers SET
    archived = true,
    archived_at = now(),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2
RETURNING *;

-- name: ListSupplierContacts :many
SELECT * FROM supplier_contacts WHERE supplier_id = $1 AND archived = false ORDER BY name;

-- name: GetSupplierContact :one
SELECT * FROM supplier_contacts WHERE id = $1 AND archived = false;

-- name: CreateSupplierContact :one
INSERT INTO supplier_contacts (
    id, supplier_id, name, name_en, email, phone, role
) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING *;

-- name: UpdateSupplierContact :one
UPDATE supplier_contacts SET
    name = COALESCE(sqlc.narg(name), name),
    name_en = COALESCE(sqlc.narg(name_en), name_en),
    email = COALESCE(sqlc.narg(email), email),
    phone = COALESCE(sqlc.narg(phone), phone),
    role = COALESCE(sqlc.narg(role), role),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2 AND archived = false
RETURNING *;

-- name: ArchiveSupplierContact :one
UPDATE supplier_contacts SET
    archived = true,
    archived_at = now(),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2
RETURNING *;

-- name: ListSupplierProducts :many
SELECT * FROM supplier_products WHERE supplier_id = $1 AND archived = false ORDER BY id;

-- name: GetSupplierProduct :one
SELECT * FROM supplier_products WHERE id = $1;

-- name: CreateSupplierProduct :one
INSERT INTO supplier_products (
    id, supplier_id, product_id, purchase_price, purchase_currency
) VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: ArchiveSupplierProduct :one
UPDATE supplier_products SET
    archived = true,
    archived_at = now(),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2
RETURNING *;

-- name: ListPurchaseOrders :many
SELECT * FROM purchase_orders WHERE archived = false ORDER BY order_date DESC;

-- name: ListPurchaseOrdersByStatus :many
SELECT * FROM purchase_orders WHERE archived = false AND status = $1 ORDER BY order_date DESC;

-- name: ListPurchaseOrdersBySupplier :many
SELECT * FROM purchase_orders WHERE supplier_id = $1 AND archived = false ORDER BY order_date DESC;

-- name: GetPurchaseOrder :one
SELECT * FROM purchase_orders WHERE id = $1 AND archived = false;

-- name: CreatePurchaseOrder :one
INSERT INTO purchase_orders (
    id, supplier_id, order_number, currency, order_date, expected_date,
    status, total_amount, notes
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING *;

-- name: UpdatePurchaseOrder :one
UPDATE purchase_orders SET
    status = COALESCE(sqlc.narg(status), status),
    expected_date = COALESCE(sqlc.narg(expected_date), expected_date),
    total_amount = COALESCE(sqlc.narg(total_amount), total_amount),
    notes = COALESCE(sqlc.narg(notes), notes),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2 AND archived = false
RETURNING *;

-- name: ArchivePurchaseOrder :one
UPDATE purchase_orders SET
    archived = true,
    archived_at = now(),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2
RETURNING *;

-- name: ListPurchaseOrderLines :many
SELECT * FROM purchase_order_lines WHERE purchase_order_id = $1 AND archived = false ORDER BY id;

-- name: GetPurchaseOrderLine :one
SELECT * FROM purchase_order_lines WHERE id = $1;

-- name: CreatePurchaseOrderLine :one
INSERT INTO purchase_order_lines (
    id, purchase_order_id, product_id, quantity, unit_price,
    unit_currency, line_total
) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING *;

-- name: UpdatePurchaseOrderLine :one
UPDATE purchase_order_lines SET
    quantity = COALESCE(sqlc.narg(quantity), quantity),
    unit_price = COALESCE(sqlc.narg(unit_price), unit_price),
    line_total = COALESCE(sqlc.narg(line_total), line_total),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2 AND archived = false
RETURNING *;

-- name: ArchivePurchaseOrderLine :one
UPDATE purchase_order_lines SET
    archived = true,
    archived_at = now(),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2
RETURNING *;

-- name: ListShipments :many
SELECT * FROM shipments WHERE archived = false ORDER BY created_at DESC;

-- name: ListShipmentsByPurchaseOrder :many
SELECT * FROM shipments WHERE purchase_order_id = $1 AND archived = false ORDER BY created_at DESC;

-- name: ListShipmentsByStatus :many
SELECT * FROM shipments WHERE status = $1 AND archived = false ORDER BY created_at DESC;

-- name: GetShipment :one
SELECT * FROM shipments WHERE id = $1 AND archived = false;

-- name: CreateShipment :one
INSERT INTO shipments (
    id, purchase_order_id, from_location, to_location, carrier,
    tracking_number, shipped_at, estimated_arrival, actual_arrival,
    batch_number, status, documents
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING *;

-- name: UpdateShipment :one
UPDATE shipments SET
    from_location = COALESCE(sqlc.narg(from_location), from_location),
    to_location = COALESCE(sqlc.narg(to_location), to_location),
    carrier = COALESCE(sqlc.narg(carrier), carrier),
    tracking_number = COALESCE(sqlc.narg(tracking_number), tracking_number),
    shipped_at = COALESCE(sqlc.narg(shipped_at), shipped_at),
    estimated_arrival = COALESCE(sqlc.narg(estimated_arrival), estimated_arrival),
    actual_arrival = COALESCE(sqlc.narg(actual_arrival), actual_arrival),
    batch_number = COALESCE(sqlc.narg(batch_number), batch_number),
    status = COALESCE(sqlc.narg(status), status),
    documents = COALESCE(sqlc.narg(documents), documents),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2 AND archived = false
RETURNING *;

-- name: ArchiveShipment :one
UPDATE shipments SET
    archived = true,
    archived_at = now(),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2
RETURNING *;
