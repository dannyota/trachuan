#!/usr/bin/env bash
# Exercise the installer with verified local archives and no network access.
set -Eeuo pipefail

source_repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT
mkdir -p "$scratch/bin" "$scratch/payload"
cat > "$scratch/bin/curl" <<'EOF'
#!/usr/bin/env bash
echo 'network forbidden in installer tests' >&2
exit 99
EOF
chmod +x "$scratch/bin/curl"
real_unzip=$(command -v unzip)
cat > "$scratch/bin/unzip" <<EOF
#!/usr/bin/env bash
echo "\$*" >> "\$UNZIP_LOG"
exec "$real_unzip" "\$@"
EOF
chmod +x "$scratch/bin/unzip"
export PATH="$scratch/bin:$PATH"
printf 'native fixture\n' > "$scratch/payload/library"
tar -czf "$scratch/fixture.tgz" -C "$scratch/payload" library
checksum() {
  if command -v shasum >/dev/null; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    sha256sum "$1" | awk '{print $1}'
  fi
}
sha256=$(checksum "$scratch/fixture.tgz")
mkdir -p "$scratch/zip/runtimes/native"
printf 'zip fixture\n' > "$scratch/zip/runtimes/native/library"
(cd "$scratch/zip" && zip -q -r "$scratch/fixture.nupkg" runtimes)
zip_sha256=$(checksum "$scratch/fixture.nupkg")
mkdir -p "$scratch/empty/runtimes/other"
printf 'other\n' > "$scratch/empty/runtimes/other/file"
(cd "$scratch/empty" && zip -q -r "$scratch/nomember.nupkg" runtimes)
nomember_sha256=$(checksum "$scratch/nomember.nupkg")

failures=0
for platform in darwin-arm64 linux-arm64; do
  for scenario in safe work native cache platform dangling-platform \
    first-directory second-directory directory-symlink \
    archive-directory archive-symlink first-archive-symlink; do
    root="$scratch/$platform-$scenario"
    repo="$root/repo"
    outside="$root/outside"
    mkdir -p "$repo/scripts" "$repo/apps/queryembed" "$outside"
    cp "$source_repo/scripts/queryembed-native.sh" "$repo/scripts/"
    jq -n --arg platform "$platform" --arg sha256 "$sha256" '
      {version: 1, assets: ["first", "second"] | map({
        platform: $platform, url: ("https://invalid.example/" + . + ".tgz"),
        sha256: $sha256, member: "library", install: (. + ".a")
      })}' > "$repo/apps/queryembed/native.lock"

    case "$scenario" in
      work) link=work ;;
      native) link=work/native ;;
      cache) link=work/native/cache ;;
      platform|dangling-platform) link="work/native/$platform" ;;
      *) link= ;;
    esac
    if [[ -n "$link" ]]; then
      mkdir -p "$repo/$(dirname "$link")"
      ln -s "$outside/linked" "$repo/$link"
    fi
    if [[ "$scenario" != dangling-platform ]]; then
      [[ -z "$link" ]] || mkdir -p "$outside/linked"
      mkdir -p "$repo/work/native/$platform"
    fi
    cache="$repo/work/native/cache"
    mkdir -p "$cache"
    cp "$scratch/fixture.tgz" "$cache/first.tgz"
    cp "$scratch/fixture.tgz" "$cache/second.tgz"
    case "$scenario" in
      archive-directory) rm "$cache/second.tgz"; mkdir "$cache/second.tgz" ;;
      archive-symlink) rm "$cache/second.tgz"; ln -s "$outside" "$cache/second.tgz" ;;
      first-archive-symlink) rm "$cache/first.tgz"; ln -s "$outside" "$cache/first.tgz" ;;
    esac
    destination="$repo/work/native/$platform"
    case "$scenario" in
      first-directory) mkdir "$destination/first.a" ;;
      second-directory) mkdir "$destination/second.a" ;;
      directory-symlink) ln -s "$outside" "$destination/second.a" ;;
    esac
    before=$(find "$repo" "$outside" -print | sort)
    status=0
    output=$(bash "$repo/scripts/queryembed-native.sh" "$platform" 2>&1) || status=$?
    if [[ "$scenario" == safe ]]; then
      if [[ "$status" != 0 ]] || ! cmp -s "$scratch/payload/library" "$destination/first.a" ||
        ! cmp -s "$scratch/payload/library" "$destination/second.a"; then
        echo "FAIL $platform/$scenario: valid cache did not install: $output" >&2
        failures=$((failures + 1))
      fi
    elif [[ "$status" == 0 ]] || [[ "$before" != "$(find "$repo" "$outside" -print | sort)" ]] ||
      [[ "$output" != *symlink* && "$output" != *directory* ]]; then
      echo "FAIL $platform/$scenario: expected rejection before writes (status $status): $output" >&2
      failures=$((failures + 1))
    fi
  done
