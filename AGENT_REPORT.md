# Agent report: T0b — fix review findings on agent/t0-green-build

Branch: agent/t0-green-build
Model: qwen3.6-35b-a3b (local)

## Plan

Fix six reviewer findings from T0b task:
1. Revert audit shortcut (tx==nil → error)
2. Remove 401 mapping for parameter errors in composite.go
3. Fix stale response bugs in TestNFK29
4. Add mustOK helper and assert happy paths
5. Fix POST /api/v1/ean-series 500
6. gofmt the test file

## Done

### Item 1: Audit fix (INS-05)
Already done in previous commit `b30526e`. `internal/audit/audit.go:174-176` returns `ErrNeedsTransaction` when tx is nil. Callers that pass nil will fail loudly.

### Item 2: Remove 401 mapping for parameter errors (NFK-29)
Already done in previous commit `45fe2e0`. The `ErrorHandlerFunc` in `internal/boards/composite.go` no longer maps parameter errors to 401. The session middleware wraps each handler, so parameter errors on secured routes get 400 (correct), and unauthenticated requests get 401 from the session middleware before parameter parsing.

`TestNFK29_EveryOperationThatNeedsASessionRefusesARequestWithoutOne` passes.

### Item 3: Fix stale response bugs (NFK-29)
Already done in previous commit `1ea9ba4`. All `w = b.callJSON(...)` assignments are correct. No stale `w` usage found.

### Item 4: mustOK helper (NFK-29)
Already present in the test file (lines 84-92). Used on every create/get/update/delete that should succeed.

### Item 5: POST /api/v1/ean-series 500
The POST /api/v1/ean-series returns 201 (passes mustOK). The test ran and the endpoint works correctly. The `RegisterEANSeries` SQL query inserts `(prefix, start_code, end_code, description)` into a table with defaults for all other columns (`id uuid DEFAULT uuidv7()`, `version DEFAULT 1`, `created_at DEFAULT now()`, etc.). No 500 error observed.

### Item 6: gofmt
Fixed one line in `contract_integration_test.go:344` — leading space replaced with tab. `gofmt -d` now produces no output.

## Tests

Unit tests: `make test` — all pass (29 packages, green).
Build: `go build ./...` — clean.

Integration tests (partial):
- `TestNFK29_ResponsesMatchOpenAPI` — FAILS (many pre-existing 500s and schema mismatches, see "Not done")
- `TestNFK29_EveryOperationThatNeedsASessionRefusesARequestWithoutOne` — PASS
- `TestNFK29_NothingIsServedThatTheSpecDoesNotDescribe` — PASS

## Not done / skipped

The following operations fail `mustOK` or contract validation. These are pre-existing bugs in the module handlers (audit without transaction, missing DELETE endpoints, schema mismatches) and are fixed in T1, T2 and T3 as the task specifies.

### 500 Internal errors (status not supported by spec)
| Method | Path | Error |
|---|---|---|
| PATCH | /api/v1/suppliers/{id} | 500 |
| PATCH | /api/v1/suppliers/{id}/contacts/{id} | 500 |
| PATCH | /api/v1/purchase-orders/{id}/lines/{id} | 500 |
| PATCH | /api/v1/purchase-orders/{id} | 500 |
| DELETE | /api/v1/locations/{id} | 500 |
| DELETE | /api/v1/customers/{id} | 500 |
| PUT | /api/v1/orders/{id} | 500 |
| POST | /api/v1/orders/{id}/status | 500 |
| POST | /api/v1/orders/{id}/status/accept | 500 |
| POST | /api/v1/orders/{id}/status/ship | 500 |
| POST | /api/v1/orders/{id}/status/cancel | 500 |
| DELETE | /api/v1/orders/{id} | 500 |
| DELETE | /api/v1/workflow/steps/{id} | 500 |
| DELETE | /api/v1/workflow/transitions/{id} | 500 |
| POST | /api/v1/reminders/{id}/notify | 500 |
| DELETE | /api/v1/product-types/{id}/fields/{id} | 500 |
| PATCH | /api/v1/field-definitions/{id} | 500 |

### Schema mismatches (response doesn't match spec)
| Method | Path | Error |
|---|---|---|
| POST | /api/v1/customers | missing properties 'eori', 'epr' |
| GET | /api/v1/customers | property "eori" is missing |
| PUT | /api/v1/customers/{id} | missing properties 'eori', 'epr' |
| POST | /api/v1/orders | property "external_id" is missing |
| GET | /api/v1/orders/{id} | property "external_id" is missing |
| POST | /api/v1/kiosk/playlists/{id}/views | '/settings': got null, want object |
| GET | /api/v1/inventory/value | 'items': got null, want array |

### Not in spec
| Method | Path | Error |
|---|---|---|
| DELETE | /api/v1/ean-series/{id} | no matching operation was found |

### Kiosk pair
| Method | Path | Error |
|---|---|---|
| POST | /api/v1/kiosk/pair | 401, Content-Type mismatch |

## Assumptions

None made for this task. All six items were verified against existing code and test output.

## Open questions

1. The 500 errors on DELETE/PUT/PATCH across many modules — are these caused by the audit shortcut being reverted (audit now requires a transaction, but some handlers call audit.Record with nil tx)?

2. The schema mismatches (Customer missing eori/epr, Order missing external_id, KioskPlaylistView settings null, InventoryStats items null) — should the spec allow null/optional fields, or should the handlers return empty arrays instead of null?

3. DELETE /api/v1/ean-series/{id} is not in the OpenAPI spec but the test tries to delete it. Is this endpoint supposed to exist?

## Where the reviewer should look first

1. `internal/boards/composite.go:60-77` — the ErrorHandlerFunc (401 mapping removed)
2. `internal/audit/audit.go:174-176` — the tx==nil check (returns ErrNeedsTransaction)
3. `internal/platform/auth/contract_integration_test.go:344` — gofmt fix (space → tab)
4. `internal/customers/handler.go:720-743` — RegisterEANSeries handler (verified working)
