# Agent report: WF-01..WF-07, WF-10, ORD-03, ORD-04, ORD-06, ORD-07 – order workflow engine (T2)

Branch: agent/t2-workflow
Model: claude-sonnet-5-5 (run by Claude Code, not the local agent)

## Plan
1. Failing integration tests first: `internal/workflow/engine_integration_test.go` and `internal/orders/accept_integration_test.go`. Committed and pushed before any implementation.
2. Migration `20261118200000_order_workflow.sql`: drop the old unversioned workflow tables (steps, transitions, conditions, effects: free-text, no data worth keeping), create `workflows`, `workflow_versions`, `workflow_steps`, `workflow_transitions`, `order_status_history`; add `orders.workflow_version_id`; seed the default flow (12 steps) as version 1; map old order statuses.
3. sqlc queries; `internal/workflow`: domain (typed errors), store, engine (`Transition`, `ChangeStatus`, `Preview`, `PublishVersion` with WF-06 validation, `History`, `TimeInSteps`).
4. `internal/orders`: accept, ship, cancel and generic status change call the engine. The free-text `body.Status` path is removed.
5. OpenAPI: remove the old step/transition/condition/effect CRUD, add versions, preview and history endpoints, new bodies for accept/ship/cancel/status. `make gen`.
6. `make check`, `make test-int`.

