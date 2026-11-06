# linochi-erp-backend

The Go backend for Linochi ERP. See `PROJECT.md` and `AGENTS.md` in the workspace
(`linochi-erp-workspace`) for what we are building and how we work.

## Requirements

- Go 1.27.3
- make
- Docker with Compose (the integration tests start PostgreSQL with testcontainers)

Nothing else needs to be installed. golangci-lint is a `tool` in `tools/go.mod`, a
module of its own, so that the tools' dependencies do not end up in the app's
`go.mod`. sqlc, goose and oapi-codegen are added there in their steps.

## Commands

| Command | Does |
|---|---|
| `make dev` | Starts Postgres in Docker Compose, migrates as the owner and runs the API as the app role |
| `make build` | Builds `bin/api` (static, with version and commit) |
| `make test` | Unit tests |
| `make test-int` | Integration tests with testcontainers |
| `make lint` | gofmt, go vet, golangci-lint, `sqlc diff` (generated code is up to date) and `go mod tidy -diff` |
| `make gen` | Generates Go code from `db/queries` with sqlc (the OpenAPI client joins in B6) |
| `make migrate-new name=...` | A new goose migration in `db/migrations` |
| `make check` | Lint, `test` and `test-int`. Must be green before commit |

`make dev` reads `.env` if it exists (start from `.env.example`). Compose starts only
Postgres until the first feature that stores files arrives, then RustFS is added.
The two database roles are created by `db/dev/initdb` when the Compose volume is
empty; after changing that script run `docker compose down -v`.

The race detector (`-race`) runs by default. CI on arm64 runs `make check RACE=`
because ThreadSanitizer aborts on the Raspberry Pi 5 default kernel (see
`docs/DECISIONS.md` in the workspace).

## Configuration

Environment variables only (NFK-02, NFK-14). No files are read in production.

| Variable | Default | |
|---|---|---|
| `DATABASE_URL` | | Required to serve. `postgres://…` as the restricted app role. Contains a secret and is masked in logs |
| `MIGRATE_DATABASE_URL` | | Required by `api migrate` only. The same database as the owner role, which can change the schema. There is no fallback to `DATABASE_URL` |
| `HTTP_ADDR` | `:8080` | The public API (`/api`, `/auth`) |
| `INTERNAL_ADDR` | `:9090` | `/healthz`, `/readyz`, `/metrics` and later API keys. No ingress route |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |
| `SHUTDOWN_DELAY` | `5s` | After SIGTERM `/readyz` turns 503 at once and the ports stay open this long, so that Traefik has time to stop sending traffic. `0s` locally |
| `SHUTDOWN_TIMEOUT` | `20s` | How long in-flight requests get to finish afterwards |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | empty | Base URL for OTLP/HTTP, in the cluster `http://alloy.alloy.svc.cluster.local:4318`. Empty = spans are created (the logs get `trace_id`) but not sent |
| `POD_NAME` | empty | The replica's name in traces |
| `DB_MAX_CONNS` | `5` | Pool size per pod, 1 to 50. The cluster has 100 connections for all services, see the budget in `docs/DECISIONS.md` |
| `DB_MIN_CONNS` | `0` | Connections kept open when idle |
| `DB_MAX_CONN_LIFETIME` | `30m` | How long a connection is reused |

## Database

PostgreSQL through `pgx` and `sqlc`. The schema is the goose migrations in `db/migrations`, embedded in the binary.

- **Two roles** (see `docs/DECISIONS.md`): `linochi_erp_owner` owns the schema and runs migrations (`MIGRATE_DATABASE_URL`), `linochi_erp_app` is what the running app connects as (`DATABASE_URL`). The app role gets only what each migration grants it, per table: no DELETE (data is kept, NFK-09), no DDL, no TRUNCATE. A third role, `linochi_erp_audit_redactor`, cannot log in and owns the redaction function; the owner is a member of it (`WITH SET TRUE, INHERIT FALSE`) so that a migration can hand the function over, and the app role never is. The roles must exist before `api migrate up`: in the cluster H1 creates them, in Compose `db/dev/initdb`, in tests `internal/platform/testdb`. Every migration grants on its own tables, explicitly.

