.DEFAULT_GOAL := help
SHELL := /usr/bin/env bash

help: ## List targets
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*## "}{printf "  %-16s %s\n", $$1, $$2}'

hooks-install: ## Point git at .githooks (gitleaks and length checks on commit)
	git config core.hooksPath .githooks

lengths-check: ## Fail on Markdown over 300 lines or code over 400
	bash scripts/check-lengths.sh

pre-push: ## Static checks before pushing
	bash scripts/pre-push.sh

docs-fmt: ## Format human-read Markdown with Prettier
	npx prettier --write "**/*.md" "!**/AGENTS.md" "!**/CLAUDE.md" "!instructions/**" "!docs/plans/**" "!.claude/**" "!.codex/**" "!**/node_modules/**"

test: ## Unit tests (added with the first Go module)
	@echo "no modules yet"

.PHONY: help hooks-install lengths-check pre-push docs-fmt test
