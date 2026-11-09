.DEFAULT_GOAL := help
SHELL := /usr/bin/env bash

help: ## List targets
	@grep -E '^[a-zA-Z_-]+:.*## ' $(firstword $(MAKEFILE_LIST)) | awk 'BEGIN{FS=":.*## "}{printf "  %-16s %s\n", $$1, $$2}'

hooks-install: ## Point git at .githooks (gitleaks and length checks on commit)
	git config core.hooksPath .githooks

lengths-check: ## Fail on Markdown over 300 lines or code over 400
	bash scripts/check-lengths.sh

pre-push: ## Static checks before pushing
	bash scripts/pre-push.sh

docs-fmt: ## Format human-read Markdown with Prettier
	npx prettier --write "**/*.md" "!**/AGENTS.md" "!**/CLAUDE.md" "!instructions/**" "!docs/plans/**" "!.claude/**" "!.codex/**" "!**/node_modules/**" "!**/.venv/**"

test: ## Run unit tests
	go test ./...

# Local runs reuse the compose password for pgx; CI sets the password in its URL.
LOAD_ENV := set -a; if [ -f .env ]; then . ./.env; fi; set +a; \
	if [ -z "$$PGPASSWORD" ] && [ -n "$$POSTGRES_PASSWORD" ]; then export PGPASSWORD="$$POSTGRES_PASSWORD"; fi

test-integration: ## Run integration tests (loads .env when present)
	$(LOAD_ENV); go test -tags integration -count=1 ./...

test-proof: ## Run proof tests (loads .env when present)
	@if [ ! -d apps/corpus/test/proof ]; then echo "test-proof: no proof tests yet"; exit 0; fi; \
	$(LOAD_ENV); go test -tags 'integration faultinject' -count=1 ./apps/corpus/test/proof/...

generate: ## Generate the typed data layer when sqlc is configured
	@if [ -f apps/corpus/sqlc.yaml ]; then cd apps/corpus && sqlc generate; fi

sqlc-check: ## Fail when generated data layer files drift
	@if [ -f apps/corpus/sqlc.yaml ]; then \
	  cd apps/corpus && sqlc generate && \
	  { [ -z "$$(git status --porcelain -- internal/store)" ] || \
	    { echo "generated data layer drifts; run make generate" >&2; \
	      git status --porcelain -- internal/store; exit 1; }; }; \
	fi

COMPOSE := podman compose --env-file .env -f deploy/compose.yml

db-up: ## Start local PostgreSQL with pgvector and wait until it is ready
	$(COMPOSE) up -d postgres
	@for i in $$(seq 1 60); do podman exec traquydinh_postgres_1 pg_isready -q -U traquydinh && exit 0; sleep 1; done; echo "postgres not ready" >&2; exit 1

db-setup: ## Create the pipeline and Temporal roles and databases (idempotent)
	bash deploy/local/db-setup.sh

temporal-schema: ## Create or upgrade the Temporal schemas (idempotent)
	bash deploy/local/temporal-schema.sh

temporal-up: ## Start the Temporal server and UI
	$(COMPOSE) up -d temporal temporal-ui

temporal-namespace: ## Create the corpus namespace when missing
	bash deploy/local/temporal-namespace.sh

services-up: db-up db-setup temporal-schema temporal-up temporal-namespace ## Start every local pipeline service in order

db-down: ## Stop local services, keep their volumes
	$(COMPOSE) down

.PHONY: help hooks-install lengths-check pre-push docs-fmt test test-integration test-proof generate sqlc-check db-up db-setup temporal-schema temporal-up temporal-namespace services-up db-down
