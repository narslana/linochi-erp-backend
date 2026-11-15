# Agent Report: linochi-erp-backend

## Status: WIP - Module 4 (Inventory) in progress

### Completed
- Migration: `20261115010000_inventory.sql` (locations, stockmovements tables)
- sqlc queries: `db/queries/inventory.sql` (ListLocations, GetLocation, CreateLocation, UpdateLocation, ArchiveLocation, ListStockMovements, CreateStockMovement, BalanceByLocation, AvailableBalance, MovementsForFEFO, InventoryValueByLocation)
- Domain: `internal/inventory/domain.go` (Location, StockMovement, Balance, InventoryValue, FEFOBatch types and errors)
- Store: `internal/inventory/store.go` (sqlc wrappers)
- Service: `internal/inventory/service.go` (business logic: CRUD locations, stock movements, balance, FEFO, inventory value, reserve/release/withdraw stock)
- Handler: `internal/inventory/handler.go` (HTTP handlers for all inventory endpoints)
- OpenAPI: Added inventory tag, paths (`/api/v1/locations`, `/api/v1/stockmovements`, `/api/v1/inventory/*`), and schemas (Location, StockMovement, Balance, FEFOBatch, InventoryValue)
- Wiring: Updated `internal/boards/composite.go` and `cmd/api/run.go` to include inventory handler

### Active
- Backend `make check` needs to be run and fixed (pre-existing migration issue in `20261114200158_products_fields_product_types_documents_ids.sql`)
- Frontend: needs `src/features/inventory/` views, `router.ts` update, i18n keys, and `make check` in web
- AGENT_REPORT.md files need creation/update and final commit

### Blocked
- Pre-existing migration file `20261114200158_products_fields_product_types_documents_ids.sql` missing `-- +goose Up` annotation (causes integration test failures)

### Next Steps
1. Run `make check` on backend and fix any remaining issues
2. Build frontend inventory view and update router
3. Update i18n keys and run `make check` on web
4. Update both AGENT_REPORT.md files, commit, and push
