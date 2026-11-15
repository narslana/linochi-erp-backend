-- name: ListFieldDefinitions :many
SELECT id, name, name_en, field_type, group_name, help_text, mandatory, choices, unit, show_on_card, archived, archived_at, version, created_at, updated_at
FROM field_definitions
WHERE archived = false
ORDER BY group_name, name;

-- name: GetFieldDefinition :one
SELECT id, name, name_en, field_type, group_name, help_text, mandatory, choices, unit, show_on_card, archived, archived_at, version, created_at, updated_at
FROM field_definitions
WHERE id = $1;

-- name: CreateFieldDefinition :one
INSERT INTO field_definitions (id, name, name_en, field_type, group_name, help_text, mandatory, choices, unit, show_on_card)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id, name, name_en, field_type, group_name, help_text, mandatory, choices, unit, show_on_card, archived, archived_at, version, created_at, updated_at;

-- name: UpdateFieldDefinition :one
UPDATE field_definitions
SET name = $1,
    name_en = $2,
    field_type = $3,
    group_name = $4,
    help_text = $5,
    mandatory = $6,
    choices = $7,
    unit = $8,
    show_on_card = $9,
    updated_at = now(),
    version = version + 1
WHERE id = $10 AND version = $11
RETURNING id, name, name_en, field_type, group_name, help_text, mandatory, choices, unit, show_on_card, archived, archived_at, version, created_at, updated_at;

-- name: ArchiveFieldDefinition :one
UPDATE field_definitions
SET archived = true, archived_at = now(), updated_at = now(), version = version + 1
WHERE id = $1 AND version = $2
RETURNING id, name, name_en, field_type, group_name, help_text, mandatory, choices, unit, show_on_card, archived, archived_at, version, created_at, updated_at;

-- name: ListProductTypes :many
SELECT id, name, name_en, type_code, archived, archived_at, version, created_at, updated_at
FROM product_types
WHERE archived = false
ORDER BY name;

-- name: GetProductType :one
SELECT id, name, name_en, type_code, archived, archived_at, version, created_at, updated_at
FROM product_types
WHERE id = $1;

-- name: CreateProductType :one
INSERT INTO product_types (id, name, name_en, type_code)
VALUES ($1, $2, $3, $4)
RETURNING id, name, name_en, type_code, archived, archived_at, version, created_at, updated_at;

-- name: UpdateProductType :one
UPDATE product_types
SET name = $1,
    name_en = $2,
    type_code = $3,
    updated_at = now(),
    version = version + 1
WHERE id = $4 AND version = $5
RETURNING id, name, name_en, type_code, archived, archived_at, version, created_at, updated_at;

-- name: ArchiveProductType :one
UPDATE product_types
SET archived = true, archived_at = now(), updated_at = now(), version = version + 1
WHERE id = $1 AND version = $2
RETURNING id, name, name_en, type_code, archived, archived_at, version, created_at, updated_at;

-- name: ListProductTypeFields :many
SELECT ptf.id, ptf.product_type_id, ptf.field_definition_id, ptf.sort_order, ptf.mandatory_override, ptf.archived, ptf.archived_at, ptf.version, ptf.created_at, ptf.updated_at,
       fd.name AS field_name, fd.name_en AS field_name_en, fd.field_type, fd.group_name
FROM product_type_fields ptf
JOIN field_definitions fd ON fd.id = ptf.field_definition_id
WHERE ptf.product_type_id = $1 AND ptf.archived = false
ORDER BY ptf.sort_order;

-- name: AssignFieldToProductType :one
INSERT INTO product_type_fields (id, product_type_id, field_definition_id, sort_order, mandatory_override)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, product_type_id, field_definition_id, sort_order, mandatory_override, archived, archived_at, version, created_at, updated_at;

-- name: UpdateProductTypeField :one
UPDATE product_type_fields
SET sort_order = $1,
    mandatory_override = $2,
    updated_at = now(),
    version = version + 1
