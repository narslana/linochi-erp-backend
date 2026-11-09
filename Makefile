# Linochi ERP backend. Run `make help` for the targets.
SHELL := /bin/bash
.DEFAULT_GOAL := help

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short=7 HEAD 2>/dev/null || echo unknown)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)

# Race detector. Off in CI on arm64: ThreadSanitizer aborts on the Pi 5 default
# kernel (47-bit VMA). Runs locally and wherever the kernel supports it.
RACE ?= -race

GOLANGCI := go tool -modfile=tools/go.mod golangci-lint

.PHONY: help dev dev-oidc gen migrate-new build test test-int lint check

help: ## List the targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-10s %s\n", $$1, $$2}'

DEV_DATABASE_URL ?= postgres://linochi_erp_app:dev@localhost:5432/linochi_erp?sslmode=disable
DEV_MIGRATE_DATABASE_URL ?= postgres://linochi_erp_owner:dev@localhost:5432/linochi_erp?sslmode=disable

# What cmd/devoidc serves by default, so that `make dev-oidc` and `make dev` meet
# without a .env. The secret is nothing real.
DEV_OIDC_ISSUER ?= http://127.0.0.1:9096
DEV_OIDC_CLIENT_SECRET ?= dev-only-client-secret
DEV_OIDC_REDIRECT_URL ?= http://localhost:8080/auth/callback

dev: ## Start Postgres in Docker Compose, migrate as the owner and run the API as the app role (login: make dev-oidc)
	docker compose up -d --wait
	@set -a; [ -f .env ] && . ./.env; set +a; \
	  export DATABASE_URL="$${DATABASE_URL:-$(DEV_DATABASE_URL)}" \
	         MIGRATE_DATABASE_URL="$${MIGRATE_DATABASE_URL:-$(DEV_MIGRATE_DATABASE_URL)}" \
	         OIDC_ISSUER="$${OIDC_ISSUER:-$(DEV_OIDC_ISSUER)}" \
	         OIDC_CLIENT_ID="$${OIDC_CLIENT_ID:-linochi-erp}" \
	         OIDC_CLIENT_SECRET="$${OIDC_CLIENT_SECRET:-$(DEV_OIDC_CLIENT_SECRET)}" \
	         OIDC_REDIRECT_URL="$${OIDC_REDIRECT_URL:-$(DEV_OIDC_REDIRECT_URL)}"; \
	  go run -ldflags "$(LDFLAGS)" ./cmd/api migrate up && \
	  go run -ldflags "$(LDFLAGS)" ./cmd/api

dev-oidc: ## Run a local OIDC provider that logs everybody in as one fixed user (loopback only, never in the image)
	go run ./cmd/devoidc -redirect "$(DEV_OIDC_REDIRECT_URL)"

SQLC := go tool -modfile=tools/go.mod sqlc
GOOSE := go tool -modfile=tools/go.mod goose
OAPI := go tool -modfile=tools/go.mod oapi-codegen

gen: ## Generate Go code from db/queries with sqlc and from api/openapi.yaml with oapi-codegen
	$(SQLC) generate
	$(OAPI) -config api/oapi-codegen.yaml -o internal/api/api.gen.go api/openapi.yaml

migrate-new: ## New goose migration: make migrate-new name=add_things
	@test -n "$(name)" || { echo "usage: make migrate-new name=<description>"; exit 2; }
	$(GOOSE) -dir db/migrations create $(name) sql

build: ## Build bin/api (static, with version and commit)
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/api ./cmd/api

test: ## Unit tests
	go test $(RACE) -cover -timeout 5m ./...

test-int: ## Integration tests with testcontainers (needs Docker)
	go test $(RACE) -tags=integration -count=1 -timeout 8m ./...

lint: ## gofmt, go vet, golangci-lint, sqlc diff and go mod tidy
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	go vet -tags=integration ./...
	$(GOLANGCI) run ./...
	$(SQLC) diff
	@$(OAPI) -config api/oapi-codegen.yaml api/openapi.yaml | diff -q - internal/api/api.gen.go >/dev/null \
	  || { echo "internal/api/api.gen.go is out of date: run make gen"; exit 1; }
	go mod tidy -diff

check: lint test test-int ## Everything. Must be green before commit
