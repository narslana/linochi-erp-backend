# Agent report: LAG-01..LAG-05, LAG-07 – inventory ledger and balances (T1)

Branch: agent/t1-inventory
Model: claude-sonnet-5-5 (run by Claude Code, not the local agent)

## Plan
1. Failing integration tests first (`internal/inventory/ledger_integration_test.go`), committed and pushed.
2. Migration `20261118100000_inventory_ledger.sql`: append-only ledger, sign check, `receipt`/`return` types, `products.manufacturing_cost`, `products.channel_buffer`, drop `locations.channel_buffer`, `order_line_batches`.
3. sqlc queries, domain/service/store/handler, OpenAPI, `make gen`.

## Done
- Ledger is append-only: UPDATE revoked from the app role (DELETE was never granted). DB CHECK enforces sign by type, so a bypass of the service is refused too.
- Sign decided in one place (`SignedQuantity`); client sends a magnitude. Zero and negative refused (422).
- in_stock / reserved / available computed from the ledger; `GetBalance` also returns `marketplace_quantity`.
- `Reserve`, `Release`, `Ship` take a `*db.Tx` and lock the product (advisory xact lock). Refusal is a 409 `insufficient-stock` problem; nothing is written.
- `Ship` writes one `release` + one `sale` per batch, FEFO or manual batch, records batches in `order_line_batches` and bumps `order_lines.quantity_shipped`.
- Manual adjustments need a non-blank reason (422). `Transfer` writes a paired out/in in one transaction.
- `InventoryValue` uses `products.manufacturing_cost`, no supplier join.
- Metrics `linochi_inventory_movements_total{type}` and `linochi_inventory_refusals_total{reason}`; every write is audited.
- OpenAPI: Balance fields, movement types, 409/422 responses, withdraw `order_line_id`/`batch_number`/`batches`; `channel_buffer` removed from Location.

## Tests
- `go test -tags integration ./internal/inventory/`: 14 new tests, all green (the 12 listed in the brief plus transfer pairing and FEFO/manual split).
- `make check` (lint, unit, integration): green.

## Not done / skipped
- `manufacturing_cost` and `channel_buffer` are not yet settable through the products API (products service/OpenAPI untouched). Tests set them by SQL. Needs a small follow-up.
- No HTTP endpoint for `Transfer` (service only).
- No structured slog lines in the service; audit entries and the two metrics cover it.
- Not touched: web, workflow (T2).

## Assumptions
- Brief test 9 says `sale` with +5 is refused and `sale` with 5 works, which are the same integer. Implemented as: quantity is a magnitude, negative or zero is refused (`SignedQuantity`, domain.go). The test uses -5 for the refused case.
- `Reserve/Release/Ship` accept `*db.Tx` (the project's wrapper around pgx.Tx), not a raw `pgx.Tx`, because the audit recorder requires it.
- Inventory value is returned in öre: quantity x `manufacturing_cost` (SEK numeric) x 100, rounded.
- `marketplace_quantity` is product-level (sum of available over locations minus buffer) and repeated on each location row.
- Generic `CreateStockMovement` refuses `reservation`, `release`, `transfer_in`, `transfer_out` (own entry points) and checks available/batch for outgoing types.
- `purchase` renamed to `receipt` in existing rows; existing rows with a wrong sign were normalised in the migration (rows with quantity 0 deleted).
- Location buffer values were not copied to products (dropped).

## Open questions
- Should the product API expose `manufacturing_cost` and `channel_buffer` in this task or a follow-up?

## Where the reviewer should look first
- `db/migrations/20261118100000_inventory_ledger.sql` (data normalisation, Down).
- `internal/inventory/service.go`: `Ship`, `pickBatches`, `checkWithdrawal`.
- Changes outside the brief's files, all mechanical: `internal/audit/t3_integration_test.go` (CreateLocation/UpdateLocation lost the buffer argument), `internal/platform/auth/contract_integration_test.go` (`purchase` -> `receipt`), `cmd/api/migrate_integration_test.go` (hard-coded "more than 10 down steps" limit raised to 50 because there are now 13 migrations), `db/queries/products.sql` (explicit column lists extended with the two new columns so row types still match).
