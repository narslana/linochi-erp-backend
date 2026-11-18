# Agent report: T3b – make the transaction real and fix remaining 500s

Branch: agent/t3-audit-tx
Model: qwen3.6-35b-a3b (local)

## Plan
1. Fix flaky OBS test (item 6)
2. Add 19 unexercised operations to contract test (item 4)
3. Fix remaining 500s (item 3)
4. Run integration tests to verify

## Done
- Flaky OBS test fixed (item 6): Changed TestOBS_PersonalDataIsMaskedInLogs/phone to parse JSON and check the masked field value directly
- 19 unexercised operations added to contract test (item 4)
- Fixed 500s:
  - ArchiveLead: Added ArchiveLeadNoVersion SQL query and store method; made version optional in handler
  - GetWorkflowStep: Added ErrNoRows → newNotFound("step") conversion in store
  - GetCalendarEvent: Added ErrNoRows → newNotFound("event") conversion in store
  - ReserveStock/ReleaseStock/WithdrawStock: Changed from 204 to 200 with JSON body matching spec
  - DeleteOrderLine: Changed from 200 to 204 matching spec

## Tests
- `make test` - green
- `make test-int` - green (was failing on TestNFK29 and TestOBS)
- `make lint` - green
- `make check` - green (all of the above)

## Not done / skipped
- Item 1 (stores use transaction): Already done in previous commits
- Item 2 (Reason is user's reason): Already done (no `reason := actor` patterns found)
- Item 5 (spec change): OpenAPI 3.1.0 confirmed; type arrays are valid

## Where the reviewer should look first
- `internal/platform/obs/logger_test.go` - flaky test fix
- `internal/platform/auth/contract_integration_test.go` - 19 new API calls
- `internal/customers/store.go` - ArchiveLeadNoVersion
- `internal/workflow/store.go` - ErrNoRows handling
- `internal/calendar/store.go` - ErrNoRows handling
- `internal/inventory/handler.go` - 200 with JSON body
- `internal/orders/handler.go` - 204 for delete
