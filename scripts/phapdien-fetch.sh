#!/usr/bin/env bash
set -euo pipefail

url='https://raw.githubusercontent.com/definev/phapdien/ef75c3edb63fa52d979ad55cec7cbe37d6944ced/data/BoPhapDienDienTu.zip'
expected='dc2fc7d14e208b551e3e7667e911dcfd24d8f246a7e24d3d24ad7507cd4b5ff5'
dir='work/phapdien'
tmp=''
cleanup() { if [[ -n "$tmp" ]]; then rm -f "$tmp"; fi; }
trap cleanup EXIT
fail() { printf 'error: %s\n' "$1" >&2; exit 1; }
mkdir -p "$dir" || fail 'cannot create destination'
tmp=$(mktemp "$dir/.phapdien.XXXXXXXX") || fail 'cannot create temporary file'
curl -fsSL "$url" -o "$tmp" 2>/dev/null || fail 'download failed'
size=$(wc -c < "$tmp" | tr -d '[:space:]')
[[ "$size" == '33893115' ]] || fail 'size mismatch'
if command -v sha256sum >/dev/null 2>&1; then
  digest=$(sha256sum "$tmp")
else
  digest=$(shasum -a 256 "$tmp")
fi
[[ "${digest%% *}" == "$expected" ]] || fail 'SHA-256 mismatch'
mv -f "$tmp" "$dir/BoPhapDienDienTu.zip" || fail 'cannot move download'
tmp=''
printf 'ok\n'
