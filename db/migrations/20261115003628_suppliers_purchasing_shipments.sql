-- +goose Up
-- +goose StatementBegin
CREATE TABLE suppliers (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name                text NOT NULL,
    name_en             text NOT NULL DEFAULT '',
    org_number          text NOT NULL DEFAULT '',
    vat_number          text NOT NULL DEFAULT '',
    country             text NOT NULL DEFAULT '',
    address             text NOT NULL DEFAULT '',
    city                text NOT NULL DEFAULT '',
    postal_code         text NOT NULL DEFAULT '',
    website             text NOT NULL DEFAULT '',
    contact_email       text NOT NULL DEFAULT '',
    contact_phone       text NOT NULL DEFAULT '',
    role                text NOT NULL DEFAULT '' CHECK (role IN ('component', 'manufacturer', 'freight')),
    lead_time_days      int NOT NULL DEFAULT 0,
    moq                 int NOT NULL DEFAULT 0,
    currency            char(3) NOT NULL DEFAULT 'SEK',
    payment_terms       text NOT NULL DEFAULT '',
    delivery_terms      text NOT NULL DEFAULT '',
    notes               text NOT NULL DEFAULT '',
    archived            boolean NOT NULL DEFAULT false,
    archived_at         timestamptz,
    version             int NOT NULL DEFAULT 1,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE supplier_contacts (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    supplier_id     uuid NOT NULL REFERENCES suppliers(id),
    name            text NOT NULL,
    name_en         text NOT NULL DEFAULT '',
    email           text NOT NULL DEFAULT '',
    phone           text NOT NULL DEFAULT '',
    role            text NOT NULL DEFAULT '',
    archived        boolean NOT NULL DEFAULT false,
    archived_at     timestamptz,
    version         int NOT NULL DEFAULT 1,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE supplier_products (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    supplier_id         uuid NOT NULL REFERENCES suppliers(id),
    product_id          uuid NOT NULL REFERENCES products(id),
    purchase_price      bigint NOT NULL DEFAULT 0,
    purchase_currency   char(3) NOT NULL DEFAULT 'SEK',
    archived            boolean NOT NULL DEFAULT false,
    archived_at         timestamptz,
    version             int NOT NULL DEFAULT 1,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE purchase_orders (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    supplier_id         uuid NOT NULL REFERENCES suppliers(id),
    order_number        text NOT NULL DEFAULT '',
    currency            char(3) NOT NULL DEFAULT 'SEK',
    order_date          date NOT NULL DEFAULT now(),
    expected_date       date,
    status              text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'ordered', 'paid', 'done', 'cancelled')),
    total_amount        bigint NOT NULL DEFAULT 0,
    notes               text NOT NULL DEFAULT '',
    archived            boolean NOT NULL DEFAULT false,
    archived_at         timestamptz,
    version             int NOT NULL DEFAULT 1,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE purchase_order_lines (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    purchase_order_id       uuid NOT NULL REFERENCES purchase_orders(id) ON DELETE CASCADE,
    product_id              uuid NOT NULL REFERENCES products(id),
    quantity                int NOT NULL,
    unit_price              bigint NOT NULL DEFAULT 0,
    unit_currency           char(3) NOT NULL DEFAULT 'SEK',
    line_total              bigint NOT NULL DEFAULT 0,
    archived                boolean NOT NULL DEFAULT false,
    archived_at             timestamptz,
    version                 int NOT NULL DEFAULT 1,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE shipments (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    purchase_order_id       uuid NOT NULL REFERENCES purchase_orders(id),
    from_location           text NOT NULL DEFAULT '',
    to_location             text NOT NULL DEFAULT '',
    carrier                 text NOT NULL DEFAULT '',
    tracking_number         text NOT NULL DEFAULT '',
    shipped_at              timestamptz,
    estimated_arrival       date,
    actual_arrival          date,
    batch_number            text NOT NULL DEFAULT '',
    status                  text NOT NULL DEFAULT 'planned' CHECK (status IN ('planned', 'shipped', 'in_transit', 'arrived', 'delayed')),
    documents               jsonb NOT NULL DEFAULT '[]',
    archived                boolean NOT NULL DEFAULT false,
    archived_at             timestamptz,
    version                 int NOT NULL DEFAULT 1,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_supplier_contacts_supplier ON supplier_contacts(supplier_id) WHERE archived = false;
CREATE INDEX idx_supplier_products_supplier ON supplier_products(supplier_id) WHERE archived = false;
CREATE INDEX idx_supplier_products_product ON supplier_products(product_id) WHERE archived = false;
CREATE INDEX idx_purchase_orders_supplier ON purchase_orders(supplier_id) WHERE archived = false;
CREATE INDEX idx_purchase_orders_status ON purchase_orders(status) WHERE archived = false;
CREATE INDEX idx_purchase_order_lines_po ON purchase_order_lines(purchase_order_id) WHERE archived = false;
CREATE INDEX idx_shipments_po ON shipments(purchase_order_id) WHERE archived = false;
CREATE INDEX idx_shipments_status ON shipments(status) WHERE archived = false;
-- +goose StatementEnd

-- +goose StatementBegin
GRANT SELECT, INSERT, UPDATE ON suppliers TO linochi_erp_app;
GRANT SELECT, INSERT, UPDATE ON supplier_contacts TO linochi_erp_app;
GRANT SELECT, INSERT, UPDATE ON supplier_products TO linochi_erp_app;
GRANT SELECT, INSERT, UPDATE ON purchase_orders TO linochi_erp_app;
GRANT SELECT, INSERT, UPDATE ON purchase_order_lines TO linochi_erp_app;
GRANT SELECT, INSERT, UPDATE ON shipments TO linochi_erp_app;
-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS shipments;
DROP TABLE IF EXISTS purchase_order_lines;
DROP TABLE IF EXISTS purchase_orders;
DROP TABLE IF EXISTS supplier_products;
DROP TABLE IF EXISTS supplier_contacts;
DROP TABLE IF EXISTS suppliers;
-- +goose StatementEnd
