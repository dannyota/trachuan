"""Create and verify a local model file manifest."""

import hashlib
import json
import re
from pathlib import Path


def digest(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def create_bundle(model_dir: Path, model: str, revision: str, resolved_revision: str) -> dict:
    if not re.fullmatch(r"[0-9a-f]{40}", resolved_revision) or not resolved_revision.startswith(
        revision
    ):
        raise ValueError("revision must resolve to a full commit")
    files = []
    for path in sorted(model_dir.rglob("*")):
        if path.is_symlink():
            raise ValueError("symlinks are not permitted")
        if path.is_file() and path.name != "bundle.json":
            files.append(
                {
                    "path": path.relative_to(model_dir).as_posix(),
                    "size": path.stat().st_size,
                    "sha256": digest(path),
                }
            )
    manifest = {
        "model": model,
        "revision": revision,
        "resolved_revision": resolved_revision,
        "files": files,
    }
    (model_dir / "bundle.json").write_text(
        json.dumps(manifest, ensure_ascii=False, indent=2) + "\n"
    )
    return manifest


def verify_bundle(model_dir: Path) -> dict:
    manifest = json.loads((model_dir / "bundle.json").read_text())
    if not re.fullmatch(r"[0-9a-f]{40}", manifest["resolved_revision"]) or not manifest[
        "resolved_revision"
    ].startswith(manifest["revision"]):
        raise ValueError("invalid revision")
    seen = set()
    for entry in manifest["files"]:
        relative = Path(entry["path"])
        if relative.is_absolute() or ".." in relative.parts or str(relative) in seen:
            raise ValueError("invalid path")
        seen.add(str(relative))
        path = model_dir / relative
        if (
            path.is_symlink()
            or not path.resolve().is_relative_to(model_dir.resolve())
            or path.stat().st_size != entry["size"]
            or digest(path) != entry["sha256"]
        ):
            raise ValueError("bundle mismatch")
    actual = set()
    for path in model_dir.rglob("*"):
        if path.is_symlink():
            raise ValueError("symlinks are not permitted")
        if path.is_file() and path != model_dir / "bundle.json":
            actual.add(path.relative_to(model_dir).as_posix())
    if not seen or actual != seen:
        raise ValueError("bundle file set mismatch")
    return manifest
