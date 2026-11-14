import hashlib
import json
from pathlib import Path
import re

from .deploy_io import atomic, command, CommandError
from .deploy_verify import match


def layers(artifact, kind):
    if artifact.get('artifactType') != 'application/vnd.trachuan.' + kind + '.v1':
        raise ValueError('invalid artifact type')
    limits = {'manifest.json': 1024 * 1024, 'manifest.sigstore.json': 2 * 1024 * 1024}
    if kind == 'release':
        limits['host-bundle.tar.gz'] = 16 * 1024 * 1024
    descriptors = artifact.get('layers', [])
    result = {}
    for layer in descriptors:
        name = layer.get('annotations', {}).get('org.opencontainers.image.title')
        if name not in limits or name in result:
            raise ValueError('invalid artifact layer title')
        match('sha256:[0-9a-f]{64}', layer.get('digest'), 'layer digest')
        if type(layer.get('size')) is not int or not 0 < layer['size'] <= limits[name]:
            raise ValueError('invalid artifact layer size')
        result[name] = layer
    if set(result) != set(limits):
        raise ValueError('missing artifact layer')
    return result


class Registry:
    def __init__(self, rules, runner=command):
        self.rules, self.runner = rules, runner

    def oras(self, *args, **kwargs):
        return self.runner(['/usr/local/libexec/trachuan/bin/oras', *args], **kwargs)

    def tags(self, repository):
        tags = json.loads(self.oras('repo', 'tags', '--format', 'json', repository))['tags'] or []
        if not isinstance(tags, list) or len(tags) > 10000:
            raise ValueError('invalid artifact tags')
        for tag in tags:
            match(r'[a-zA-Z0-9_][a-zA-Z0-9_.-]{0,127}', tag, 'artifact tag')
        return tags

    def latest_rollback(self):
        try:
            tags = self.tags(self.rules['rollback_repository'])
        except CommandError as error:
            if error.reason == 'not_found':
                return None
            raise
        numbers = [tag for tag in tags if re.fullmatch('[1-9][0-9]*', tag)]
        return max(numbers, key=int) if numbers else None

    def fetch(self, repository, tag, directory, kind):
        match(r'[a-zA-Z0-9_][a-zA-Z0-9_.-]{0,127}', tag, 'artifact tag')
        digest = self.oras('resolve', repository + ':' + tag).decode().strip()
        match('sha256:[0-9a-f]{64}', digest, 'artifact digest')
        reference = repository + '@' + digest
        raw = self.oras('manifest', 'fetch', reference)
        if len(raw) > 1024 * 1024:
            raise ValueError('artifact manifest too large')
        descriptors = layers(json.loads(raw), kind)
        directory = Path(directory)
        directory.mkdir(mode=0o700, parents=True, exist_ok=True)
        for name, layer in descriptors.items():
            path = directory / name
            data = self.oras('blob', 'fetch', '--output', '-', repository + '@' + layer['digest'], stdout_limit=layer['size'])
            if len(data) != layer['size'] or 'sha256:' + hashlib.sha256(data).hexdigest() != layer['digest']:
                raise ValueError('artifact layer size or hash mismatch')
            atomic(path, data)
        return directory

    def find(self, hash_value, directory):
        match('[0-9a-f]{64}', hash_value, 'manifest hash')
        repository = self.rules['release_repository']
        for index, tag in enumerate(self.tags(repository)):
            path = self.fetch(repository, tag, Path(directory) / str(index), 'release')
            if hashlib.sha256((path / 'manifest.json').read_bytes()).hexdigest() == hash_value:
                return path
        raise ValueError('bootstrap manifest not found')
