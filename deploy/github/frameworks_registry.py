import subprocess

from corpus_registry import REPOSITORY, signed_artifact
from frameworks_manifest import read_manifest
from frameworks_release import release_id

WORKFLOW = 'frameworks-release.yml'
PRODUCTION = 'frameworks-production'


def missing_tag(stderr, reference):
    expected = f'Error response from registry: failed to resolve digest: {reference}: not found'
    return expected in (line.strip() for line in stderr.splitlines())


def resolve(tag):
    reference = REPOSITORY + ':' + tag
    result = subprocess.run(['oras', 'resolve', reference], capture_output=True, text=True, errors='replace')
    if result.returncode == 0:
        return result.stdout.strip()
    if missing_tag(result.stderr, reference):
        return None
    raise ValueError('registry lookup failed: ' + tag)


def production(directory):
    digest = resolve(PRODUCTION)
    if digest is None:
        return None
    return signed_artifact(digest, directory, read_manifest, WORKFLOW)


def release(rel, directory):
    release_id(rel)
    tag = 'frameworks-' + rel
    digest = resolve(tag)
    if digest is None:
        raise ValueError('signed frameworks manifest missing: ' + tag)
    obj = signed_artifact(digest, directory, read_manifest, WORKFLOW)
    if obj['rel'] != rel:
        raise ValueError('signed frameworks manifest differs from its tag: ' + tag)
    return obj
