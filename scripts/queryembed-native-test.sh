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
export PATH="$scratch/bin:$PATH"
printf 'native fixture\n' > "$scratch/payload/library"
tar -czf "$scratch/fixture.tgz" -C "$scratch/payload" library
if command -v shasum >/dev/null; then
  sha256=$(shasum -a 256 "$scratch/fixture.tgz" | awk '{print $1}')
else
  sha256=$(sha256sum "$scratch/fixture.tgz" | awk '{print $1}')
fi

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
if [[ "$failures" != 0 ]]; then
  echo "$failures installer cases failed" >&2
  exit 1
fi
echo 'All installer cases passed'
