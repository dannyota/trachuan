import argparse
import hashlib
import json
import os
from pathlib import Path
import re


def canonical(obj):
    return json.dumps(obj, sort_keys=True, separators=(',', ':'), ensure_ascii=False).encode()


def require_match(pattern, value, field):
    if not isinstance(value, str) or re.fullmatch(pattern, value) is None:
        raise ValueError(f'invalid {field}')


def positive(value, field):
    if type(value) is not int or value <= 0:
        raise ValueError(f'invalid {field}')


def caddy_from_env(text):
    values = [line.removeprefix('CADDY_IMAGE=') for line in text.splitlines()
              if line.startswith('CADDY_IMAGE=')]
    if len(values) != 1:
        raise ValueError('expected one CADDY_IMAGE')
    require_match(r'docker\.io/library/caddy@sha256:[0-9a-f]{64}', values[0], 'caddy')
    return values[0]


def validate_release(obj):
    keys = {'kind', 'version', 'repository', 'commit', 'ref', 'release', 'sequence',
            'target', 'images', 'upstream', 'host_bundle', 'corpus_release', 'rollback_target'}
    if not isinstance(obj, dict) or set(obj) != keys:
        raise ValueError('invalid release fields')
    for key, value in {'kind': 'trachuan.image-release', 'version': 1,
                       'repository': 'dannyota/trachuan', 'target': 'production',
                       'corpus_release': None, 'rollback_target': None}.items():
        if type(obj[key]) is not type(value) or obj[key] != value:
            raise ValueError(f'invalid {key}')
    require_match(r'[0-9a-f]{40}', obj['commit'], 'commit')
    require_match(r'refs/tags/v[0-9]+\.[0-9]+\.[0-9]+', obj['ref'], 'ref')
    if obj['release'] != obj['ref'].removeprefix('refs/tags/'):
        raise ValueError('release does not match ref')
    positive(obj['sequence'], 'sequence')
    names = {'server', 'web', 'queryembed', 'postgres'}
    if not isinstance(obj['images'], dict) or set(obj['images']) != names:
        raise ValueError('expected four images')
    for name in names:
        require_match(rf'ghcr\.io/dannyota/trachuan-{name}@sha256:[0-9a-f]{{64}}',
                      obj['images'][name], name)
    if not isinstance(obj['upstream'], dict) or set(obj['upstream']) != {'caddy'}:
        raise ValueError('invalid upstream')
    caddy_from_env('CADDY_IMAGE=' + str(obj['upstream']['caddy']))
    bundle = obj['host_bundle']
    if not isinstance(bundle, dict) or set(bundle) != {'sha256', 'size'}:
        raise ValueError('invalid host_bundle')
    require_match(r'[0-9a-f]{64}', bundle['sha256'], 'host bundle hash')
    positive(bundle['size'], 'host bundle size')
    return obj


def release(commit, ref, sequence, images, caddy, bundle):
    if set(images) != {'server', 'web', 'queryembed', 'postgres'}:
        raise ValueError('expected four images')
    for name, digest in images.items():
        require_match(r'sha256:[0-9a-f]{64}', digest, name)
    obj = dict(kind='trachuan.image-release', version=1, repository='dannyota/trachuan',
               commit=commit, ref=ref, release=ref.removeprefix('refs/tags/'), sequence=sequence,
               target='production', images={name: f'ghcr.io/dannyota/trachuan-{name}@{digest}'
                                            for name, digest in images.items()},
               upstream={'caddy': caddy},
               host_bundle={'sha256': hashlib.sha256(bundle).hexdigest(), 'size': len(bundle)},
               corpus_release=None, rollback_target=None)
    return canonical(validate_release(obj))


def read_release(data):
    obj = validate_release(json.loads(data))
    if canonical(obj) != data:
        raise ValueError('manifest is not canonical JSON')
    return obj


def rollback(current_hash, target_hash, sequence, current, target):
    for value in (current_hash, target_hash):
        require_match(r'[0-9a-f]{64}', value, 'manifest hash')
    positive(sequence, 'sequence')
    validate_release(current)
    validate_release(target)
    if target['sequence'] >= current['sequence']:
        raise ValueError('rollback target must have a lower sequence')
    return canonical(dict(kind='trachuan.rollback', version=1, current=current_hash,
                          target=target_hash, sequence=sequence))


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--bundle', required=True, type=Path)
    parser.add_argument('--images-env', required=True, type=Path)
    parser.add_argument('--output', required=True, type=Path)
    args = parser.parse_args()
    args.output.write_bytes(release(
        commit=os.environ['GITHUB_SHA'], ref=os.environ['GITHUB_REF'],
        sequence=int(os.environ['GITHUB_RUN_NUMBER']),
        images={name: os.environ[name.upper()] for name in ('server', 'web', 'queryembed', 'postgres')},
        caddy=caddy_from_env(args.images_env.read_text()), bundle=args.bundle.read_bytes()))


if __name__ == '__main__':
    main()
