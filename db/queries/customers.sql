-- name: ListCustomers :many
SELECT * FROM customers WHERE archived = false ORDER BY name;

-- name: GetCustomer :one
SELECT * FROM customers WHERE id = $1 AND archived = false;

-- name: CreateCustomer :one
INSERT INTO customers (
    id, name, type_code, address, city, country, eori, epr, currency, notes
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
) RETURNING *;

-- name: UpdateCustomer :one
UPDATE customers SET
    name = COALESCE(sqlc.arg(name), name),
    type_code = COALESCE(sqlc.arg(type_code), type_code),
    address = COALESCE(sqlc.arg(address), address),
    city = COALESCE(sqlc.arg(city), city),
    country = COALESCE(sqlc.arg(country), country),
    eori = COALESCE(sqlc.arg(eori), eori),
    epr = COALESCE(sqlc.arg(epr), epr),
    currency = COALESCE(sqlc.arg(currency), currency),
    notes = COALESCE(sqlc.arg(notes), notes),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2 AND archived = false
RETURNING *;

-- name: ArchiveCustomer :one
UPDATE customers SET
    archived = true,
    archived_at = now(),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2
RETURNING *;

-- name: AnonymiseContact :one
UPDATE contacts SET
    first_name = '',
    last_name = '',
    email = '',
    phone = '',
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2
RETURNING *;

-- name: ListContacts :many
SELECT * FROM contacts WHERE customer_id = $1 AND archived = false ORDER BY last_name, first_name;

-- name: GetContact :one
SELECT * FROM contacts WHERE id = $1;

-- name: CreateContact :one
INSERT INTO contacts (
    id, customer_id, first_name, last_name, email, phone
) VALUES (
    $1, $2, $3, $4, $5, $6
) RETURNING *;

-- name: UpdateContact :one
UPDATE contacts SET
    first_name = COALESCE(sqlc.arg(first_name), first_name),
    last_name = COALESCE(sqlc.arg(last_name), last_name),
    email = COALESCE(sqlc.arg(email), email),
    phone = COALESCE(sqlc.arg(phone), phone),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2 AND archived = false
RETURNING *;

-- name: ArchiveContact :one
UPDATE contacts SET
    archived = true,
    archived_at = now(),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2
RETURNING *;

-- name: ListLeads :many
SELECT * FROM leads WHERE archived = false ORDER BY created_at DESC;

-- name: GetLead :one
SELECT * FROM leads WHERE id = $1 AND archived = false;

-- name: CreateLead :one
INSERT INTO leads (
    id, company, contact_name, email, phone, address, city, country, status, notes
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
) RETURNING *;

-- name: UpdateLead :one
UPDATE leads SET
    company = COALESCE(sqlc.arg(company), company),
    contact_name = COALESCE(sqlc.arg(contact_name), contact_name),
    email = COALESCE(sqlc.arg(email), email),
    phone = COALESCE(sqlc.arg(phone), phone),
    address = COALESCE(sqlc.arg(address), address),
    city = COALESCE(sqlc.arg(city), city),
    country = COALESCE(sqlc.arg(country), country),
    status = COALESCE(sqlc.arg(status), status),
    notes = COALESCE(sqlc.arg(notes), notes),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2 AND archived = false
RETURNING *;

-- name: ArchiveLead :one
UPDATE leads SET
    archived = true,
    archived_at = now(),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2
RETURNING *;

-- name: ArchiveLeadNoVersion :one
UPDATE leads SET
    archived = true,
    archived_at = now(),
    version = version + 1,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: InactiveCustomers :many
-- KUN-05: customers who have not ordered in X days
SELECT c.id, c.name, c.type_code,
       MAX(o.created_at) AS last_order_date,
       COUNT(o.id) AS order_count
FROM customers c
LEFT JOIN orders o ON o.customer_id = c.id AND o.archived = false
WHERE c.archived = false
GROUP BY c.id, c.name, c.type_code
HAVING MAX(o.created_at) IS NULL OR MAX(o.created_at) < now() - ($1::integer || ' days')::interval
ORDER BY last_order_date NULLS FIRST;

-- name: CustomerStats :one
-- KUN-04: customer card statistics
SELECT
    COUNT(o.id) AS total_orders,
    COALESCE(SUM(o.total_sek), 0) AS total_value,
    COALESCE(AVG(o.total_sek), 0) AS avg_order_value,
    MAX(o.created_at) AS latest_order,
    o.channel,
    COUNT(DISTINCT o.channel) AS channel_count
FROM orders o
WHERE o.customer_id = $1 AND o.archived = false
GROUP BY o.channel;

-- name: TopProductsByCustomer :many
-- KUN-04: most purchased products per customer
SELECT ol.product_id, p.name, SUM(ol.quantity) AS total_quantity
FROM order_lines ol
JOIN orders o ON o.id = ol.order_id AND o.customer_id = $1 AND o.archived = false
JOIN products p ON p.id = ol.product_id AND p.archived = false
WHERE ol.archived = false
GROUP BY ol.product_id, p.name
ORDER BY total_quantity DESC
LIMIT 10;

-- name: ListEanSeries :many
SELECT * FROM ean_series WHERE archived = false ORDER BY prefix, start_code;

-- name: GetEanSeries :one
SELECT * FROM ean_series WHERE id = $1 AND archived = false;

-- name: CreateEanSeries :one
INSERT INTO ean_series (
    id, prefix, start_code, end_code, description
) VALUES (
    $1, $2, $3, $4, $5
) RETURNING *;

-- name: UpdateEanSeries :one
UPDATE ean_series SET
    prefix = COALESCE(sqlc.arg(prefix), prefix),
    start_code = COALESCE(sqlc.arg(start_code), start_code),
    end_code = COALESCE(sqlc.arg(end_code), end_code),
    description = COALESCE(sqlc.arg(description), description),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2 AND archived = false
RETURNING *;

-- name: ArchiveEanSeries :one
UPDATE ean_series SET
    archived = true,
    archived_at = now(),
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2
RETURNING *;

-- name: NextFreeEanCode :one
-- REG-04: suggest the next free EAN code for a product
SELECT es.start_code
FROM ean_series es
WHERE es.archived = false
  AND es.start_code NOT IN (
    SELECT p.ean_code FROM products p WHERE p.ean_code IS NOT NULL AND p.ean_code != ''
  )
ORDER BY es.start_code
LIMIT 1;

-- name: ListCustomerEANSeries :many
-- KUN-07: list EAN series (delegates to global list until customer-specific linkage exists)
SELECT * FROM ean_series WHERE archived = false ORDER BY prefix, start_code;

-- name: ListCustomerLeads :many
-- KUN-05: list leads (delegates to global list until customer-specific linkage exists)
SELECT * FROM leads WHERE archived = false ORDER BY created_at DESC;

-- name: RegisterCustomerEANSeries :one
-- KUN-07: register an EAN series for a customer
INSERT INTO ean_series (
    id, prefix, start_code, end_code, description
) VALUES (
    gen_random_uuid(), $1, $2, $3, $4
) RETURNING *;

-- name: RegisterEANSeries :one
-- REG-04: register a new EAN series
INSERT INTO ean_series (
    prefix, start_code, end_code, description
) VALUES (
    $1, $2, $3, $4
) RETURNING *;
