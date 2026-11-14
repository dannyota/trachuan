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
    data = command(['oras', 'blob', 'fetch', '--output', '-', repository + '@' + digest])
    if len(data) != layer['size'] or 'sha256:' + hashlib.sha256(data).hexdigest() != digest:
        raise ValueError('layer size or hash mismatch')
    return data


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
        if artifact.get('artifactType') != 'application/vnd.trachuan.release.v1':
            continue
        layers = artifact['layers']
        by_name = {layer.get('annotations', {}).get('org.opencontainers.image.title'): layer for layer in layers}
        if len(layers) != 3 or set(by_name) != {'manifest.json', 'manifest.sigstore.json', 'host-bundle.tar.gz'}:
            raise ValueError('invalid release artifact layers')
        manifest_layer = by_name['manifest.json']
        if manifest_layer['digest'].removeprefix('sha256:') not in hashes:
            continue
        data = fetch_blob(repository, manifest_layer, 1024 * 1024)
        manifest_hash = hashlib.sha256(data).hexdigest()
        obj = read_release(data)
        path = Path(directory) / manifest_hash
        path.mkdir(parents=True, exist_ok=True)
        manifest_path = path / 'manifest.json'
        signature_path = path / 'manifest.sigstore.json'
        manifest_path.write_bytes(data)
        signature_path.write_bytes(fetch_blob(repository, by_name['manifest.sigstore.json'], 2 * 1024 * 1024))
        verify(manifest_path, signature_path, 'images.yml', obj['ref'], obj['commit'], release=True)
        found[manifest_hash] = obj
        if set(found) == hashes:
            return found
    raise ValueError('requested release manifests not found')
