import argparse
import hashlib
import os
from pathlib import Path
import subprocess

from corpus_manifest import decode, read_manifest
from corpus_production import require_signable
from manifest import require_match
from publish import push
from signing import sign


def publish(report_sha256, directory, ref, commit):
    require_match(r'[0-9a-f]{64}', report_sha256, 'report SHA-256')
    if ref != 'refs/heads/main':
        raise ValueError('dispatch corpus release from main')
    path = directory / 'manifest.json'
    obj = read_manifest(path.read_bytes())
    if obj['commit'] != commit:
        raise ValueError('manifest source differs from workflow source')
    report = (directory / 'report.json').read_bytes()
    digest = hashlib.sha256(report).hexdigest()
    if digest != report_sha256:
        raise ValueError('report differs from the dispatched SHA-256')
    if digest != obj['report']['sha256']:
        raise ValueError('report differs from corpus manifest')
    run = decode((directory / 'release.json').read_bytes())['run']
    current = require_signable(obj, decode(report), run, directory / 'production')
    sign(path, directory / 'manifest.sigstore.json', 'corpus-release.yml', ref, commit)
    repository = 'ghcr.io/dannyota/trachuan-corpus-release'
    tag = obj['country'] + '-' + obj['rel']
    push(directory, repository, tag, 'application/vnd.trachuan.corpus-release.v1')
    if obj['sequence'] > current['sequence']:
        subprocess.run(['oras', 'tag', repository + ':' + tag, obj['country'] + '-production'], check=True)
    else:
        print(f"Production sequence {current['sequence']} is not below release sequence {obj['sequence']}; leaving production unchanged")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('report_sha256')
    parser.add_argument('directory', type=Path)
    args = parser.parse_args()
    publish(args.report_sha256, args.directory, os.environ['GITHUB_REF'], os.environ['GITHUB_SHA'])


if __name__ == '__main__':
    main()