WHERE id = $3 AND version = $4
RETURNING id, product_type_id, field_definition_id, sort_order, mandatory_override, archived, archived_at, version, created_at, updated_at;

-- name: ArchiveProductTypeField :one
UPDATE product_type_fields
SET archived = true, archived_at = now(), updated_at = now(), version = version + 1
WHERE id = $1 AND version = $2
RETURNING id, product_type_id, field_definition_id, sort_order, mandatory_override, archived, archived_at, version, created_at, updated_at;

-- name: ListProducts :many
SELECT id, name, sku, ean, product_type_id, short_description, wholesale_price_sek, wholesale_currency, msrp_sek, msrp_currency, status, attributes, sku_locked, archived, archived_at, version, created_at, updated_at
FROM products
WHERE archived = false
ORDER BY name;

-- name: SearchProducts :many
SELECT id, name, sku, ean, product_type_id, short_description, wholesale_price_sek, wholesale_currency, msrp_sek, msrp_currency, status, attributes, sku_locked, archived, archived_at, version, created_at, updated_at
FROM products
WHERE archived = false AND (
    name ILIKE $1 OR
    sku ILIKE $1 OR
    ean ILIKE $1
)
ORDER BY name
LIMIT 50;

-- name: GetProduct :one
SELECT id, name, sku, ean, product_type_id, short_description, wholesale_price_sek, wholesale_currency, msrp_sek, msrp_currency, status, attributes, sku_locked, archived, archived_at, version, created_at, updated_at
FROM products
WHERE id = $1;

-- name: GetProductBySKU :one
SELECT id, name, sku, ean, product_type_id, short_description, wholesale_price_sek, wholesale_currency, msrp_sek, msrp_currency, status, attributes, sku_locked, archived, archived_at, version, created_at, updated_at
FROM products
WHERE sku = $1;

-- name: CreateProduct :one
INSERT INTO products (id, name, sku, ean, product_type_id, short_description, wholesale_price_sek, wholesale_currency, msrp_sek, msrp_currency, status, attributes)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING id, name, sku, ean, product_type_id, short_description, wholesale_price_sek, wholesale_currency, msrp_sek, msrp_currency, status, attributes, sku_locked, archived, archived_at, version, created_at, updated_at;

-- name: UpdateProduct :one
UPDATE products
SET name = $1,
    sku = $2,
    ean = $3,
    product_type_id = $4,
    short_description = $5,
    wholesale_price_sek = $6,
    wholesale_currency = $7,
    msrp_sek = $8,
    msrp_currency = $9,
    status = $10,
    attributes = $11,
    updated_at = now(),
    version = version + 1
WHERE id = $12 AND version = $13
RETURNING id, name, sku, ean, product_type_id, short_description, wholesale_price_sek, wholesale_currency, msrp_sek, msrp_currency, status, attributes, sku_locked, archived, archived_at, version, created_at, updated_at;

-- name: ArchiveProduct :one
UPDATE products
SET archived = true, archived_at = now(), updated_at = now(), version = version + 1
WHERE id = $1 AND version = $2
RETURNING id, name, sku, ean, product_type_id, short_description, wholesale_price_sek, wholesale_currency, msrp_sek, msrp_currency, status, attributes, sku_locked, archived, archived_at, version, created_at, updated_at;

-- name: LockSKU :one
UPDATE products
SET sku_locked = true, updated_at = now(), version = version + 1
WHERE id = $1 AND version = $2
RETURNING id, name, sku, ean, product_type_id, short_description, wholesale_price_sek, wholesale_currency, msrp_sek, msrp_currency, status, attributes, sku_locked, archived, archived_at, version, created_at, updated_at;

-- name: ListProductImages :many
SELECT id, product_id, url, sort_order, is_main, archived, archived_at, version, created_at, updated_at
FROM product_images
WHERE product_id = $1 AND archived = false
ORDER BY sort_order, is_main DESC;

