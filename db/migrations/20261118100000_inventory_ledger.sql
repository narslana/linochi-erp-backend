-- +goose Up

-- LAG-01, LAG-02, LAG-03, LAG-05, LAG-07: the stock ledger becomes append-only,
-- the sign of a movement follows its type, and cost and channel buffer live on
-- the product (owner decisions 2026-11-16).

-- +goose StatementBegin
-- LAG-05: manufacturing cost in SEK (nullable: unknown cost counts as 0 in the value).
-- LAG-07: ONE channel buffer per product, the same for every channel.
ALTER TABLE products
    ADD COLUMN manufacturing_cost numeric(12, 2) CHECK (manufacturing_cost >= 0),
    ADD COLUMN channel_buffer     int NOT NULL DEFAULT 0 CHECK (channel_buffer >= 0);
ALTER TABLE locations DROP COLUMN channel_buffer;
-- +goose StatementEnd

-- +goose StatementBegin
-- 'purchase' is now 'receipt'; 'return' is new. Existing rows are normalised so
-- that the sign check below holds (still the owner role: UPDATE is revoked at the end).
ALTER TABLE stockmovements DROP CONSTRAINT stockmovements_movement_type_check;
UPDATE stockmovements SET movement_type = 'receipt' WHERE movement_type = 'purchase';
UPDATE stockmovements
SET quantity = CASE
        WHEN movement_type IN ('receipt', 'adjustment_in', 'transfer_in', 'release', 'return') THEN abs(quantity)
        ELSE -abs(quantity)
    END
WHERE quantity = 0 OR (quantity > 0) <> (movement_type IN ('receipt', 'adjustment_in', 'transfer_in', 'release', 'return'));
DELETE FROM stockmovements WHERE quantity = 0;
ALTER TABLE stockmovements
    ADD CONSTRAINT stockmovements_movement_type_check CHECK (movement_type IN
        ('receipt', 'return', 'sale', 'adjustment_in', 'adjustment_out', 'transfer_in', 'transfer_out', 'reservation', 'release')),
    -- The sign is decided by the type. The service does it in one place; this
    -- makes the database refuse anything that bypasses it.
    ADD CONSTRAINT stockmovements_sign_check CHECK (
        (movement_type IN ('receipt', 'adjustment_in', 'transfer_in', 'release', 'return') AND quantity > 0)
        OR (movement_type IN ('sale', 'adjustment_out', 'transfer_out', 'reservation') AND quantity < 0)
    );
CREATE INDEX idx_stockmovements_product_location ON stockmovements(product_id, location_id);
-- +goose StatementEnd

-- +goose StatementBegin
-- LAG-01: append-only. Corrections are new movements.
REVOKE UPDATE ON stockmovements FROM linochi_erp_app;
-- +goose StatementEnd

-- +goose StatementBegin
-- LAG-03: which batch was shipped on an order line, for recall traceability. One
-- row per batch, since FEFO may take a line from several batches. Insert-only.
CREATE TABLE order_line_batches (
    id              uuid PRIMARY KEY DEFAULT uuidv7(),
    order_line_id   uuid NOT NULL REFERENCES order_lines(id),
    batch_number    text NOT NULL,
    quantity        int NOT NULL CHECK (quantity > 0),
    movement_id     uuid NOT NULL REFERENCES stockmovements(id),
    version         int NOT NULL DEFAULT 1,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_order_line_batches_line ON order_line_batches(order_line_id);
CREATE INDEX idx_order_line_batches_batch ON order_line_batches(batch_number);
GRANT SELECT, INSERT ON order_line_batches TO linochi_erp_app;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS order_line_batches;
GRANT UPDATE ON stockmovements TO linochi_erp_app;
DROP INDEX IF EXISTS idx_stockmovements_product_location;
ALTER TABLE stockmovements DROP CONSTRAINT stockmovements_sign_check;
ALTER TABLE stockmovements DROP CONSTRAINT stockmovements_movement_type_check;
UPDATE stockmovements SET movement_type = 'receipt' WHERE movement_type = 'return';
UPDATE stockmovements SET movement_type = 'purchase' WHERE movement_type = 'receipt';
ALTER TABLE stockmovements ADD CONSTRAINT stockmovements_movement_type_check CHECK (movement_type IN
    ('purchase', 'sale', 'adjustment_in', 'adjustment_out', 'transfer_in', 'transfer_out', 'reservation', 'release'));
ALTER TABLE locations ADD COLUMN channel_buffer int NOT NULL DEFAULT 0;
ALTER TABLE products DROP COLUMN channel_buffer, DROP COLUMN manufacturing_cost;
-- +goose StatementEnd
