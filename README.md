# linochi-erp-backend

Go-backend för Linochi ERP. Se `PROJECT.md` och `AGENTS.md` i arbetsmappen
(`linochi-erp-workspace`) för vad vi bygger och hur vi arbetar.

## Krav

- Go 1.27.3
- make
- Docker med Compose (integrationstesterna startar PostgreSQL med testcontainers)

Inget annat behöver installeras. golangci-lint ligger som `tool` i
`tools/go.mod`, en egen modul, så att verktygens beroenden inte hamnar i
appens `go.mod`. sqlc, goose och oapi-codegen läggs dit i sina steg.

## Kommandon

| Kommando | Gör |
|---|---|
| `make dev` | Startar Postgres i Docker Compose och kör API:t lokalt |
| `make build` | Bygger `bin/api` (statisk, med version och commit) |
| `make test` | Enhetstester |
| `make test-int` | Integrationstester med testcontainers |
| `make lint` | gofmt, go vet, golangci-lint och `go mod tidy -diff` |
| `make gen` | Kodgenerering (inget att generera ännu) |
| `make check` | Lint, `test` och `test-int`. Måste vara grönt före commit |

`make dev` läser `.env` om den finns (börja från `.env.example`). Compose
startar bara Postgres tills första funktionen som lagrar filer kommer, då
läggs RustFS till.

## Konfiguration

Bara miljövariabler (NFK-02, NFK-14). Inga filer läses i drift.

| Variabel | Standard | |
|---|---|---|
| `DATABASE_URL` | | Krävs. `postgres://…`. Innehåller hemlighet och maskeras i loggar |
| `HTTP_ADDR` | `:8080` | Publikt API |
| `INTERNAL_ADDR` | `:9090` | `/metrics` och senare API-nycklar. Ingen ingress-route |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` eller `error` |
| `SHUTDOWN_TIMEOUT` | `20s` | Hur länge pågående anrop får avslutas vid SIGTERM |

## PostgreSQL-version

Klustret kör `ghcr.io/cloudnative-pg/postgresql:18.6-system-trixie`. Lokalt och
i tester används `postgres:18.6-trixie`: samma major och minor och samma Debian,
så att kollationering och locale stämmer. Taggen står på två ställen som byts
tillsammans med klustret: `compose.yaml` och `internal/platform/testdb/testdb.go`.

## Tester

Testnamn börjar med krav-id (`TestNFK02_…`, `TestINS07_…`). Integrationstester
har build-taggen `integration` och körs med `make test-int`. De ansluter till
containern över nätverket (inte `exec` i containern), eftersom det är det som
brukar gå sönder i Docker-in-Docker i CI.
