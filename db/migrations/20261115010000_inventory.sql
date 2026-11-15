-- +goose Up
-- +goose StatementBegin
-- LAG-01: locations for inventory (LAG-01)
CREATE TABLE locations (
    id                  uuid PRIMARY KEY DEFAULT uuidv7(),
    name                text NOT NULL,
    name_en             text NOT NULL DEFAULT '',
    address             text NOT NULL DEFAULT '',
    city                text NOT NULL DEFAULT '',
    country             text NOT NULL DEFAULT '',
    channel_buffer      int NOT NULL DEFAULT 0,
    archived            boolean NOT NULL DEFAULT false,
    archived_at         timestamptz,
    version             int NOT NULL DEFAULT 1,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
-- LAG-01, LAG-02, LAG-03, LAG-04: stock movements are the source of truth for balance
CREATE TABLE stockmovements (
    id                      uuid PRIMARY KEY DEFAULT uuidv7(),
    product_id              uuid NOT NULL REFERENCES products(id),
    location_id             uuid NOT NULL REFERENCES locations(id),
    batch_number            text NOT NULL DEFAULT '',
    manufacturing_date      date,
    best_before_date        date,
    movement_type           text NOT NULL CHECK (movement_type IN ('purchase', 'sale', 'adjustment_in', 'adjustment_out', 'transfer_in', 'transfer_out', 'reservation', 'release')),
    quantity                int NOT NULL,
    reason                  text NOT NULL DEFAULT '',
    reference_type          text NOT NULL DEFAULT '',
    reference_id            uuid,
    version                 int NOT NULL DEFAULT 1,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_stockmovements_product ON stockmovements(product_id);
CREATE INDEX idx_stockmovements_location ON stockmovements(location_id);
CREATE INDEX idx_stockmovements_type ON stockmovements(movement_type);
CREATE INDEX idx_stockmovements_batch ON stockmovements(batch_number) WHERE batch_number != '';
CREATE INDEX idx_stockmovements_ref ON stockmovements(reference_type, reference_id) WHERE reference_id IS NOT NULL;
-- +goose StatementEnd

-- +goose StatementBegin
GRANT SELECT, INSERT, UPDATE ON locations TO linochi_erp_app;
GRANT SELECT, INSERT, UPDATE ON stockmovements TO linochi_erp_app;
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS stockmovements;
DROP TABLE IF EXISTS locations;
-- +goose StatementEnd
