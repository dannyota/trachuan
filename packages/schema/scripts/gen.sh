#!/usr/bin/env bash
set -Eeuo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
./node_modules/.bin/openapi-typescript openapi.yaml -o gen/openapi.ts
