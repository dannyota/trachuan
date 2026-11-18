import hashlib
import json

from corpus_manifest import read_manifest
from manifest import require_match
from registry import command, fetch_blob
from signing import verify


def repository_tags():
    repository = 'ghcr.io/dannyota/trachuan-corpus-release'
    return json.loads(command(['oras', 'repo', 'tags', '--format', 'json', repository]))['tags']


def production(country, directory):
    require_match(r'[a-z]{2}', country, 'country')
    tag = country + '-production'
    if tag not in repository_tags():
        return None
    return signed_manifest(country, tag, directory)


def release(country, rel, directory):
    require_match(r'[a-z]{2}', country, 'country')
    require_match(r'[0-9]{8}(?:_[0-9]+)?', rel, 'previous rel')
    obj = signed_manifest(country, country + '-' + rel, directory)
    if obj['rel'] != rel:
        raise ValueError('previous manifest release differs from report')
    return obj


def existing_release(country, rel, directory):
    require_match(r'[a-z]{2}', country, 'country')
    if f'{country}-{rel}' not in repository_tags():
        return None
    return release(country, rel, directory)


def signed_manifest(country, tag, directory):
    repository = 'ghcr.io/dannyota/trachuan-corpus-release'
    digest = command(['oras', 'resolve', repository + ':' + tag]).decode().strip()
    require_match(r'sha256:[0-9a-f]{64}', digest, 'artifact digest')
    artifact = json.loads(command(['oras', 'manifest', 'fetch', repository + '@' + digest]))
    if artifact.get('artifactType') != 'application/vnd.trachuan.corpus-release.v1':
        raise ValueError('invalid corpus artifact type')
    layers = artifact['layers']
    by_name = {layer.get('annotations', {}).get('org.opencontainers.image.title'): layer for layer in layers}
    if len(layers) != 2 or set(by_name) != {'manifest.json', 'manifest.sigstore.json'}:
        raise ValueError('invalid corpus artifact layers')
    data = fetch_blob(repository, by_name['manifest.json'], 1024 * 1024)
    obj = read_manifest(data)
    if obj['country'] != country:
        raise ValueError('production country differs from inputs')
    path = directory / hashlib.sha256(data).hexdigest()
    path.mkdir(parents=True, exist_ok=True)
    manifest, bundle = path / 'manifest.json', path / 'manifest.sigstore.json'
    manifest.write_bytes(data)
    bundle.write_bytes(fetch_blob(repository, by_name['manifest.sigstore.json'], 2 * 1024 * 1024))
    verify(manifest, bundle, 'corpus-release.yml', 'refs/heads/main', obj['commit'])
    return obj
