-- +goose Up
-- +goose StatementBegin
CREATE TABLE field_definitions (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name            text NOT NULL,
    name_en         text NOT NULL DEFAULT '',
    field_type      text NOT NULL CHECK (field_type IN (
        'short_text', 'long_text', 'text_per_language', 'integer',
        'decimal', 'amount', 'date', 'yes_no', 'choice',
        'multiple_choice', 'link', 'document'
    )),
    group_name      text NOT NULL DEFAULT '',
    help_text       text NOT NULL DEFAULT '',
    mandatory       boolean NOT NULL DEFAULT false,
    choices         jsonb NOT NULL DEFAULT '[]',
    unit            text NOT NULL DEFAULT '',
    show_on_card    boolean NOT NULL DEFAULT false,
    archived        boolean NOT NULL DEFAULT false,
    archived_at     timestamptz,
    version         int NOT NULL DEFAULT 1,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE product_types (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name            text NOT NULL,
    name_en         text NOT NULL DEFAULT '',
    type_code       text NOT NULL,
    archived        boolean NOT NULL DEFAULT false,
    archived_at     timestamptz,
    version         int NOT NULL DEFAULT 1,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE product_type_fields (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    product_type_id     uuid NOT NULL REFERENCES product_types(id),
    field_definition_id uuid NOT NULL REFERENCES field_definitions(id),
    sort_order          int NOT NULL DEFAULT 0,
    mandatory_override  boolean,
    archived            boolean NOT NULL DEFAULT false,
    archived_at         timestamptz,
    version             int NOT NULL DEFAULT 1,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE products (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name                    text NOT NULL,
    sku                     text NOT NULL,
    ean                     text,
    product_type_id         uuid NOT NULL REFERENCES product_types(id),
    short_description       text NOT NULL DEFAULT '',
    wholesale_price_sek     bigint NOT NULL DEFAULT 0,
    wholesale_currency      char(3) NOT NULL DEFAULT 'SEK',
    msrp_sek                bigint NOT NULL DEFAULT 0,
    msrp_currency           char(3) NOT NULL DEFAULT 'SEK',
    status                  text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'discontinuing', 'archived')),
    attributes              jsonb NOT NULL DEFAULT '{}',
    sku_locked              boolean NOT NULL DEFAULT false,
    archived                boolean NOT NULL DEFAULT false,
    archived_at             timestamptz,
    version                 int NOT NULL DEFAULT 1,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE UNIQUE INDEX idx_unique_locked_sku ON products(sku) WHERE archived = false AND sku_locked = true;
CREATE INDEX idx_products_product_type ON products(product_type_id) WHERE archived = false;
CREATE INDEX idx_products_status ON products(status) WHERE archived = false;
CREATE INDEX idx_products_attributes ON products USING gin(attributes);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE product_images (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id      uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    url             text NOT NULL,
    sort_order      int NOT NULL DEFAULT 0,
    is_main         boolean NOT NULL DEFAULT false,
    archived        boolean NOT NULL DEFAULT false,
    archived_at     timestamptz,
    version         int NOT NULL DEFAULT 1,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE product_documents (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id      uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    title           text NOT NULL,
    title_en        text NOT NULL DEFAULT '',
    document_type   text NOT NULL,
    url             text NOT NULL,
    expires_at      timestamptz,
    archived        boolean NOT NULL DEFAULT false,
    archived_at     timestamptz,
    version         int NOT NULL DEFAULT 1,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE ids_entries (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    category        text NOT NULL,
    name            text NOT NULL,
    value           text NOT NULL,
    issuer          text NOT NULL DEFAULT '',
    valid_from      date,
    valid_to        date,
    owner           text NOT NULL DEFAULT '',
    note            text NOT NULL DEFAULT '',
    document_url    text,
    archived        boolean NOT NULL DEFAULT false,
    archived_at     timestamptz,
    version         int NOT NULL DEFAULT 1,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_ids_entries_category ON ids_entries(category) WHERE archived = false;
CREATE INDEX idx_ids_entries_value ON ids_entries(value) WHERE archived = false;
CREATE INDEX idx_ids_entries_name ON ids_entries(name) WHERE archived = false;
-- +goose StatementEnd

-- +goose StatementBegin
GRANT SELECT, INSERT, UPDATE ON field_definitions TO linochi_erp_app;
GRANT SELECT, INSERT, UPDATE ON product_types TO linochi_erp_app;
GRANT SELECT, INSERT, UPDATE ON product_type_fields TO linochi_erp_app;
GRANT SELECT, INSERT, UPDATE ON products TO linochi_erp_app;
GRANT SELECT, INSERT, UPDATE ON product_images TO linochi_erp_app;
GRANT SELECT, INSERT, UPDATE ON product_documents TO linochi_erp_app;
GRANT SELECT, INSERT, UPDATE ON ids_entries TO linochi_erp_app;
-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS ids_entries;
DROP TABLE IF EXISTS product_documents;
DROP TABLE IF EXISTS product_images;
DROP TABLE IF EXISTS products;
DROP TABLE IF EXISTS product_type_fields;
DROP TABLE IF EXISTS product_types;
DROP TABLE IF EXISTS field_definitions;
-- +goose StatementEnd