- **Migrate:** `api migrate up|down|status` (locally `./bin/api migrate up`). The serving process never migrates; the PreSync Job in the chart runs `api migrate up` with the same image. `down` rolls back one migration and is meant for local use.
- **Queries:** write SQL in `db/queries/<module>.sql` with a `-- name: GetThing :one` comment and run `make gen`. The name is also the label of the query metrics.
- **Every table** has `id` (UUIDv7, made in the app, with `uuidv7()` as a default), `created_at`, `updated_at` and `version`. A test checks this for every table. Writes check `version` (optimistic locking) and return `db.ErrStaleVersion`, which becomes a 409 problem.
- **Start-up:** the app starts while the database is down or the schema is behind. `/readyz` is red (503) until the database answers and the schema is at least as new as the binary's migrations. A newer schema is accepted, so a rollback to the previous image works.
- **Metrics:** `linochi_db_query_duration_seconds{query}` and `linochi_db_pool_*`. SQL spans are children of the request span.

## Change log (INS-05)

Every write that changes data records an entry in `audit_log` **in the same transaction**, with `internal/audit`. `audit.Recorder.Record` takes a `*db.Tx` and not a pool, so it cannot be called outside a transaction.

- **Actor:** from the context (`audit.WithActor`): a user, an API key or the system (named by a label such as `oidc-login`, never a person). No actor means `audit.ErrNoActor` and the whole transaction is rolled back: an unattributed change is not saved.
- **What is stored:** a create stores the whole record as `after`, an archive the whole record as `before`, anything else only the changed fields, with `before` and `after` per field. `version` and `updated_at` are left out of those diffs. A change that changes nothing writes nothing.
- **Append-only:** the app role has no UPDATE, DELETE or TRUNCATE on `audit_log`, and a trigger also stops the owner when it is connected directly. The one way to change old rows is the SECURITY DEFINER function `audit_redact_personal_data()` (GDPR, KUN-07), called through `Recorder.RedactPersonalData`, which also records an entry about the redaction. The function is owned by a role of its own, `linochi_erp_audit_redactor`: it cannot log in and can UPDATE only `before`, `after` and `redacted_at`. The trigger lets through `current_user = linochi_erp_audit_redactor` and nobody else, so a SECURITY DEFINER function written by the owner is stopped too. The function replaces the values of the given fields with `[redacted]` and nothing else. Old values stay in backups until those are rotated out.
- **Loki and metrics:** after the commit, one log line `audit` with `trace_id`, the actor, the record and the **names** of the changed fields, never the values, and `linochi_audit_entries_total{module,action}`. A rolled-back change never shows up.

## Observability

- **Internal port (`INTERNAL_ADDR`):** `/healthz` (liveness, depends on nothing), `/readyz` (503 while draining or when a check fails) and `/metrics`.
- **Metrics:** `linochi_http_requests_total`, `linochi_http_request_duration_seconds` (bounds at 0.2 and 0.5 s for NFK-03), `linochi_http_requests_in_flight`, `linochi_build_info`, and `go_*` and `process_*`. The `route` label is the pattern (`GET /api/v1/items/{id}`), and everything that does not match is called `unmatched`.
- **Logs:** JSON on stderr with `trace_id` and `span_id`. Name, e-mail, phone, address, passwords and tokens are masked (`[masked]`), also through `obs.Personal(...)`. The access log never contains the query string.
- **Errors:** `application/problem+json` (RFC 9457) with `trace_id`. Typed errors implement `httpx.ProblemError`.
- **Traces against the real Alloy locally:** `kubectl port-forward -n alloy svc/alloy 14318:4318`, then `OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:14318 make dev`. The trace shows up in Tempo after about half a minute.

## PostgreSQL version

The cluster runs `ghcr.io/cloudnative-pg/postgresql:18.6-system-trixie`. Locally and
in tests `postgres:18.6-trixie` is used: the same major and minor and the same
Debian, so that collation and locale match. The tag is in two places that change
together with the cluster: `compose.yaml` and `internal/platform/testdb/testdb.go`.

## Tests

Test names start with the requirement ID (`TestNFK02_…`, `TestINS07_…`).
Requirements without an ID use `TestOBS_…` and `TestAPI_…`. Integration tests have
the build tag `integration` and run with `make test-int`. They connect to the
container over the network (not `exec` in the container), because that is what
tends to break in Docker-in-Docker in CI.
