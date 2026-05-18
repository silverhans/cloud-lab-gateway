# Cloud Lab Gateway — developer workflow
# Targets are grouped: tooling install, code generation, build, test, lint, sec, run.

.DEFAULT_GOAL := help
SHELL := /bin/bash

GO          ?= go
GOLANGCI    ?= golangci-lint
GOSEC       ?= gosec
GITLEAKS    ?= gitleaks
HADOLINT    ?= hadolint
TRIVY       ?= trivy
GOOSE       ?= goose
SQLC        ?= sqlc
OPENAPI_GEN ?= oapi-codegen

PG_DSN      ?= postgres://clg:clg@localhost:5432/clg?sslmode=disable
MIGRATIONS  := ./migrations

GOFLAGS     := -trimpath
LDFLAGS     := -s -w
PKGS        := ./...

##@ Help

.PHONY: help
help: ## Show this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage: make \033[36m<target>\033[0m\n"} \
		/^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } \
		/^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)

##@ Tooling

.PHONY: tools
tools: ## Install dev tools (Go binaries).
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.59.1
	$(GO) install github.com/securego/gosec/v2/cmd/gosec@v2.20.0
	$(GO) install github.com/pressly/goose/v3/cmd/goose@v3.20.0
	$(GO) install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.26.0
	$(GO) install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.3.0
	$(GO) install golang.org/x/vuln/cmd/govulncheck@latest

##@ Code generation

.PHONY: gen
gen: gen-sqlc gen-openapi ## Run all code generators.

.PHONY: gen-sqlc
gen-sqlc: ## Generate type-safe SQL bindings.
	$(SQLC) generate

.PHONY: gen-openapi
gen-openapi: ## Generate HTTP handlers and types from OpenAPI.
	$(OPENAPI_GEN) --config=api/oapi-codegen.yaml api/openapi.yaml

##@ Build

.PHONY: build
build: build-gateway build-worker ## Build all binaries.

.PHONY: build-gateway
build-gateway:
	$(GO) build $(GOFLAGS) -ldflags='$(LDFLAGS)' -o bin/gateway ./cmd/gateway

.PHONY: build-worker
build-worker:
	$(GO) build $(GOFLAGS) -ldflags='$(LDFLAGS)' -o bin/worker ./cmd/worker

##@ Run

.PHONY: up
up: ## Start full stack via docker-compose.
	docker compose -f deployments/docker-compose.yml --env-file .env up --build -d

.PHONY: down
down: ## Stop docker-compose.
	docker compose -f deployments/docker-compose.yml down -v

.PHONY: logs
logs: ## Tail logs.
	docker compose -f deployments/docker-compose.yml logs -f --tail=200

.PHONY: psql
psql: ## Open psql shell to local postgres.
	docker compose -f deployments/docker-compose.yml exec postgres psql -U clg -d clg

##@ Database

.PHONY: migrate-up
migrate-up: ## Apply DB migrations.
	$(GOOSE) -dir $(MIGRATIONS) postgres "$(PG_DSN)" up

.PHONY: migrate-down
migrate-down: ## Revert last DB migration.
	$(GOOSE) -dir $(MIGRATIONS) postgres "$(PG_DSN)" down

.PHONY: migrate-status
migrate-status:
	$(GOOSE) -dir $(MIGRATIONS) postgres "$(PG_DSN)" status

##@ Test

.PHONY: test
test: ## Run unit tests.
	$(GO) test -race -count=1 -timeout=120s $(PKGS)

.PHONY: test-cover
test-cover: ## Run unit tests with coverage.
	$(GO) test -race -count=1 -coverprofile=coverage.out -covermode=atomic $(PKGS)
	$(GO) tool cover -func=coverage.out | tail -n 1

.PHONY: test-integration
test-integration: ## Run integration tests (requires running compose).
	$(GO) test -tags=integration -race -count=1 -timeout=300s ./test/integration/...

##@ Lint & Security

.PHONY: check
check: fmt lint vet sec vuln gitleaks ## Full pre-PR check.

.PHONY: fmt
fmt:
	$(GO) fmt $(PKGS)
	gofmt -s -w .

.PHONY: lint
lint:
	$(GOLANGCI) run --timeout 5m

.PHONY: vet
vet:
	$(GO) vet $(PKGS)

.PHONY: sec
sec: ## Run gosec.
	$(GOSEC) -quiet -confidence medium -severity medium ./...

.PHONY: vuln
vuln: ## Check dependencies for known vulnerabilities.
	govulncheck $(PKGS)

.PHONY: gitleaks
gitleaks: ## Scan repo for secrets.
	$(GITLEAKS) detect --no-banner --redact --source=.

.PHONY: hadolint
hadolint: ## Lint all Dockerfiles.
	@find deployments -name 'Dockerfile*' -print0 | xargs -0 -I{} $(HADOLINT) {}

.PHONY: trivy
trivy: ## Scan filesystem for vulnerabilities.
	$(TRIVY) fs --severity HIGH,CRITICAL --exit-code 1 --no-progress .

##@ Seed

.PHONY: seed-pool
seed-pool: ## Seed the project pool from a CSV (PATH=...).
	$(GO) run ./cmd/seed --csv=$${PATH:-projects.csv}