-- name: CreateProductImage :one
INSERT INTO product_images (id, product_id, url, sort_order, is_main)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, product_id, url, sort_order, is_main, archived, archived_at, version, created_at, updated_at;

-- name: UpdateProductImage :one
UPDATE product_images
SET url = $1,
    sort_order = $2,
    is_main = $3,
    updated_at = now(),
    version = version + 1
WHERE id = $4 AND version = $5
RETURNING id, product_id, url, sort_order, is_main, archived, archived_at, version, created_at, updated_at;

-- name: ArchiveProductImage :one
UPDATE product_images
SET archived = true, archived_at = now(), updated_at = now(), version = version + 1
WHERE id = $1 AND version = $2
RETURNING id, product_id, url, sort_order, is_main, archived, archived_at, version, created_at, updated_at;

-- name: ListProductDocuments :many
SELECT id, product_id, title, title_en, document_type, url, expires_at, archived, archived_at, version, created_at, updated_at
FROM product_documents
WHERE product_id = $1 AND archived = false
ORDER BY title;

-- name: CreateProductDocument :one
INSERT INTO product_documents (id, product_id, title, title_en, document_type, url, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, product_id, title, title_en, document_type, url, expires_at, archived, archived_at, version, created_at, updated_at;

-- name: UpdateProductDocument :one
UPDATE product_documents
SET title = $1,
    title_en = $2,
    document_type = $3,
    url = $4,
    expires_at = $5,
    updated_at = now(),
    version = version + 1
WHERE id = $6 AND version = $7
RETURNING id, product_id, title, title_en, document_type, url, expires_at, archived, archived_at, version, created_at, updated_at;

-- name: ArchiveProductDocument :one
UPDATE product_documents
SET archived = true, archived_at = now(), updated_at = now(), version = version + 1
WHERE id = $1 AND version = $2
RETURNING id, product_id, title, title_en, document_type, url, expires_at, archived, archived_at, version, created_at, updated_at;

-- name: ListIdsEntries :many
SELECT id, category, name, value, issuer, valid_from, valid_to, owner, note, document_url, archived, archived_at, version, created_at, updated_at
FROM ids_entries
WHERE archived = false
ORDER BY category, name;

-- name: SearchIdsEntries :many
SELECT id, category, name, value, issuer, valid_from, valid_to, owner, note, document_url, archived, archived_at, version, created_at, updated_at
FROM ids_entries
WHERE archived = false AND (
    name ILIKE $1 OR
    value ILIKE $1 OR
    category ILIKE $1
)
ORDER BY category, name
LIMIT 50;

-- name: GetIdsEntry :one
SELECT id, category, name, value, issuer, valid_from, valid_to, owner, note, document_url, archived, archived_at, version, created_at, updated_at
FROM ids_entries
WHERE id = $1;

-- name: CreateIdsEntry :one
INSERT INTO ids_entries (id, category, name, value, issuer, valid_from, valid_to, owner, note, document_url)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id, category, name, value, issuer, valid_from, valid_to, owner, note, document_url, archived, archived_at, version, created_at, updated_at;

-- name: UpdateIdsEntry :one
UPDATE ids_entries
SET category = $1,
    name = $2,
    value = $3,
    issuer = $4,
    valid_from = $5,
    valid_to = $6,
    owner = $7,
    note = $8,
    document_url = $9,
    updated_at = now(),
    version = version + 1
WHERE id = $10 AND version = $11
RETURNING id, category, name, value, issuer, valid_from, valid_to, owner, note, document_url, archived, archived_at, version, created_at, updated_at;

-- name: ArchiveIdsEntry :one
UPDATE ids_entries
SET archived = true, archived_at = now(), updated_at = now(), version = version + 1
WHERE id = $1 AND version = $2
RETURNING id, category, name, value, issuer, valid_from, valid_to, owner, note, document_url, archived, archived_at, version, created_at, updated_at;
