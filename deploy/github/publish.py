import argparse
import os
from pathlib import Path
import subprocess

from manifest import read_release, require_match, rollback as rollback_document
from registry import find, production
from repositories import listed, package, require_listed, write_repository
from signing import sign


def push(directory, channel, tag, artifact_type, repository, host_bundle=False):
    require_listed(repository)
    layers = ['manifest.json:application/json',
              'manifest.sigstore.json:application/vnd.dev.sigstore.bundle.v0.3+json']
    if host_bundle:
        layers.append('host-bundle.tar.gz:application/gzip')
    subprocess.run(['oras', 'push', channel + ':' + tag,
                    '--artifact-type', artifact_type,
                    '--annotation', 'org.opencontainers.image.source=https://github.com/' + repository,
                    *layers], cwd=directory, check=True)


def release(directory, ref, commit, repository):
    require_listed(repository)
    path = directory / 'manifest.json'
    obj = read_release(path.read_bytes())
    if obj['ref'] != ref or obj['commit'] != commit or obj['repository'] != repository:
        raise ValueError('manifest source differs from workflow source')
    sign(path, directory / 'manifest.sigstore.json', repository, 'images.yml', ref, commit)
    channel = package(repository, 'release')
    push(directory, channel, obj['release'], 'application/vnd.trachuan.release.v1', repository, host_bundle=True)
    current = production(channel, directory / 'production')
    if current is None or obj['sequence'] > current['sequence']:
        subprocess.run(['oras', 'tag', channel + ':' + obj['release'], 'production'], check=True)
    else:
        print(f"Production sequence {current['sequence']} is not below release sequence {obj['sequence']}; leaving production unchanged")


def rollback(directory, current, target, sequence, ref, commit, repository):
    require_listed(repository)
    if ref != 'refs/heads/main':
        raise ValueError('dispatch rollback from main')
    for digest in (current, target):
        require_match(r'[0-9a-f]{64}', digest, 'manifest hash')
    if len(listed()) != 1:
        raise ValueError('rollback refused while the deploy policy lists two repositories')
    manifests = find(package(repository, 'release'), {current, target}, directory / 'releases')
    if manifests[target]['repository'] != repository:
        raise ValueError('rollback target was built by another repository')
    path = directory / 'manifest.json'
    path.write_bytes(rollback_document(current, target, sequence, manifests[current], manifests[target]))
    sign(path, directory / 'manifest.sigstore.json', repository, 'rollback.yml', ref, commit)
    push(directory, package(repository, 'rollback'), str(sequence), 'application/vnd.trachuan.rollback.v1', repository)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('kind', choices=['release', 'rollback'])
    parser.add_argument('directory', type=Path)
    args = parser.parse_args()
    args.directory.mkdir(parents=True, exist_ok=True)
    ref, commit, repository = os.environ['GITHUB_REF'], os.environ['GITHUB_SHA'], write_repository()
    if args.kind == 'release':
        release(args.directory, ref, commit, repository)
    else:
        rollback(args.directory, os.environ['CURRENT'], os.environ['TARGET'],
                 int(os.environ['GITHUB_RUN_NUMBER']), ref, commit, repository)


if __name__ == '__main__':
    main()
