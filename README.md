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
| `make dev` | Starts Postgres in Docker Compose and runs the API locally |
| `make build` | Builds `bin/api` (static, with version and commit) |
| `make test` | Unit tests |
| `make test-int` | Integration tests with testcontainers |
| `make lint` | gofmt, go vet, golangci-lint and `go mod tidy -diff` |
| `make gen` | Code generation (nothing to generate yet) |
| `make check` | Lint, `test` and `test-int`. Must be green before commit |

`make dev` reads `.env` if it exists (start from `.env.example`). Compose starts only
Postgres until the first feature that stores files arrives, then RustFS is added.

The race detector (`-race`) runs by default. CI on arm64 runs `make check RACE=`
because ThreadSanitizer aborts on the Raspberry Pi 5 default kernel (see
`docs/DECISIONS.md` in the workspace).

## Configuration

Environment variables only (NFK-02, NFK-14). No files are read in production.

| Variable | Default | |
|---|---|---|
| `DATABASE_URL` | | Required. `postgres://…`. Contains a secret and is masked in logs |
| `HTTP_ADDR` | `:8080` | The public API (`/api`, `/auth`) |
| `INTERNAL_ADDR` | `:9090` | `/healthz`, `/readyz`, `/metrics` and later API keys. No ingress route |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |
| `SHUTDOWN_DELAY` | `5s` | After SIGTERM `/readyz` turns 503 at once and the ports stay open this long, so that Traefik has time to stop sending traffic. `0s` locally |
| `SHUTDOWN_TIMEOUT` | `20s` | How long in-flight requests get to finish afterwards |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | empty | Base URL for OTLP/HTTP, in the cluster `http://alloy.alloy.svc.cluster.local:4318`. Empty = spans are created (the logs get `trace_id`) but not sent |
| `POD_NAME` | empty | The replica's name in traces |

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
