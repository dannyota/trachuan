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

EMBEDDER_MODEL_DIR ?= work/models/vi_v2-bundle

queryembed-native: ## Install pinned native libraries for the host
	@platform="$$(uname -s | tr '[:upper:]' '[:lower:]')-$$(uname -m)"; \
	case "$$platform" in darwin-arm64|linux-arm64) ;; linux-aarch64) platform=linux-arm64 ;; \
	  *) echo "unsupported native platform: $$platform" >&2; exit 1 ;; esac; \
	bash scripts/queryembed-native.sh "$$platform"

queryembed-test-ort: queryembed-native ## Run native query embedder tests with the race detector
	@platform="$$(uname -s | tr '[:upper:]' '[:lower:]')-$$(uname -m)"; \
	case "$$platform" in darwin-arm64) library=libonnxruntime.dylib ;; \
	  linux-arm64|linux-aarch64) platform=linux-arm64; library=libonnxruntime.so ;; \
	  *) echo "unsupported native platform: $$platform" >&2; exit 1 ;; esac; \
	CGO_LDFLAGS="-L$(CURDIR)/work/native/$$platform $${CGO_LDFLAGS:-}" \
	QUERYEMBED_ORT_LIB="$(CURDIR)/work/native/$$platform/$$library" \
	go test -race -tags ort ./apps/queryembed/...

queryembed-run: ## Start the native query service on 127.0.0.1:8091
	@platform="$$(uname -s | tr '[:upper:]' '[:lower:]')-$$(uname -m)"; \
	case "$$platform" in darwin-arm64) library=libonnxruntime.dylib ;; \
	  linux-arm64|linux-aarch64) platform=linux-arm64; library=libonnxruntime.so ;; \
	  *) echo "unsupported native platform: $$platform" >&2; exit 1 ;; esac; \
	CGO_LDFLAGS="-L$(CURDIR)/work/native/$$platform $${CGO_LDFLAGS:-}" \
	go run -tags ort ./apps/queryembed/cmd/queryembed --model-dir "$(EMBEDDER_MODEL_DIR)" \
	  --ort-lib "$(CURDIR)/work/native/$$platform/$$library"

docembed-test: ## Run document embedder tests
	cd apps/docembed && uv run --frozen pytest

docembed-run: ## Start the document embedder on 127.0.0.1:8092 (MPS)
	cd apps/docembed && uv run --frozen --group document python -m trachuan_docembed --model-dir $(abspath $(EMBEDDER_MODEL_DIR))

server-run: ## Start the local server
	go run ./apps/server/cmd/server serve

test: ## Run unit tests
	go test ./...

# Local runs reuse the native PostgreSQL password from .env for pgx; CI sets the password in its URL.
LOAD_ENV := set -a; if [ -f .env ]; then . ./.env; fi; set +a; \
	if [ -z "$$PGPASSWORD" ] && [ -n "$$POSTGRES_PASSWORD" ]; then export PGPASSWORD="$$POSTGRES_PASSWORD"; fi

test-integration: ## Run integration tests (loads .env when present)
	$(LOAD_ENV); go test -tags integration -count=1 ./...

e2e: ## Run end-to-end flow tests against the local stack with the fake Google provider (loads .env when present)
	bash scripts/e2e.sh

test-proof: ## Run proof tests (loads .env when present)
	@if [ ! -d apps/corpus/test/proof ]; then echo "test-proof: no proof tests yet"; exit 0; fi; \
	$(LOAD_ENV); go test -tags 'integration faultinject' -count=1 ./apps/corpus/test/proof/...

generate: ## Generate the typed data layer when sqlc is configured
	@for app in corpus server; do \
	  if [ -f apps/$$app/sqlc.yaml ]; then (cd apps/$$app && sqlc generate) || exit $$?; fi; \
	done

schema-gen: ## Generate the web API client
	bash packages/schema/scripts/gen.sh

schema-check: ## Fail when the web API client drifts
	@set -e; before="$$(shasum -a 256 packages/schema/gen/openapi.ts)"; \
	  bash packages/schema/scripts/gen.sh; \
	  after="$$(shasum -a 256 packages/schema/gen/openapi.ts)"; \
	  [ "$$before" = "$$after" ] || { echo "generated schema drifts; run make schema-gen" >&2; exit 1; }; \
	  awk '/TokenInput: \{/ {inside=1; next} inside && /^        };/ {exit} inside {print}' packages/schema/gen/openapi.ts | grep -Eq 'days\?: number \| null;'

sqlc-check: ## Fail when generated data layer files drift
	@for app in corpus server; do \
	  if [ -f apps/$$app/sqlc.yaml ]; then \
	    (cd apps/$$app && sqlc generate && \
	      { [ -z "$$(git status --porcelain -- internal/store)" ] || \
	        { echo "generated data layer drifts; run make generate" >&2; \
	          git status --porcelain -- internal/store; exit 1; }; }) || exit $$?; \
	  fi; \
	done

db-up: ## Start native PostgreSQL 18 on 127.0.0.1:25433 (initializes ~/.local/share/trachuan-pg18 once)
	bash deploy/local/postgres.sh up

db-setup: ## Create the pipeline and builder roles, the workshop database and the corpus template (idempotent)
	bash deploy/local/db-setup.sh

temporal-up: ## Start the native Temporal dev server on 127.0.0.1:27233 with the corpus namespace (UI on 28233)
	bash deploy/local/temporal.sh up

services-up: db-up db-setup temporal-up ## Start every local pipeline service in order

services-down: ## Stop the local Temporal server and PostgreSQL, keep their data
	bash deploy/local/temporal.sh down
	bash deploy/local/postgres.sh down

.PHONY: e2e schema-gen schema-check server-run help hooks-install lengths-check pre-push docs-fmt queryembed-native queryembed-test-ort queryembed-run docembed-test docembed-run test test-integration test-proof generate sqlc-check db-up db-setup temporal-up services-up services-down
