import hashlib
import json
from pathlib import Path
import subprocess

from manifest import read_release, require_match
from signing import verify


def command(args):
    return subprocess.run(args, stdout=subprocess.PIPE, check=True).stdout


def fetch_blob(repository, layer, limit):
    digest = layer['digest']
    require_match(r'sha256:[0-9a-f]{64}', digest, 'layer digest')
    if type(layer['size']) is not int or not 0 < layer['size'] <= limit:
        raise ValueError('invalid layer size')
    args = ['oras', 'blob', 'fetch', '--output', '-', repository + '@' + digest]
    data = bytearray()
    with subprocess.Popen(args, stdout=subprocess.PIPE) as process:
        try:
            while chunk := process.stdout.read(min(65536, layer['size'] + 1 - len(data))):
                data.extend(chunk)
                if len(data) > layer['size']:
                    raise ValueError('layer exceeds advertised size')
            if process.wait() != 0:
                raise subprocess.CalledProcessError(process.returncode, args)
        finally:
            if process.poll() is None:
                process.kill()
                process.wait()
    if len(data) != layer['size'] or 'sha256:' + hashlib.sha256(data).hexdigest() != digest:
        raise ValueError('layer size or hash mismatch')
    return bytes(data)


def artifact_layers(artifact):
    if artifact.get('artifactType') != 'application/vnd.trachuan.release.v1':
        return None
    layers = artifact['layers']
    by_name = {layer.get('annotations', {}).get('org.opencontainers.image.title'): layer for layer in layers}
    if len(layers) != 3 or set(by_name) != {'manifest.json', 'manifest.sigstore.json', 'host-bundle.tar.gz'}:
        raise ValueError('invalid release artifact layers')
    return by_name


def verified_release(repository, layers, directory):
    data = fetch_blob(repository, layers['manifest.json'], 1024 * 1024)
    manifest_hash = hashlib.sha256(data).hexdigest()
    obj = read_release(data)
    path = Path(directory) / manifest_hash
    path.mkdir(parents=True, exist_ok=True)
    manifest_path = path / 'manifest.json'
    signature_path = path / 'manifest.sigstore.json'
    manifest_path.write_bytes(data)
    signature_path.write_bytes(fetch_blob(repository, layers['manifest.sigstore.json'], 2 * 1024 * 1024))
    verify(manifest_path, signature_path, 'images.yml', obj['ref'], obj['commit'], release=True)
    return manifest_hash, obj


def production(directory):
    repository = 'ghcr.io/dannyota/trachuan-release'
    tags = json.loads(command(['oras', 'repo', 'tags', '--format', 'json', repository]))['tags']
    if 'production' not in tags:
        return None
    digest = command(['oras', 'resolve', repository + ':production']).decode().strip()
    require_match(r'sha256:[0-9a-f]{64}', digest, 'artifact digest')
    artifact = json.loads(command(['oras', 'manifest', 'fetch', repository + '@' + digest]))
    layers = artifact_layers(artifact)
    if layers is None:
        raise ValueError('invalid production artifact type')
    return verified_release(repository, layers, directory)[1]


def find(hashes, directory):
    for digest in hashes:
        require_match(r'[0-9a-f]{64}', digest, 'manifest hash')
    repository = 'ghcr.io/dannyota/trachuan-release'
    tags = json.loads(command(['oras', 'repo', 'tags', '--format', 'json', repository]))['tags']
    found = {}
    seen = set()
    for tag in tags:
        require_match(r'[a-zA-Z0-9_][a-zA-Z0-9_.-]{0,127}', tag, 'artifact tag')
        digest = command(['oras', 'resolve', repository + ':' + tag]).decode().strip()
        require_match(r'sha256:[0-9a-f]{64}', digest, 'artifact digest')
        if digest in seen:
            continue
        seen.add(digest)
        artifact = json.loads(command(['oras', 'manifest', 'fetch', repository + '@' + digest]))
        by_name = artifact_layers(artifact)
        if by_name is None:
            continue
        manifest_layer = by_name['manifest.json']
        if manifest_layer['digest'].removeprefix('sha256:') not in hashes:
            continue
        manifest_hash, obj = verified_release(repository, by_name, directory)
        found[manifest_hash] = obj
        if set(found) == hashes:
            return found
    raise ValueError('requested release manifests not found')
