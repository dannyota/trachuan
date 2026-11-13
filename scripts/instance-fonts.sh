#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
uvx --from 'fonttools[woff]==4.60.1' python "$script_dir/instance_fonts.py"
