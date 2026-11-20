import hashlib
import json

from corpus_manifest import read_manifest
from manifest import require_match
from registry import command, fetch_blob
from repositories import package
from signing import verify


def channel(repository):
    return package(repository, 'corpus-release')


def repository_tags(repository):
    return json.loads(command(['oras', 'repo', 'tags', '--format', 'json', channel(repository)]))['tags']


def production(repository, country, directory):
    require_match(r'[a-z]{2}', country, 'country')
    tag = country + '-production'
    if tag not in repository_tags(repository):
        return None
    return signed_manifest(repository, country, tag, directory)


def release(repository, country, rel, directory):
    require_match(r'[a-z]{2}', country, 'country')
    require_match(r'[0-9]{8}(?:_[0-9]+)?', rel, 'previous rel')
    obj = signed_manifest(repository, country, country + '-' + rel, directory)
    if obj['rel'] != rel:
        raise ValueError('previous manifest release differs from report')
    return obj


def existing_release(repository, country, rel, directory):
    require_match(r'[a-z]{2}', country, 'country')
    if f'{country}-{rel}' not in repository_tags(repository):
        return None
    return release(repository, country, rel, directory)


def signed_manifest(repository, country, tag, directory):
    def read(data):
        obj = read_manifest(data)
        if obj['country'] != country:
            raise ValueError('production country differs from inputs')
        return obj

    digest = command(['oras', 'resolve', channel(repository) + ':' + tag]).decode().strip()
    return signed_artifact(channel(repository), digest, directory, read, 'corpus-release.yml')


def signed_artifact(source, digest, directory, read, workflow):
    require_match(r'sha256:[0-9a-f]{64}', digest, 'artifact digest')
    artifact = json.loads(command(['oras', 'manifest', 'fetch', source + '@' + digest]))
    if artifact.get('artifactType') != 'application/vnd.trachuan.corpus-release.v1':
        raise ValueError('invalid corpus artifact type')
    layers = artifact['layers']
    by_name = {layer.get('annotations', {}).get('org.opencontainers.image.title'): layer for layer in layers}
    if len(layers) != 2 or set(by_name) != {'manifest.json', 'manifest.sigstore.json'}:
        raise ValueError('invalid corpus artifact layers')
    data = fetch_blob(source, by_name['manifest.json'], 1024 * 1024)
    obj = read(data)
    path = directory / hashlib.sha256(data).hexdigest()
    path.mkdir(parents=True, exist_ok=True)
    manifest, bundle = path / 'manifest.json', path / 'manifest.sigstore.json'
    manifest.write_bytes(data)
    bundle.write_bytes(fetch_blob(source, by_name['manifest.sigstore.json'], 2 * 1024 * 1024))
    verify(manifest, bundle, obj['repository'], workflow, 'refs/heads/main', obj['commit'])
    return obj
