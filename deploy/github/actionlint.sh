#!/usr/bin/env bash
set -euo pipefail
directory=$(mktemp -d)
trap 'rm -rf "$directory"' EXIT
curl -fsSL --retry 3 -o "$directory/actionlint.tar.gz" https://github.com/rhysd/actionlint/releases/download/v1.7.12/actionlint_1.7.12_linux_amd64.tar.gz
printf '%s  %s\n' 8aca8db96f1b94770f1b0d72b6dddcb1ebb8123cb3712530b08cc387b349a3d8 "$directory/actionlint.tar.gz" | sha256sum --check --strict
tar -xzf "$directory/actionlint.tar.gz" -C "$directory" actionlint
"$directory/actionlint" .github/workflows/*.yml
