#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/trachuan-render-test.XXXXXX")
trap 'rm -rf "$work"' EXIT
shipped=$here/deploy-policy.json
python3 -I -c 'import json, sys
p = json.load(open(sys.argv[1]))
assert p["repositories"] == ["dntmakers/trachuan"]
p["repositories"] = ["dannyota/trachuan", "dntmakers/trachuan"]
p["images"]["dannyota/trachuan"] = {k: v.replace("ghcr.io/dntmakers/", "ghcr.io/dannyota/") for k, v in p["images"]["dntmakers/trachuan"].items()}
json.dump(p, open(sys.argv[2], "w"))' "$shipped" "$work/dual.json"
export DEPLOY_POLICY=$work/dual.json
pins=$work/images.env
digest=sha256:$(printf '%064d' 1)
cat >"$pins" <<PINS
SERVER_IMAGE=ghcr.io/dannyota/trachuan-server@$digest
WEB_IMAGE=ghcr.io/dannyota/trachuan-web@$digest
QUERYEMBED_IMAGE=ghcr.io/dannyota/trachuan-queryembed@$digest
CADDY_IMAGE=docker.io/library/caddy@$digest
POSTGRES_IMAGE=ghcr.io/dannyota/trachuan-postgres@$digest
PINS
bash "$here/render-units.sh" "$pins" "$work/units"
grep -qxF "Image=ghcr.io/dannyota/trachuan-server@$digest" "$work/units/trachuan-server.container"
grep -qxF "RequiresMountsFor=/srv/trachuan/data" "$work/units/trachuan-postgres.container"
grep -qxF "ExecStartPre=/usr/bin/mountpoint -q /srv/trachuan/data" "$work/units/trachuan-postgres.container"
grep -qxF "Volume=/srv/trachuan/data/postgres:/var/lib/postgresql:Z,U" "$work/units/trachuan-postgres.container"
grep -Fq 'archive_command=pgbackrest --stanza=trachuan archive-push %%p' "$work/units/trachuan-postgres.container"
[[ ! -e $work/units/trachuan-postgres.volume ]]
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
mkdir "$work/templates"
printf '[Container]\nImage=@UNKNOWN_IMAGE@\n' >"$work/templates/bad.container"
if QUADLET_TEMPLATES="$work/templates" bash "$here/render-units.sh" "$pins" "$work/units"; then
 echo 'accepted unknown template placeholder' >&2; exit 1
fi
[[ ! -e $work/units/bad.container ]]
sed 's|ghcr.io/dannyota/|ghcr.io/dntmakers/|' "$pins" >"$work/moved.env"
bash "$here/render-units.sh" "$work/moved.env" "$work/dual-moved"
grep -qxF "Image=ghcr.io/dntmakers/trachuan-server@$digest" "$work/dual-moved/trachuan-server.container"
DEPLOY_POLICY=$shipped bash "$here/render-units.sh" "$work/moved.env" "$work/moved"
grep -qxF "Image=ghcr.io/dntmakers/trachuan-server@$digest" "$work/moved/trachuan-server.container"
installed=$(python3 -I -B -c 'import sys; sys.path.insert(0, sys.argv[1]); from updater.deploy_store import Paths; print(Paths().policy)' "$here")
grep -qxF "policy=\${DEPLOY_POLICY:-$installed}" "$here/render-units.sh"
ln -s "$shipped" "$work/linked.json"
for name in server web queryembed postgres; do
 sed "s|ghcr.io/dannyota/trachuan-$name@|ghcr.io/dntmakers/trachuan-$name@|" "$pins" >"$work/mixed-$name.env"
done
sed 's|docker.io/library/caddy@|docker.io/library/other@|' "$pins" >"$work/mixed-caddy.env"
short=sha256:$(printf '%063d' 1) long=sha256:$(printf '%065d' 1) upper=sha256:$(printf 'A%063d' 1)
for bad in "$short" "$long" "$upper" "$digest:latest" "x$digest"; do
 index=$((${index:-0} + 1))
 sed "s|^SERVER_IMAGE=.*|SERVER_IMAGE=ghcr.io/dannyota/trachuan-server@$bad|" "$pins" >"$work/digest-$index.env"
done
for attack in mixed-server mixed-web mixed-queryembed mixed-postgres mixed-caddy narrowed missing-policy linked-policy \
 digest-1 digest-2 digest-3 digest-4 digest-5; do
 case $attack in
  mixed-*|digest-*) policy=$DEPLOY_POLICY env=$work/$attack.env ;;
  narrowed) policy=$shipped env=$pins ;;
  missing-policy) policy=$work/absent.json env=$pins ;;
  linked-policy) policy=$work/linked.json env=$pins ;;
 esac
 if DEPLOY_POLICY=$policy bash "$here/render-units.sh" "$env" "$work/units"; then
  echo "accepted $attack" >&2; exit 1
 fi
 cmp "$work/before" "$work/units/trachuan-server.container"
done
echo 'render-units: all checks passed'
