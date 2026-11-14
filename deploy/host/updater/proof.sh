#!/usr/bin/env bash
set -euo pipefail
module_root=$(readlink -f /usr/local/libexec/trachuan/updater)
exec /usr/bin/python3 -I "$module_root/proof.py" "$@"
