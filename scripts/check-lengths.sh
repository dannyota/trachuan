#!/usr/bin/env bash
# Fails when a tracked Markdown doc passes MD_MAX lines or a non-test source
# file passes CODE_MAX lines. Split an oversize file by topic (Go: a sibling
# file in the same package; docs: a focused page) or trim it; git keeps history.
# It reads working-tree files, so the pre-commit hook checks the whole tree,
# not only staged content.
set -Eeuo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT"

MD_MAX=300
CODE_MAX=400

# Generated paths without a standard header.
generated='^(apps/web/app/api/generated/|packages/schema/gen/)|generated\.[a-z]+$|\.sql\.go$'
tests='(_test\.(go|sh)|[-.]test\.sh|\.test\.[cm]?[jt]s|\.spec\.[cm]?[jt]s)$|(^|/)(test|tests|testdata)/'

violations=0
check() {
  local max=$1 file lines
  while IFS= read -r file; do
    [[ -f "$file" ]] || continue
    # Generated files follow their generator, not authored size.
    head -n 5 "$file" | grep -Eq 'Code generated .* DO NOT EDIT|@generated' && continue
    lines=$(wc -l <"$file")
    if ((lines > max)); then
      echo "too long: $file has $lines lines (max $max)"
      violations=$((violations + 1))
    fi
  done
}

check "$MD_MAX" < <(git ls-files -- '*.md')
check "$CODE_MAX" < <(
  git ls-files -- '*.go' '*.ts' '*.vue' '*.mjs' '*.js' '*.sh' |
    grep -Ev "$generated" | grep -Ev "$tests" || true
)

if ((violations > 0)); then
  echo "check-lengths: $violations file(s) over the limit" >&2
  exit 1
fi
echo "check-lengths: ok (docs <= $MD_MAX, code <= $CODE_MAX lines)"
