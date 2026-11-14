import argparse
import os
from pathlib import Path
import subprocess

from manifest import read_release, require_match, rollback as rollback_document
from registry import find
from signing import sign


def push(directory, repository, tag, artifact_type, host_bundle=False):
    layers = ['manifest.json:application/json',
              'manifest.sigstore.json:application/vnd.dev.sigstore.bundle.v0.3+json']
    if host_bundle:
        layers.append('host-bundle.tar.gz:application/gzip')
    subprocess.run(['oras', 'push', repository + ':' + tag,
                    '--artifact-type', artifact_type,
                    '--annotation', 'org.opencontainers.image.source=https://github.com/dannyota/trachuan',
                    *layers], cwd=directory, check=True)


def release(directory, ref, commit):
    path = directory / 'manifest.json'
    obj = read_release(path.read_bytes())
    if obj['ref'] != ref or obj['commit'] != commit:
        raise ValueError('manifest source differs from workflow source')
    sign(path, directory / 'manifest.sigstore.json', 'images.yml', ref, commit)
    repository = 'ghcr.io/dannyota/trachuan-release'
    push(directory, repository, obj['release'], 'application/vnd.trachuan.release.v1', host_bundle=True)
    subprocess.run(['oras', 'tag', repository + ':' + obj['release'], 'production'], check=True)


def rollback(directory, current, target, sequence, ref, commit):
    if ref != 'refs/heads/main':
        raise ValueError('dispatch rollback from main')
    for digest in (current, target):
        require_match(r'[0-9a-f]{64}', digest, 'manifest hash')
    manifests = find({current, target}, directory / 'releases')
    path = directory / 'manifest.json'
    path.write_bytes(rollback_document(current, target, sequence, manifests[current], manifests[target]))
    sign(path, directory / 'manifest.sigstore.json', 'rollback.yml', ref, commit)
    push(directory, 'ghcr.io/dannyota/trachuan-rollback', str(sequence), 'application/vnd.trachuan.rollback.v1')


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('kind', choices=['release', 'rollback'])
    parser.add_argument('directory', type=Path)
    args = parser.parse_args()
    args.directory.mkdir(parents=True, exist_ok=True)
    ref, commit = os.environ['GITHUB_REF'], os.environ['GITHUB_SHA']
    if args.kind == 'release':
        release(args.directory, ref, commit)
    else:
        rollback(args.directory, os.environ['CURRENT'], os.environ['TARGET'],
                 int(os.environ['GITHUB_RUN_NUMBER']), ref, commit)


if __name__ == '__main__':
    main()
