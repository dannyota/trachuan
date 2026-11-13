#!/usr/bin/env python3
"""List binaries and reject additions outside the production allowlist."""

import hashlib
import pathlib
import subprocess
import sys
import tarfile


def inventory(stream, baseline=None, allowed=None):
    binaries = {}
    with tarfile.open(fileobj=stream, mode="r|*") as archive:
        for entry in archive:
            name = entry.name.removeprefix("./").lstrip("/")
            if not entry.isfile():
                continue
            if baseline is not None and (
                pathlib.PurePosixPath(name).name in {"fakegoogle", "fakeembed", "e2efixture"}
                or name.endswith(".onnx")
            ):
                raise ValueError(f"forbidden payload: {name}")
            payload = archive.extractfile(entry)
            header = payload.read(8)
            if not header.startswith((b"\x7fELF", b"#!", b"!<arch>\n")):
                continue
            digest = hashlib.sha256(header)
            while chunk := payload.read(1024 * 1024):
                digest.update(chunk)
            checksum = digest.hexdigest()
            binaries[name] = checksum
            if (
                baseline is not None
                and name not in allowed
                and baseline.get(name) != checksum
            ):
                raise ValueError(f"unexpected binary: {name}")
    return binaries


def image_inventory(image, baseline=None, allowed=None):
    container = subprocess.check_output(
        ["docker", "create", "--entrypoint", "/unused", image], text=True
    ).strip()
    try:
        with subprocess.Popen(
            ["docker", "export", container], stdout=subprocess.PIPE
        ) as export:
            try:
                result = inventory(export.stdout, baseline, allowed)
            except Exception:
                export.kill()
                raise
            if export.wait() != 0:
                raise RuntimeError("docker export failed")
            return result
    finally:
        subprocess.run(
            ["docker", "rm", "--volumes", container],
            check=True, stdout=subprocess.DEVNULL,
        )


def main():
    image, kind = sys.argv[1:]
    containerfile = pathlib.Path(__file__).with_name(f"{kind}.Containerfile")
    runtime = [
        line.split()[1]
        for line in containerfile.read_text().splitlines()
        if line.startswith("FROM ")
    ][-1]
    subprocess.run(["docker", "pull", "--platform", "linux/arm64", runtime], check=True)
    expected = {
        "server": {"app/server", "app/healthcheck"},
        "queryembed": {
            "app/queryembed", "app/healthcheck",
            "opt/native/libonnxruntime.so", "opt/native/libtokenizers.a",
        },
        "web": {"usr/local/bin/node"},
    }[kind]
    baseline = image_inventory(runtime)
    binaries = image_inventory(image, baseline, expected)
    if not expected <= binaries.keys():
        raise ValueError("missing production binary or library")
    for name, checksum in sorted(binaries.items()):
        print(f"{checksum}  /{name}")
    print(f"{kind}: only pinned base binaries and approved production binaries")


if __name__ == "__main__":
    main()
