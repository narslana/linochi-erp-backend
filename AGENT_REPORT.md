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
(updated as the work proceeds)

## Tests

## Not done / skipped

## Assumptions

## Open questions

## Where the reviewer should look first
