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
| `make dev-oidc` | In another terminal: a local OIDC provider that logs everybody in as one fixed user, so that the login can be tried without Authelia. Loopback only, never in the image |
| `make build` | Builds `bin/api` (static, with version and commit) |
| `make test` | Unit tests |
| `make test-int` | Integration tests with testcontainers |
| `make lint` | gofmt, go vet, golangci-lint, `sqlc diff` and `oapi-codegen` (generated code is up to date) and `go mod tidy -diff` |
| `make gen` | Generates Go code from `db/queries` with sqlc and from `api/openapi.yaml` with oapi-codegen (`internal/api`) |
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
| `OIDC_ISSUER` | | Required to serve. Authelia's URL, `https://auth.lab.internal`. https only (http is accepted for `localhost`, `127.0.0.1` and `[::1]`, for `make dev-oidc`). The discovery document must claim exactly this issuer |
| `OIDC_CLIENT_ID` | | Required to serve. `linochi-erp` |
| `OIDC_CLIENT_SECRET` | | Required to serve. The clear text of the client secret whose hash is in Authelia's configuration. Masked in logs and errors |
| `OIDC_REDIRECT_URL` | | Required to serve. `https://erp.lab.internal/auth/callback`: it must end in `/auth/callback`, and Authelia matches it exactly |
| `AUTH_ADMIN_GROUPS` | `admins,linochi_erp_users` | The Authelia groups that give the admin role. Exact names, comma-separated. There is no read role yet |

`api migrate` and `api sessions` need only their database URL: an operator must be able to end sessions when the login is what is broken.

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

## Login (NFK-10, NFK-12)

Login is the authorization code flow with PKCE against Authelia, done by the backend: the browser never sees a token, only the session cookie. The routes are in `api/openapi.yaml` (OpenAPI 3.1), which is the source: `make gen` makes `internal/api`, and `make lint` fails when it is out of date. The code is in `internal/platform/auth`.

- **Routes:** `GET /auth/login`, `GET /auth/callback`, `POST /auth/logout` and `GET /api/v1/me`. Only the last needs a session. A test reads the spec and checks that every operation it declares as secured answers 401 without one, and that every answer matches the spec (NFK-29).
- **`/auth/login`** stores a login in progress in Postgres (`oidc_login_attempts`, 10 minutes, so the callback may land on any replica), binds it to the browser with the `__Host-linochi_erp_login` cookie and sends the browser to Authelia with `state`, `nonce` and a PKCE challenge (S256). `return_to` must be a path of this app: anything else, `//host` and backslashes included, becomes `/`. At most 5000 logins may be open at once, since the route needs no session.
- **`/auth/callback`** consumes the attempt first (`DELETE ... RETURNING`: a state works once, also across replicas, and also when the callback is refused), then checks, in this order: the browser holds the binding cookie; the code is traded for tokens with the PKCE verifier and `client_secret_basic`; the ID token is **RS256** and has the right issuer, audience, expiry and nonce; **`amr` contains `mfa`** if the claim is there; userinfo is for the same subject as the ID token; the groups give a role; the user is not archived. Only then are a user and a session made. Tokens are used and dropped, never stored or logged.
- **Roles** are worked out from the Authelia groups (`AUTH_ADMIN_GROUPS`) at every login and kept in the session. A person with no matching group gets no user and no session. Removing someone's access in Authelia does not reach a session that exists: end it with `api sessions revoke-user`.
- **Two-factor** is Authelia's `two_factor` policy, and `amr` is a second lock. If the provider does not deliver `amr` the login goes through and the log says so, `the ID token has no amr claim`. What Authelia 4.39 really sends is checked in H3, against the real one.
- **Authelia down** does not stop the app: the provider is contacted at the first login (and again five seconds after a failure), so the app starts, and `/readyz` stays green for the people already in. A login then ends at the error page.
- **Failed logins** always end in a `303` to the web app's page `/login-error?code=<code>` with one of three fixed codes: `no-access` (no role, an archived account, no second factor, Authelia said no), `expired` (too slow, already used or unknown) and `failed` (anything else). Never a detail in the URL: the reason is in the log with the `trace_id`, and in `linochi_auth_logins_total{result}`. The web app owns that page and its Swedish text.
- **Metric:** `linochi_auth_logins_total{result}`, `ok` or the reason (`no_role`, `mfa_missing`, `attempt_expired`, `binding_mismatch`, `provider_unavailable` and so on, a closed set, every series there from the start). The refused-logins alert (B7) watches everything but `ok`.
- **The internal CA (NFK-12):** the app trusts the system roots and nothing else. In the cluster the CA is mounted from a ConfigMap and `SSL_CERT_DIR=/etc/ssl/certs:/extra-ca-certs` adds it, as for Gitea and Grafana (Go adds those directories). A test starts the real binary with and without it.
- **Locally:** `make dev-oidc` (`cmd/devoidc`) is a provider on `127.0.0.1:9096` that logs everybody in as one fixed user, and `make dev` already points at it. It refuses to listen anywhere but on loopback, and a test checks that it is not in the `cmd/api` binary. `__Host-` cookies work over `http://localhost` in Chromium 156 and Firefox 157 (tried with the whole login, from Playwright), so there is no development mode for the cookie. Safari was not tried and MDN says it does not make the `localhost` exception for `Secure` cookies; use another browser locally, or HTTPS.

