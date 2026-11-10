#!/usr/bin/env bash
# Install the pinned native libraries, reusing verified archives offline.
set -Eeuo pipefail

repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
platform=${1:?usage: queryembed-native.sh <darwin-arm64|linux-arm64>}
case "$platform" in
  darwin-arm64|linux-arm64) ;;
  *) echo "unsupported native platform: $platform" >&2; exit 1 ;;
esac

checksum() {
  if command -v shasum >/dev/null; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    sha256sum "$1" | awk '{print $1}'
  fi
}

assets=$(jq -er --arg platform "$platform" '
  .assets | map(select(.platform == $platform)) |
  if length == 2 then
    .[] | [.url, .sha256, .member, .install] | @tsv
  else error("native.lock must contain two assets per platform") end
' "$repo/apps/queryembed/native.lock")
cache="$repo/work/native/cache"
destination="$repo/work/native/$platform"
for directory in "$repo/work" "$repo/work/native" "$cache" "$destination"; do
  if [[ -L "$directory" ]]; then
    echo "native directory must not be a symlink: $directory" >&2
    exit 1
  fi
done
# Check every archive and install target before writing anything: each must
# be absent or a regular file, never a directory or a symlink that mv follows.
while IFS=$'\t' read -r url _ _ install; do
  for target in "$cache/${url##*/}" "$destination/$install"; do
    if [[ -L "$target" || -d "$target" ]]; then
      echo "native target must not be a directory or symlink: $target" >&2
      exit 1
    fi
  done
done <<< "$assets"
mkdir -p "$cache" "$destination"
staging=
trap 'if [[ -n "$staging" ]]; then rm -f "$staging"; fi' EXIT
while IFS=$'\t' read -r url sha256 member install; do
  archive="$cache/${url##*/}"
  if [[ ! -f "$archive" ]] || [[ "$(checksum "$archive")" != "$sha256" ]]; then
    staging=$(mktemp "$cache/.download.XXXXXX")
    curl --fail --location --retry 3 --output "$staging" "$url"
    if [[ "$(checksum "$staging")" != "$sha256" ]]; then
      echo "native archive checksum mismatch: ${url##*/}" >&2
      exit 1
    fi
    mv "$staging" "$archive"
    staging=
  fi
  staging=$(mktemp "$destination/.install.XXXXXX")
  tar -xOf "$archive" "$member" > "$staging"
  chmod 644 "$staging"
  mv "$staging" "$destination/$install"
  staging=
done <<< "$assets"
