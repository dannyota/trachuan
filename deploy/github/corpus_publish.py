import argparse
import os
from pathlib import Path
import subprocess

from corpus_manifest import read_manifest
from corpus_registry import production
from publish import push
from signing import sign


def publish(directory, ref, commit):
    if ref != 'refs/heads/main':
        raise ValueError('dispatch corpus release from main')
    path = directory / 'manifest.json'
    obj = read_manifest(path.read_bytes())
    if obj['commit'] != commit:
        raise ValueError('manifest source differs from workflow source')
    sign(path, directory / 'manifest.sigstore.json', 'corpus-release.yml', ref, commit)
    repository = 'ghcr.io/dannyota/trachuan-corpus-release'
    tag = obj['country'] + '-' + obj['rel']
    push(directory, repository, tag, 'application/vnd.trachuan.corpus-release.v1')
    current = production(obj['country'], directory / 'production')
    if current is None or obj['sequence'] > current['sequence']:
        subprocess.run(['oras', 'tag', repository + ':' + tag, obj['country'] + '-production'], check=True)
    else:
        print(f"Production sequence {current['sequence']} is not below release sequence {obj['sequence']}; leaving production unchanged")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('directory', type=Path)
    args = parser.parse_args()
    publish(args.directory, os.environ['GITHUB_REF'], os.environ['GITHUB_SHA'])


if __name__ == '__main__':
    main()
