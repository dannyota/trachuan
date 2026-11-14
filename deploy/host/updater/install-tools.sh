#!/usr/bin/env bash
set -euo pipefail
[[ $# == 1 && $(uname -s) == Linux && $(uname -m) == aarch64 ]]
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
target=$1
[[ ! -L $target ]]
cosign_sha=ade83b7aac62e131cacfe96d958948b2150cc9efec46a570a2649b8e27dedad2
oras_sha=e61243d2c862a5aaf6451914edd8cc089ae91df425201fa9ee7659cbed36e7c0
archive_sha=b84269e78dfc2d5329a4aab373da3200de95e6c138338cf6c66c10c0a3f248c1
matches() {
 [[ -f $1 && ! -L $1 ]] && printf '%s  %s\n' "$2" "$1" | sha256sum --check --strict >/dev/null 2>&1
}
need_cosign=true
need_oras=true
if [[ -x $target/cosign ]] && matches "$target/cosign" "$cosign_sha"; then need_cosign=false; fi
if [[ -x $target/oras ]] && matches "$target/oras" "$oras_sha"; then need_oras=false; fi
if [[ $need_cosign == false && $need_oras == false ]]; then exit 0; fi
install -d -m 0755 "$target"
target=$(cd "$target" && pwd)
directory=$(mktemp -d)
trap 'rm -rf "$directory"' EXIT
cd "$directory"
if [[ $need_cosign == true ]]; then
 curl -fsSL --retry 3 -o cosign https://github.com/sigstore/cosign/releases/download/v3.1.4/cosign-linux-arm64
 matches cosign "$cosign_sha"
 python3 -I "$here/publish.py" executable cosign "$target/cosign"
fi
if [[ $need_oras == true ]]; then
 curl -fsSL --retry 3 -o oras.tar.gz https://github.com/oras-project/oras/releases/download/v1.3.6/oras_1.3.6_linux_arm64.tar.gz
 matches oras.tar.gz "$archive_sha"
 tar -xzf oras.tar.gz oras
 matches oras "$oras_sha"
 python3 -I "$here/publish.py" executable oras "$target/oras"
fi