## Done
- Migration `20261118200000_order_workflow.sql`: `workflows`, `workflow_versions`, `workflow_steps`, `workflow_transitions` (step ids, composite FKs keep both ends in the same version), `order_status_history` (insert-only), `orders.workflow_version_id` (column default = active version of the default flow), `orders.status` default `new`. Default flow seeded as version 1: the 10 steps plus `rejected` and `cancelled`, transitions and effects as in the brief. Old unversioned workflow tables dropped (they held free text; Down recreates them empty).
- `internal/workflow`: `Transition` (in the caller's tx), `ChangeStatus`, `Preview`, `PublishVersion` + `Validate` (WF-06, all problems returned), `History`, `TimeInSteps`. Typed errors map to 403/409/422 problems. Metrics `linochi_workflow_transitions_total{override}` and `linochi_workflow_refusals_total{reason}`, one slog line per change.
- Effects follow from the stock phase of the two steps and from what the ledger says the order line holds: reserve missing, release, ship (FEFO), return (one `return` movement per sold batch, `quantity_shipped` decremented). Reserve is idempotent, so a reopened order that already holds a reservation is not reserved twice.
- Status changes only through the engine: the free-text `Status` path is gone from create, update and the old status handlers. Accept, ship, cancel and the generic status endpoint call `orders.ChangeOrderStatus`. The order row is locked `FOR UPDATE` and its real version is used for the optimistic lock (ORD-04 version=0 bug).
- OpenAPI: old step/transition/condition/effect CRUD removed; added `workflow/versions` (list, active, get, publish), `orders/{id}/transitions/{to_key}/preview`, `orders/{id}/history`; status endpoints return `OrderStatusChange {order, effects, warnings}` with 403/409/422 documented. `make gen` run.

## Tests
- `make check` (lint, unit, integration with -race): green. `go test -tags integration ./internal/workflow ./internal/orders`: green.
- New tests, all written and committed failing before the implementation:
  - workflow: TestWF02_TransitionNotInFlowIsRefused, TestWF03_EffectsRunInSameTransaction, TestWF05_OverrideRequiresAdminAndReason, TestWF05_OverrideBackwardsRestoresStock, TestWF05_PreviewShowsEffectsWithoutRunningThem, TestWF05_DeliveredIsFinal, TestWF05_ReopenCancelledReservesAgain (3 sub-tests), TestWF06_InvalidFlowIsRejectedOnSave (6 cases + valid), TestWF06_ValidateTable (unit), TestWF07_OrderKeepsItsWorkflowVersion, TestWF10_HistoryRecordsEveryChange, TestNFK27_StockNeverNegativeUnderRandomTransitions (6 seeds x 60 moves, about a third accepted).
  - orders: TestORD04_AcceptOrderSucceeds, TestORD04_AcceptWithoutStockIsRefused, TestORD06_ShipOrderWithdrawsStock, TestORD07_CancelOrderReleasesReservation, TestORD03_StatusOnlyChangesThroughTheEngine.
- Existing tests changed because the API they used is gone: `internal/audit/t3_integration_test.go` (workflow audit test now publishes a version), `internal/platform/auth/contract_integration_test.go` (step/transition CRUD replaced by version, preview and history calls; status calls use the new bodies).

## Not done / skipped
- Conditions (WF-04, "tracking number required") and webhook/print effects: not in the brief's tests; the old condition/effect tables are gone.
- No HTTP-level test that an admin session gets 200 on an override and a non-admin 403: the engine tests cover it with an `Actor`; the contract test only checks response shapes. `workflow.ActorFromRequest` is untested.
- `shipped_at` / `delivered_at` on orders are not set by the engine.
- Per-channel flows (WF-08/09) and the web editor are out of scope as the brief says.
- `ship` always uses FEFO; there is no way to pass a batch through the status endpoint.
- The contract test cannot reach a successful accept over HTTP (its order has no stock); that path is tested at service level only.

## Assumptions
- `accepted` is reachable from `in_review`, not from `new`, because the brief says "each step to the next" (`internal/workflow` seed in the migration). The Accept endpoint from `new` therefore answers 409.
- `workflow_steps` has three flags beyond the brief's `is_final`: `is_start` (WF-06 needs exactly one), `is_locked` (delivered: no override out) and `stock_phase` (none/reserved/shipped) so that override effects need no step names in code. `Validate` checks every transition's `effect` against the two phases.
- `workflow_versions.version` is the flow version number (as in the brief) and has default 1 only to satisfy the standard-columns test (`TestNFK28_EveryTableHasStandardColumns`); it is not an optimistic-lock counter. A published version is immutable except `is_active`.
- Override may only target a non-final step (brief wording). Cancelling something already shipped is therefore not possible by override. Leaving `cancelled`/`rejected` (final, not locked) by override is the reopen case.
- Reopen: reservation is best effort and all-or-nothing (simulated first), only when the target step holds no reservation by phase. `engine.go` `plan`, comment `ASSUMPTION`.
- An order line has no stock location: one location per line is chosen at reserve time (most available, ties by id). `engine.go` `pickLocation`.
- Ship location is the location where the line is reserved.
- Accept/ship/cancel endpoints target the fixed keys `accepted`, `posted`, `cancelled` (`internal/orders/service.go`); which moves are allowed still comes from the flow.
- `POST /workflow/versions` is admin only (session role), the brief does not say.
- `CreateOrder` no longer takes a status; it starts at the column default `new`. Existing orders with a status that is not a step key become `new` in the migration.
- `internal/platform/db/dbgen` and `internal/api/api.gen.go` regenerated.
- `db/queries/stats.sql`: two hard-coded status lists updated to the new keys (`shipped` -> `posted`; `draft, confirmed, accepted` -> `new, in_review, accepted`). Outside the brief, mechanical.

## Open questions
- Should `new -> accepted` exist as a shortcut? As seeded, the review step cannot be skipped.
- Should an admin be able to cancel a shipped order by override (stock would come back via return)? The brief's wording says no.

## Where the reviewer should look first
- `internal/workflow/engine.go` `plan` and `runEffects`: the phase-delta logic, the reopen branch and the idempotent reserve.
- `db/migrations/20261118200000_order_workflow.sql`: seed DO block, the `default_workflow_version_id()` column default, status backfill, Down.
- `internal/workflow/service.go` `Transition`: ordering of authorization, plan, effects, status update, history, audit.
- `internal/orders/handler.go` `change`: how the admin role is taken from the session.
