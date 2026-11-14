#!/usr/bin/env bash
set -euo pipefail
[[ $# == 2 && ! -L $2 && ! -L $2/updaters ]]
source_dir=$(cd "$1" && pwd)
install -d -m 0755 "$2" "$2/updaters"
target=$(cd "$2" && pwd)
version=$(cd "$source_dir" && sha256sum __init__.py deploy_*.py main.py proof.py | sha256sum | cut -d' ' -f1)
destination=$target/updaters/$version
staged=$(mktemp -d "$target/updaters/.staging.XXXXXX")
pointer=$target/.updater-link.$$
trap 'rm -rf "$staged"; rm -f "$pointer" "$target/.entry.$$"' EXIT
chmod 0755 "$staged"
install -d -m 0755 "$staged/updater"
for module in "$source_dir"/deploy_*.py "$source_dir/__init__.py" "$source_dir/main.py" "$source_dir/proof.py"; do
 install -m 0644 "$module" "$staged/updater/${module##*/}"
done
if [[ -e $destination ]]; then
 for module in "$staged"/updater/*; do
  cmp -s "$module" "$destination/updater/${module##*/}"
 done
else
 mv "$staged" "$destination"
fi
[[ ! -e $target/updater || -L $target/updater ]]
ln -s "$destination/updater" "$pointer"
python3 -c 'import os, sys; os.replace(sys.argv[1], sys.argv[2])' "$pointer" "$target/updater"
for script in trachuan-deploy proof.sh; do
 [[ ! -L $target/$script ]]
 install -m 0755 "$source_dir/$script" "$target/.entry.$$"
 python3 -c 'import os, sys; os.replace(sys.argv[1], sys.argv[2])' "$target/.entry.$$" "$target/$script"
done
touch "$destination"
python3 - "$target/updaters" <<'PY'
from pathlib import Path
import re
import shutil
import sys
versions = [path for path in Path(sys.argv[1]).iterdir()
            if re.fullmatch('[0-9a-f]{64}', path.name) and path.is_dir() and not path.is_symlink()]
for path in sorted(versions, key=lambda path: path.stat().st_mtime_ns, reverse=True)[5:]:
    shutil.rmtree(path)
PY
