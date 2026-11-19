import subprocess

from corpus_registry import channel, signed_artifact
from frameworks_manifest import read_manifest
from frameworks_release import release_id

WORKFLOW = 'frameworks-release.yml'
PRODUCTION = 'frameworks-production'


def missing_tag(stderr, reference):
    expected = f'Error response from registry: failed to resolve digest: {reference}: not found'
    return expected in (line.strip() for line in stderr.splitlines())


def resolve(repository, tag):
    reference = channel(repository) + ':' + tag
    result = subprocess.run(['oras', 'resolve', reference], capture_output=True, text=True, errors='replace')
    if result.returncode == 0:
        return result.stdout.strip()
    if missing_tag(result.stderr, reference):
        return None
    raise ValueError('registry lookup failed: ' + tag)


def production(repository, directory):
    digest = resolve(repository, PRODUCTION)
    if digest is None:
        return None
    return signed_artifact(channel(repository), digest, directory, read_manifest, WORKFLOW)


def release(repository, rel, directory):
    release_id(rel)
    tag = 'frameworks-' + rel
    digest = resolve(repository, tag)
    if digest is None:
        raise ValueError('signed frameworks manifest missing: ' + tag)
    obj = signed_artifact(channel(repository), digest, directory, read_manifest, WORKFLOW)
    if obj['rel'] != rel:
        raise ValueError('signed frameworks manifest differs from its tag: ' + tag)
    return obj
