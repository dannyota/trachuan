#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
[[ $# == 2 || $# == 3 ]] || { echo 'usage: render-units.sh images.env destination [user-unit-destination]' >&2; exit 1; }
pins=$1
destination=$2
user_destination=${3:-$destination}
templates=${QUADLET_TEMPLATES:-$here/quadlet}
policy=${DEPLOY_POLICY:-/etc/trachuan/deploy-policy.json}
fail() { echo "render-units: $*" >&2; exit 1; }
[[ -f $pins && ! -L $pins ]] || fail 'expected a regular images.env'
[[ -f $policy && ! -L $policy ]] || fail 'expected a regular deploy policy'
names=$(python3 -I -c 'import json, sys
policy = json.load(open(sys.argv[1], "rb"))
for repository in policy["repositories"]:
    images = policy["images"][repository]
    print(images["server"], images["web"], images["queryembed"], policy["upstream"]["caddy"], images["postgres"])' "$policy") ||
 fail 'unreadable deploy policy'
for key in SERVER_IMAGE WEB_IMAGE QUERYEMBED_IMAGE CADDY_IMAGE POSTGRES_IMAGE; do
 printf -v "$key" '%s' ''
done
while IFS= read -r line || [[ -n $line ]]; do
 [[ -n $line && $line != \#* ]] || continue
 [[ $line == *=* ]] || fail 'invalid image assignment'
 key=${line%%=*}
 value=${line#*=}
 case $key in
  SERVER_IMAGE|WEB_IMAGE|QUERYEMBED_IMAGE|CADDY_IMAGE|POSTGRES_IMAGE) ;;
  *) fail 'unknown image key' ;;
 esac
 [[ -z ${!key} ]] || fail "duplicate $key"
 [[ ${value#*@} =~ ^sha256:[0-9a-f]{64}$ ]] || fail "invalid digest reference for $key"
 printf -v "$key" '%s' "$value"
done <"$pins"
for key in SERVER_IMAGE WEB_IMAGE QUERYEMBED_IMAGE CADDY_IMAGE POSTGRES_IMAGE; do
 [[ -n ${!key} ]] || fail "missing $key"
done
listed=false
while read -r server web queryembed caddy postgres; do
 if [[ ${SERVER_IMAGE%@*} == "$server" && ${WEB_IMAGE%@*} == "$web" && ${QUERYEMBED_IMAGE%@*} == "$queryembed" &&
  ${CADDY_IMAGE%@*} == "$caddy" && ${POSTGRES_IMAGE%@*} == "$postgres" ]]; then
  listed=true
 fi
done <<<"$names"
[[ $listed == true ]] || fail 'images do not belong to one policy repository'
files=() contents=()
for file in "$templates"/*.container "$templates"/*.volume "$here"/systemd/*.service "$here"/systemd/*.timer; do
 [[ -e $file ]] || continue
 [[ -f $file && ! -L $file ]] || fail 'unsafe template'
 content=$(cat "$file")
 for key in SERVER_IMAGE WEB_IMAGE QUERYEMBED_IMAGE CADDY_IMAGE POSTGRES_IMAGE; do
  content=${content//@$key@/${!key}}
 done
 [[ ! $content =~ @[A-Z_]+@ ]] || fail 'unknown template placeholder'
 case $file in
  *.service|*.timer) target=$user_destination/${file##*/} ;;
  *) target=$destination/${file##*/} ;;
 esac
 [[ ! -L $target && ! -d $target ]] || fail 'unsafe destination file'
 files+=("$target")
 contents+=("$content")
done
((${#files[@]} > 0)) || fail 'no templates found'
[[ ! -L $destination && ! -L $user_destination ]] || fail 'destination must not be a symlink'
for obsolete in trachuan.network trachuan-db.network; do
 [[ ! -L $destination/$obsolete && ! -d $destination/$obsolete ]] || fail 'unsafe obsolete network unit'
done
mkdir -p "$destination" "$user_destination"
staged=
trap 'if [[ -n $staged ]]; then rm -f "$staged"; fi' EXIT
for ((index=0; index<${#files[@]}; index++)); do
 staged=$(mktemp "$(dirname "${files[index]}")/.unit.XXXXXX")
 printf '%s\n' "${contents[index]}" >"$staged"
 chmod 0644 "$staged"
 mv -f "$staged" "${files[index]}"
 staged=
done

rm -f "$destination/trachuan.network" "$destination/trachuan-db.network"
