#!/usr/bin/env bash
# Static checks before every push. No tests, no builds, no network.
set -Eeuo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
bash scripts/check-lengths.sh
if command -v gofmt >/dev/null && git ls-files -- '*.go' | grep -q .; then
  unformatted=$(gofmt -l $(git ls-files -- '*.go'))
  [[ -z "$unformatted" ]] || { echo "gofmt: $unformatted" >&2; exit 1; }
fi
for mod in $(git ls-files -- '**/go.mod' 'go.mod' 2>/dev/null); do
  dir=$(dirname "$mod")
  if command -v golangci-lint >/dev/null; then (cd "$dir" && golangci-lint run ./...); fi
done
if command -v npx >/dev/null && [[ -f package.json ]]; then
  npx prettier --check "**/*.md" "!**/AGENTS.md" "!**/CLAUDE.md" "!instructions/**" "!docs/plans/**" "!.claude/**" "!.codex/**" "!**/node_modules/**"
  npx markdownlint-cli2 "**/*.md" "#**/node_modules" "#**/AGENTS.md" "#**/CLAUDE.md" "#instructions" "#docs/plans" "#.claude" "#.codex"
fi
if command -v shellcheck >/dev/null; then shellcheck $(git ls-files -- '*.sh' '.githooks/*'); fi
echo "pre-push: ok"
