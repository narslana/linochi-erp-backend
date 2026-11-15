-- +goose Up
-- +goose StatementBegin
-- KUN-01, KUN-04, KUN-05, KUN-07, KUN-08: customers
CREATE TABLE customers (
    id              uuid PRIMARY KEY DEFAULT uuidv7(),
    name            text NOT NULL,
    type_code       text NOT NULL,
    address         text NOT NULL DEFAULT '',
    city            text NOT NULL DEFAULT '',
    country         text NOT NULL DEFAULT '',
    eori            text NOT NULL DEFAULT '',
    epr             text NOT NULL DEFAULT '',
    currency        char(3) NOT NULL DEFAULT 'SEK',
    notes           text NOT NULL DEFAULT '',
    archived        boolean NOT NULL DEFAULT false,
    archived_at     timestamptz,
    version         int NOT NULL DEFAULT 1,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_customers_type ON customers(type_code) WHERE archived = false;

-- KUN-01, KUN-04, KUN-07: contacts
CREATE TABLE contacts (
    id              uuid PRIMARY KEY DEFAULT uuidv7(),
    customer_id     uuid NOT NULL REFERENCES customers(id),
    first_name      text NOT NULL DEFAULT '',
    last_name       text NOT NULL DEFAULT '',
    email           text NOT NULL DEFAULT '',
    phone           text NOT NULL DEFAULT '',
    archived        boolean NOT NULL DEFAULT false,
    archived_at     timestamptz,
    version         int NOT NULL DEFAULT 1,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_contacts_customer ON contacts(customer_id) WHERE archived = false;

-- KUN-06: leads
CREATE TABLE leads (
    id                  uuid PRIMARY KEY DEFAULT uuidv7(),
    company             text NOT NULL,
    contact_name        text NOT NULL DEFAULT '',
    email               text NOT NULL DEFAULT '',
    phone               text NOT NULL DEFAULT '',
    address             text NOT NULL DEFAULT '',
    city                text NOT NULL DEFAULT '',
    country             text NOT NULL DEFAULT '',
    status              text NOT NULL DEFAULT 'new' CHECK (status IN ('new', 'contacted', 'interested', 'customer', 'not_relevant')),
    notes               text NOT NULL DEFAULT '',
    converted_customer_id uuid,
    archived            boolean NOT NULL DEFAULT false,
    archived_at         timestamptz,
    version             int NOT NULL DEFAULT 1,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_leads_status ON leads(status) WHERE archived = false;

-- REG-04: EAN series
CREATE TABLE ean_series (
    id              uuid PRIMARY KEY DEFAULT uuidv7(),
    prefix          text NOT NULL,
    start_code      text NOT NULL,
    end_code        text NOT NULL,
    description     text NOT NULL DEFAULT '',
    assigned_count  int NOT NULL DEFAULT 0,
    archived        boolean NOT NULL DEFAULT false,
    archived_at     timestamptz,
    version         int NOT NULL DEFAULT 1,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_eanseries_prefix ON ean_series(prefix) WHERE archived = false;

-- ORD: grant permissions
GRANT SELECT, INSERT, UPDATE ON customers TO linochi_erp_app;
GRANT SELECT, INSERT, UPDATE ON contacts TO linochi_erp_app;
GRANT SELECT, INSERT, UPDATE ON leads TO linochi_erp_app;
GRANT SELECT, INSERT, UPDATE ON ean_series TO linochi_erp_app;
-- +goose StatementEnd

-- +goose StatementBegin
-- WF-01, WF-03: workflow steps
CREATE TABLE workflow_steps (
    id              uuid PRIMARY KEY DEFAULT uuidv7(),
    key             text NOT NULL,
    name            text NOT NULL,
    name_en         text NOT NULL DEFAULT '',
    sort_order      int NOT NULL DEFAULT 0,
    is_start        boolean NOT NULL DEFAULT false,
    is_end          boolean NOT NULL DEFAULT false,
    archived        boolean NOT NULL DEFAULT false,
    archived_at     timestamptz,
    version         int NOT NULL DEFAULT 1,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_workflowsteps_key ON workflow_steps(key) WHERE archived = false;

-- WF-02: workflow transitions
CREATE TABLE workflow_transitions (
    id              uuid PRIMARY KEY DEFAULT uuidv7(),
    step_key        text NOT NULL,
    from_key        text NOT NULL,
    to_key          text NOT NULL,
    label           text NOT NULL,
    label_en        text NOT NULL DEFAULT '',
    sort_order      int NOT NULL DEFAULT 0,
    archived        boolean NOT NULL DEFAULT false,
    archived_at     timestamptz,
    version         int NOT NULL DEFAULT 1,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_workflowtransitions_step ON workflow_transitions(step_key) WHERE archived = false;

-- WF-04: workflow conditions
CREATE TABLE workflow_conditions (
    id              uuid PRIMARY KEY DEFAULT uuidv7(),
    transition_id   uuid NOT NULL REFERENCES workflow_transitions(id),
    kind            text NOT NULL,
    config          jsonb NOT NULL DEFAULT '{}',
    archived        boolean NOT NULL DEFAULT false,
    archived_at     timestamptz,
    version         int NOT NULL DEFAULT 1,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

-- WF-05: workflow effects
CREATE TABLE workflow_effects (
    id              uuid PRIMARY KEY DEFAULT uuidv7(),
    transition_id   uuid NOT NULL REFERENCES workflow_transitions(id),
    kind            text NOT NULL,
    config          jsonb NOT NULL DEFAULT '{}',
    archived        boolean NOT NULL DEFAULT false,
    archived_at     timestamptz,
    version         int NOT NULL DEFAULT 1,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

-- WF: grant permissions
GRANT SELECT, INSERT, UPDATE ON workflow_steps TO linochi_erp_app;
GRANT SELECT, INSERT, UPDATE ON workflow_transitions TO linochi_erp_app;
GRANT SELECT, INSERT, UPDATE ON workflow_conditions TO linochi_erp_app;
GRANT SELECT, INSERT, UPDATE ON workflow_effects TO linochi_erp_app;
-- +goose StatementEnd

-- +goose StatementBegin
-- ORD-01 to ORD-08, ORD-09 to ORD-13: orders
CREATE TABLE orders (
    id                  uuid PRIMARY KEY DEFAULT uuidv7(),
    external_id         text NOT NULL DEFAULT '',
    channel             text NOT NULL,
    status              text NOT NULL DEFAULT 'draft',
    currency            char(3) NOT NULL DEFAULT 'SEK',
    exchange_rate_sek   numeric(10, 6) NOT NULL DEFAULT 1,
    subtotal_sek        bigint NOT NULL DEFAULT 0,
    tax_sek             bigint NOT NULL DEFAULT 0,
    freight_sek         bigint NOT NULL DEFAULT 0,
    commission_sek      bigint NOT NULL DEFAULT 0,
    total_sek           bigint NOT NULL DEFAULT 0,
    customer_id         uuid NOT NULL REFERENCES customers(id),
    shipping_name       text NOT NULL DEFAULT '',
    shipping_address    text NOT NULL DEFAULT '',
    shipping_city       text NOT NULL DEFAULT '',
    shipping_postal_code text NOT NULL DEFAULT '',
    shipping_country    text NOT NULL DEFAULT '',
    billing_name        text NOT NULL DEFAULT '',
    billing_address     text NOT NULL DEFAULT '',
    billing_city        text NOT NULL DEFAULT '',
    billing_postal_code text NOT NULL DEFAULT '',
    billing_country     text NOT NULL DEFAULT '',
    tracking_number     text NOT NULL DEFAULT '',
    carrier             text NOT NULL DEFAULT '',
    shipping_label_url  text NOT NULL DEFAULT '',
    delivered_at        timestamptz,
    shipped_at          timestamptz,
    notes               text NOT NULL DEFAULT '',
    archived            boolean NOT NULL DEFAULT false,
    archived_at         timestamptz,
    version             int NOT NULL DEFAULT 1,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_orders_external_id ON orders(external_id) WHERE external_id != '' AND archived = false;
CREATE INDEX idx_orders_channel ON orders(channel) WHERE archived = false;
CREATE INDEX idx_orders_status ON orders(status) WHERE archived = false;
CREATE INDEX idx_orders_customer ON orders(customer_id) WHERE archived = false;

-- ORD-01, ORD-10: order lines
CREATE TABLE order_lines (
    id                  uuid PRIMARY KEY DEFAULT uuidv7(),
    order_id            uuid NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    product_id          uuid NOT NULL REFERENCES products(id),
    quantity            int NOT NULL,
    quantity_shipped    int NOT NULL DEFAULT 0,
    unit_price_sek      bigint NOT NULL,
    tax_rate            numeric(5, 2) NOT NULL DEFAULT 25.00,
    tax_amount_sek      bigint NOT NULL DEFAULT 0,
    total_sek           bigint NOT NULL DEFAULT 0,
    archived            boolean NOT NULL DEFAULT false,
    archived_at         timestamptz,
    version             int NOT NULL DEFAULT 1,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_orderlines_order ON order_lines(order_id) WHERE archived = false;
CREATE INDEX idx_orderlines_product ON order_lines(product_id) WHERE archived = false;

-- ORD: grant permissions
GRANT SELECT, INSERT, UPDATE ON orders TO linochi_erp_app;
GRANT SELECT, INSERT, UPDATE ON order_lines TO linochi_erp_app;
-- +goose StatementEnd

-- +goose StatementBegin
-- REG-05: reminders
CREATE TABLE reminders (
    id                  uuid PRIMARY KEY DEFAULT uuidv7(),
    reminder_type       text NOT NULL CHECK (reminder_type IN ('ids_entry', 'external')),
    ids_entry_id        uuid REFERENCES ids_entries(id),
    external_ref_type   text,
    external_ref_id     uuid,
    title               text NOT NULL,
    title_en            text NOT NULL DEFAULT '',
    description         text NOT NULL DEFAULT '',
    due_date            date NOT NULL,
    notified            boolean NOT NULL DEFAULT false,
    notified_at         timestamptz,
    calendar_event_id   uuid REFERENCES calendar_events(id),
    archived            boolean NOT NULL DEFAULT false,
    archived_at         timestamptz,
    version             int NOT NULL DEFAULT 1,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_reminders_due ON reminders(due_date, notified) WHERE archived = false;

-- REG: grant permissions
GRANT SELECT, INSERT, UPDATE ON reminders TO linochi_erp_app;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS reminders;
DROP TABLE IF EXISTS order_lines;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS workflow_effects;
DROP TABLE IF EXISTS workflow_conditions;
DROP TABLE IF EXISTS workflow_transitions;
DROP TABLE IF EXISTS workflow_steps;
DROP TABLE IF EXISTS ean_series;
DROP TABLE IF EXISTS leads;
DROP TABLE IF EXISTS contacts;
DROP TABLE IF EXISTS customers;
-- +goose StatementEnd
