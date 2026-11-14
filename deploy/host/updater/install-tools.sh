#!/usr/bin/env bash
set -euo pipefail
[[ $# == 1 && $(uname -s) == Linux && $(uname -m) == aarch64 ]]
target=$1
[[ ! -L $target ]]
directory=$(mktemp -d)
staged=
trap 'rm -rf "$directory"; if [[ -n $staged ]]; then rm -rf "$staged"; fi' EXIT
cd "$directory"
curl -fsSL --retry 3 -o cosign https://github.com/sigstore/cosign/releases/download/v3.1.4/cosign-linux-arm64
printf '%s\n' 'ade83b7aac62e131cacfe96d958948b2150cc9efec46a570a2649b8e27dedad2  cosign' | sha256sum --check --strict
curl -fsSL --retry 3 -o oras.tar.gz https://github.com/oras-project/oras/releases/download/v1.3.6/oras_1.3.6_linux_arm64.tar.gz
printf '%s\n' 'b84269e78dfc2d5329a4aab373da3200de95e6c138338cf6c66c10c0a3f248c1  oras.tar.gz' | sha256sum --check --strict
tar -xzf oras.tar.gz oras
install -d -m 0755 "$target"
staged=$(mktemp -d "$target/.tools.XXXXXX")
install -m 0755 cosign oras "$staged/"
for tool in cosign oras; do
 python3 -c 'import os, sys; os.replace(sys.argv[1], sys.argv[2])' "$staged/$tool" "$target/$tool"
done
