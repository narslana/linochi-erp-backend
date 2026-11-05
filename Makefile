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

.PHONY: help dev gen build test test-int lint check

help: ## List the targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-10s %s\n", $$1, $$2}'

dev: ## Start Postgres in Docker Compose and run the API locally
	docker compose up -d --wait
	@set -a; [ -f .env ] && . ./.env; set +a; \
	  DATABASE_URL="$${DATABASE_URL:-postgres://linochi:dev@localhost:5432/linochi_erp?sslmode=disable}" \
	  go run -ldflags "$(LDFLAGS)" ./cmd/api

gen: ## Generate code (nothing to generate yet: sqlc arrives in B3, oapi-codegen in B6)
	@echo "gen: nothing to generate yet"

build: ## Build bin/api (static, with version and commit)
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/api ./cmd/api

test: ## Unit tests
	go test $(RACE) -cover ./...

test-int: ## Integration tests with testcontainers (needs Docker)
	go test $(RACE) -tags=integration -count=1 ./...

lint: ## gofmt, go vet, golangci-lint and go mod tidy
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	go vet -tags=integration ./...
	$(GOLANGCI) run ./...
	go mod tidy -diff

check: lint test test-int ## Everything. Must be green before commit
