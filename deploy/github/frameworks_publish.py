import argparse
import os
from pathlib import Path
import subprocess

from corpus_registry import REPOSITORY
from frameworks_cli import guarded
from frameworks_manifest import document, read_manifest
from frameworks_previous import require_signed
from frameworks_registry import PRODUCTION, WORKFLOW, production
from frameworks_release import LIMITS, validate_release
from publish import push
from signing import sign

MAIN = 'refs/heads/main'


def prepare(rel, directory, ref, commit, sequence):
    if ref != MAIN:
        raise ValueError('dispatch frameworks release from main')
    facts = validate_release(rel, {name: directory / 'objects' / name for name in LIMITS})
    data = document(commit, sequence, rel, facts)
    require_signed(facts.report, directory / 'previous')
    output = directory / 'sign'
    output.mkdir(mode=0o700)
    (output / 'manifest.json').write_bytes(data)
    return output


def publish(directory, current_directory, ref, commit):
    if ref != MAIN:
        raise ValueError('dispatch frameworks release from main')
    path = directory / 'manifest.json'
    obj = read_manifest(path.read_bytes())
    if obj['commit'] != commit:
        raise ValueError('manifest source differs from workflow source')
    sign(path, directory / 'manifest.sigstore.json', WORKFLOW, ref, commit, stdout=subprocess.DEVNULL)
    tag = 'frameworks-' + obj['rel']
    push(directory, REPOSITORY, tag, 'application/vnd.trachuan.corpus-release.v1')
    current = production(current_directory)
    if current is None or obj['sequence'] > current['sequence']:
        subprocess.run(['oras', 'tag', REPOSITORY + ':' + tag, PRODUCTION], check=True)
    else:
        print(f"Production sequence {current['sequence']} is not below release sequence {obj['sequence']}; leaving production unchanged")


def release(rel, directory, ref, commit, sequence):
    output = prepare(rel, directory, ref, commit, sequence)
    publish(output, directory / 'production', ref, commit)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('rel')
    parser.add_argument('directory', type=Path)
    args = parser.parse_args()
    guarded(lambda: release(args.rel, args.directory, os.environ['GITHUB_REF'], os.environ['GITHUB_SHA'],
                            int(os.environ['GITHUB_RUN_NUMBER'])))


if __name__ == '__main__':
    main()
