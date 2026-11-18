import hashlib
from typing import NamedTuple

from corpus_manifest import sha256
from frameworks_recipes import decode, decode_floats, recipe_sha256
from manifest import require_match

BUCKET = 'trachuan-frameworks-releases'
RELEASE = r'[0-9]{8}(?:_[0-9]{1,2})?'
LIMITS = {'frameworks.dump': 16 * 1024**3, 'build.json': 16 * 1024**2, 'evaluate.json': 16 * 1024**2,
          'report.json': 16 * 1024**2, 'release.json': 16 * 1024**2}
ROW_STEPS = ('snapshot', 'parse', 'relate', 'chunk', 'embed')


class Facts(NamedTuple):
    row: dict
    report: dict
    dump_sha256: str
    dump_size: int
    report_sha256: str
    build_sha256: str


def release_id(rel):
    require_match(RELEASE, rel, 'frameworks release')


def object_key(rel, name):
    return f'frameworks/{rel}/{name}'


def object_size(path, name):
    size = path.stat().st_size
    if size <= 0 or size > LIMITS[name]:
        raise ValueError('invalid object size: ' + name)
    return size


def read_json(paths, name):
    object_size(paths[name], name)
    data = paths[name].read_bytes()
    value, numbers = decode(data), decode_floats(data)
    if not isinstance(value, dict):
        raise ValueError('invalid JSON object: ' + name)
    return data, value, numbers


def check_previous(previous):
    if previous is None:
        return
    if not isinstance(previous, dict) or set(previous) != {'rel', 'report_sha256'}:
        raise ValueError('invalid previous reference')
    release_id(previous['rel'])
    require_match(r'[0-9a-f]{64}', previous['report_sha256'], 'previous report hash')


def check_recipes(report, row_numbers, build_numbers, evaluate_numbers):
    hashes = report.get('recipe_hashes')
    if not isinstance(hashes, dict):
        raise ValueError('missing recipe hashes')
    sources = {name: row_numbers.get(name + '_recipe') for name in ROW_STEPS}
    sources['build'] = build_numbers.get('recipe')
    sources['evaluate'] = evaluate_numbers.get('recipe')
    for name, recipe in sources.items():
        require_match(r'[0-9a-f]{64}', hashes.get(name), name + ' recipe hash')
        if recipe_sha256(recipe) != hashes[name]:
            raise ValueError(name + ' recipe differs from report')


def validate_release(rel, paths):
    release_id(rel)
    if set(paths) != set(LIMITS):
        raise ValueError('invalid frameworks object set')
    report_bytes, report, _ = read_json(paths, 'report.json')
    build_bytes, build, build_numbers = read_json(paths, 'build.json')
    _, _, evaluate_numbers = read_json(paths, 'evaluate.json')
    _, row, row_numbers = read_json(paths, 'release.json')
    if report.get('quality_pass') is not True:
        raise ValueError('report quality_pass must be true')
    dump_size = object_size(paths['frameworks.dump'], 'frameworks.dump')
    dump_sha256 = sha256(paths['frameworks.dump'])
    if report.get('dump_sha256') != dump_sha256:
        raise ValueError('report dump hash differs from dump')
    dump = build.get('dump')
    if not isinstance(dump, dict) or dump.get('sha256') != dump_sha256:
        raise ValueError('build manifest dump hash differs from dump')
    build_sha256 = hashlib.sha256(build_bytes).hexdigest()
    if report.get('build_manifest_sha256') != build_sha256:
        raise ValueError('report build manifest hash differs from build.json')
    if report.get('rel') != rel or row.get('rel') != rel:
        raise ValueError('release identity differs from request')
    if row.get('kind') != 'frameworks':
        raise ValueError('release row is not a frameworks release')
    check_recipes(report, row_numbers, build_numbers, evaluate_numbers)
    if 'previous' not in report:
        raise ValueError('report previous reference missing')
    check_previous(report['previous'])
    return Facts(row, report, dump_sha256, dump_size, hashlib.sha256(report_bytes).hexdigest(), build_sha256)
