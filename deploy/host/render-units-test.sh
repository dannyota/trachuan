#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/trachuan-render-test.XXXXXX")
trap 'rm -rf "$work"' EXIT
pins=$work/images.env
digest=sha256:$(printf '%064d' 1)
cat >"$pins" <<PINS
SERVER_IMAGE=ghcr.io/dannyota/trachuan-server@$digest
WEB_IMAGE=ghcr.io/dannyota/trachuan-web@$digest
QUERYEMBED_IMAGE=ghcr.io/dannyota/trachuan-queryembed@$digest
CADDY_IMAGE=docker.io/library/caddy@$digest
POSTGRES_IMAGE=docker.io/pgvector/pgvector@$digest
PINS
bash "$here/render-units.sh" "$pins" "$work/units"
# The consumer needs literal digest references, never environment expansion.
grep -qxF "Image=ghcr.io/dannyota/trachuan-server@$digest" "$work/units/trachuan-server.container"
if grep -RE '@[A-Z_]+@' "$work/units"; then exit 1; fi
cp "$work/units/trachuan-server.container" "$work/before"
for attack in tag duplicate missing repository shell; do
 cp "$pins" "$work/bad.env"
 case $attack in
  tag) sed 's|trachuan-server@.*|trachuan-server:latest|' "$pins" >"$work/bad.env" ;;
  duplicate) head -1 "$pins" >>"$work/bad.env" ;;
  missing) sed '/^SERVER_IMAGE=/d' "$pins" >"$work/bad.env" ;;
  repository) sed 's|dannyota/trachuan-server|attacker/trachuan-server|' "$pins" >"$work/bad.env" ;;
  shell) printf '\nEVIL=%s(touch %s)\n' '$' "$work/executed" >>"$work/bad.env" ;;
 esac
 if bash "$here/render-units.sh" "$work/bad.env" "$work/units"; then
  echo "accepted $attack" >&2; exit 1
 fi
 cmp "$work/before" "$work/units/trachuan-server.container"
 [[ ! -e $work/executed ]]
done
# Refuse a template that would produce a unit with an unresolved image.
mkdir "$work/templates"
printf '[Container]\nImage=@UNKNOWN_IMAGE@\n' >"$work/templates/bad.container"
if QUADLET_TEMPLATES="$work/templates" bash "$here/render-units.sh" "$pins" "$work/units"; then
 echo 'accepted unknown template placeholder' >&2; exit 1
fi
[[ ! -e $work/units/bad.container ]]
echo 'render-units: all checks passed'