done
for platform in darwin-arm64 linux-arm64; do
  for scenario in cached corrupt nomember glob; do
    repo="$scratch/$platform-zip-$scenario/repo"
    mkdir -p "$repo/scripts" "$repo/apps/queryembed" "$repo/work/native/cache"
    cp "$source_repo/scripts/queryembed-native.sh" "$repo/scripts/"
    zip_member=runtimes/native/library
    zip_pin=$zip_sha256
    zip_file=$scratch/fixture.nupkg
    case "$scenario" in
      nomember) zip_pin=$nomember_sha256; zip_file=$scratch/nomember.nupkg ;;
      glob) zip_member='runtimes/native/*' ;;
    esac
    jq -n --arg platform "$platform" --arg tgz "$sha256" --arg zip "$zip_pin" --arg member "$zip_member" '
      {version: 1, assets: [
        {platform: $platform, url: "https://invalid.example/first.tgz", sha256: $tgz,
         member: "library", install: "first.a"},
        {platform: $platform, url: "https://invalid.example/second.nupkg", sha256: $zip,
         member: $member, install: "second.a"}]}' > "$repo/apps/queryembed/native.lock"
    cp "$scratch/fixture.tgz" "$repo/work/native/cache/first.tgz"
    cp "$zip_file" "$repo/work/native/cache/second.nupkg"
    [[ "$scenario" != corrupt ]] || printf 'x' >> "$repo/work/native/cache/second.nupkg"
    export UNZIP_LOG="$scratch/$platform-zip-$scenario.log"
    : > "$UNZIP_LOG"
    status=0
    output=$(bash "$repo/scripts/queryembed-native.sh" "$platform" 2>&1) || status=$?
    destination="$repo/work/native/$platform"
    if [[ "$scenario" == cached ]]; then
      if [[ "$status" != 0 ]] || ! cmp -s "$scratch/payload/library" "$destination/first.a" ||
        ! cmp -s "$scratch/zip/runtimes/native/library" "$destination/second.a" ||
        ! grep -q 'second.nupkg' "$UNZIP_LOG"; then
        echo "FAIL $platform/zip-$scenario: zip asset did not install through unzip: $output" >&2
        failures=$((failures + 1))
      fi
    elif [[ "$status" == 0 || -e "$destination/second.a" ]] ||
      [[ -n "$(find "$destination" -name '.install.*' -print 2>/dev/null)" ]]; then
      echo "FAIL $platform/zip-$scenario: bad zip asset was accepted or left files: $output" >&2
      failures=$((failures + 1))
    elif [[ "$scenario" == nomember && ! -s "$UNZIP_LOG" ]]; then
      echo "FAIL $platform/zip-$scenario: unzip was not reached" >&2
      failures=$((failures + 1))
    elif [[ "$scenario" == glob && ( -s "$UNZIP_LOG" || "$output" != *glob* ) ]]; then
      echo "FAIL $platform/zip-$scenario: glob member not rejected before extraction: $output" >&2
      failures=$((failures + 1))
    fi
  done
done
if [[ "$failures" != 0 ]]; then
  echo "$failures installer cases failed" >&2
  exit 1
fi
echo 'All installer cases passed'
