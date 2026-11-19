import argparse
import hashlib
import os
from pathlib import Path
import subprocess

from corpus_manifest import decode, read_manifest
from corpus_production import require_signable
from corpus_registry import channel
from manifest import require_match
from publish import push
from repositories import workflow
from signing import sign


def publish(report_sha256, release_sha256, directory, ref, commit, repository):
    require_match(r'[0-9a-f]{64}', report_sha256, 'report SHA-256')
    if release_sha256 != '':
        require_match(r'[0-9a-f]{64}', release_sha256, 'release SHA-256')
    if ref != 'refs/heads/main':
        raise ValueError('dispatch corpus release from main')
    path = directory / 'manifest.json'
    obj = read_manifest(path.read_bytes())
    if obj['commit'] != commit or obj['repository'] != repository:
        raise ValueError('manifest source differs from workflow source')
    report = (directory / 'report.json').read_bytes()
    digest = hashlib.sha256(report).hexdigest()
    if digest != report_sha256:
        raise ValueError('report differs from the dispatched SHA-256')
    if digest != obj['report']['sha256']:
        raise ValueError('report differs from corpus manifest')
    release = (directory / 'release.json').read_bytes()
    if release_sha256 and hashlib.sha256(release).hexdigest() != release_sha256:
        raise ValueError('release.json differs from the dispatched SHA-256')
    run = decode(release)['run']
    current = require_signable(obj, decode(report), run, directory / 'production', repository, resign=not release_sha256)
    sign(path, directory / 'manifest.sigstore.json', repository, 'corpus-release.yml', ref, commit)
    tag = obj['country'] + '-' + obj['rel']
    push(directory, channel(repository), tag, 'application/vnd.trachuan.corpus-release.v1', repository)
    if obj['sequence'] > current['sequence']:
        subprocess.run(['oras', 'tag', channel(repository) + ':' + tag, obj['country'] + '-production'], check=True)
    else:
        print(f"Production sequence {current['sequence']} is not below release sequence {obj['sequence']}; leaving production unchanged")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('report_sha256')
    parser.add_argument('release_sha256')
    parser.add_argument('directory', type=Path)
    args = parser.parse_args()
    publish(args.report_sha256, args.release_sha256, args.directory, os.environ['GITHUB_REF'], os.environ['GITHUB_SHA'],
            workflow())


if __name__ == '__main__':
    main()
