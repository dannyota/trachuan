#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
[[ $# == 2 || $# == 3 ]] || { echo 'usage: render-units.sh images.env destination [user-unit-destination]' >&2; exit 1; }
pins=$1
destination=$2
user_destination=${3:-$destination}
templates=${QUADLET_TEMPLATES:-$here/quadlet}
fail() { echo "render-units: $*" >&2; exit 1; }
[[ -f $pins && ! -L $pins ]] || fail 'expected a regular images.env'
for key in SERVER_IMAGE WEB_IMAGE QUERYEMBED_IMAGE CADDY_IMAGE POSTGRES_IMAGE; do
 printf -v "$key" '%s' ''
done
while IFS= read -r line || [[ -n $line ]]; do
 [[ -n $line && $line != \#* ]] || continue
 [[ $line == *=* ]] || fail 'invalid image assignment'
 key=${line%%=*}
 value=${line#*=}
 case $key in
  SERVER_IMAGE) repository=ghcr.io/dannyota/trachuan-server ;;
  WEB_IMAGE) repository=ghcr.io/dannyota/trachuan-web ;;
  QUERYEMBED_IMAGE) repository=ghcr.io/dannyota/trachuan-queryembed ;;
  CADDY_IMAGE) repository=docker.io/library/caddy ;;
  POSTGRES_IMAGE) repository=ghcr.io/dannyota/trachuan-postgres ;;
  *) fail 'unknown image key' ;;
 esac
 [[ -z ${!key} ]] || fail "duplicate $key"
 [[ $value == "$repository"@* && ${value#*@} =~ ^sha256:[0-9a-f]{64}$ ]] || fail "invalid digest reference for $key"
 printf -v "$key" '%s' "$value"
done <"$pins"
for key in SERVER_IMAGE WEB_IMAGE QUERYEMBED_IMAGE CADDY_IMAGE POSTGRES_IMAGE; do
 [[ -n ${!key} ]] || fail "missing $key"
done
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