## Sessions (NFK-02, NFK-11)

Sessions live in Postgres (`sessions`), so every replica sees the same ones and a rolling update logs nobody out. The code is in `internal/platform/auth`.

- **Token:** 32 random bytes in the cookie. Only its SHA-256 is stored, never the token. `Session`, the type that goes into audit entries, has no field for the token or the hash, because an append-only log could never have it redacted out.
- **Limits:** logged out after 12 hours without use and after 7 days whatever the activity (`IdleTimeout`, `AbsoluteTimeout`: constants, so two replicas can never disagree). `expires_at` is set once and the app role has no UPDATE privilege on it. A session in use is written at most once a minute (`TouchInterval`), so a session can be cut short by up to a minute and never kept alive past its limit.
- **Cookie:** `__Host-linochi_erp_session`, `Secure`, `HttpOnly`, `SameSite=Lax`, `Path=/`, no `Domain`, and a `Max-Age` that ends with the 7 days. It is the same in every environment: there is no switch that weakens it.
- **Middleware:** `SessionMiddleware.Require` answers 401 without a valid session (and clears a cookie that is no good), puts the session in the context and the user as audit actor, and refuses cross-origin writes with 403 through `http.CrossOriginProtection`. `LogoutHandler` ends the session and clears the cookie. The routes are described under Login above.
- **Audit:** login (a create of the session), logout and "all sessions of a user ended" are recorded in the same transaction. Expiry and cleanup are not: they show up as a log line and a metric. Archiving a user ends their sessions in the same transaction as the archive, and a login for an archived user fails, also when the two happen at the same moment.
- **Cleanup:** every replica deletes ended sessions (and expired login attempts) at start-up and then every hour. It is idempotent, so two replicas at once is fine, and a failed run is only logged. `sessions` and `oidc_login_attempts` are the two tables where the app role may DELETE: a session is a credential, not a record, and what happened to it is in `audit_log`; a login attempt is a credential in the making.
- **Ending all sessions of a user** (access removed in Authelia, a lost device): the roles live in the session, so without this a person whose access was removed could stay logged in for up to 7 days.

  ```sh
  api sessions revoke-user <email|user-id>
  ```

  In the cluster it is run with `kubectl exec` in a backend pod. The exact line (namespace, Deployment, path of the binary) is added when B7 has built the image and the chart: it has not been run in a cluster yet, only in the tests. It runs as the app role with `DATABASE_URL`, accepts an e-mail address (case-insensitive) or a user id, refuses an unknown or ambiguous address (and lists the ids), prints the user id and the count and never the address, and records the change with the system actor `cli-revoke-user`.
- **Metrics:** `linochi_auth_session_checks_total{result}`, `linochi_auth_sessions_deleted_total{reason}` (`logout`, `revoked`, `cleanup`) and `linochi_auth_session_cleanup_runs_total{result}`. Lookup and cleanup have spans.

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
