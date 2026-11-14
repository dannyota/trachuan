import copy
from datetime import datetime
import hashlib
import json
from pathlib import Path
import re

from .deploy_io import canonical
from .deploy_verify import flags, match, positive


def release_name(value):
    match(r'[0-9]{8}(?:_[1-9][0-9]*)?', value, 'corpus release')
    datetime.strptime(value[:8], '%Y%m%d')


def manifest(data, rules):
    obj = json.loads(data)
    keys = {'kind', 'version', 'repository', 'commit', 'country', 'rel', 'database',
            'sequence', 'previous', 'dump', 'report', 'build_artifact_sha256',
            'schema_version', 'extensions', 'recipes'}
    if not isinstance(obj, dict) or set(obj) != keys:
        raise ValueError('invalid corpus manifest fields')
    for key, value in dict(kind='trachuan.corpus-release', version=1,
                           repository=rules['repository'], country='vn').items():
        if type(obj[key]) is not type(value) or obj[key] != value:
            raise ValueError('invalid corpus ' + key)
    match('[0-9a-f]{40}', obj['commit'], 'corpus commit')
    release_name(obj['rel'])
    release_name(obj['previous'])
    if obj['database'] != 'corpus_' + obj['country'] + '_' + obj['rel']:
        raise ValueError('corpus database mismatch')
    for field in ('sequence', 'schema_version'):
        positive(obj[field], 'corpus ' + field)
    dump = obj['dump']
    if not isinstance(dump, dict) or set(dump) != {'object', 'sha256', 'size'}:
        raise ValueError('invalid corpus dump fields')
    if dump['object'] != obj['country'] + '/' + obj['rel'] + '/corpus.dump':
        raise ValueError('corpus dump object mismatch')
    match('[0-9a-f]{64}', dump['sha256'], 'dump hash')
    positive(dump['size'], 'dump size')
    report = obj['report']
    if not isinstance(report, dict) or set(report) != {'sha256', 'quality_pass'} or report['quality_pass'] is not True:
        raise ValueError('corpus report quality_pass required')
    match('[0-9a-f]{64}', report['sha256'], 'report hash')
    match('[0-9a-f]{64}', obj['build_artifact_sha256'], 'build artifact hash')
    if not isinstance(obj['extensions'], dict) or set(obj['extensions']) != set(rules['extensions']):
        raise ValueError('unlisted or missing corpus extension')
    for version in obj['extensions'].values():
        match(r'[0-9]+(?:\.[0-9]+)+', version, 'extension version')
    recipes = obj['recipes']
    if not isinstance(recipes, dict) or set(recipes) != {'dense', 'chunk', 'lexical', 'retrieval'}:
        raise ValueError('invalid corpus recipes')
    if any(not isinstance(recipe, dict) or not recipe for recipe in recipes.values()):
        raise ValueError('invalid corpus recipe')
    match('[0-9a-f]{64}', recipes['dense'].get('query_hash'), 'dense query hash')
    if canonical(obj) != data:
        raise ValueError('corpus manifest is not canonical JSON')
    return obj


def verify(directory, rules, runner):
    path = Path(directory) / 'manifest.json'
    data = path.read_bytes()
    obj = manifest(data, rules)
    runner(['/usr/local/libexec/trachuan/bin/cosign', 'verify-blob', '--bundle',
            str(Path(directory) / 'manifest.sigstore.json'),
            *flags(rules, 'corpus', 'refs/heads/main', obj['commit']), str(path)])
    return hashlib.sha256(data).hexdigest(), obj


def uncalibrated(recipe):
    obj = copy.deepcopy(recipe)
    obj.get('abstention', {}).pop('threshold', None)
    obj.get('abstention', {}).pop('calibration_sha256', None)
    return obj


def compatible(obj, report):
    if obj['schema_version'] not in report['schema_versions']:
        raise ValueError('unsupported corpus schema version')
    recipes = obj['recipes']
    query = recipes['dense']['query_hash']
    return (query == report['dense_query_hash'] == report['running_query']['recipe_hash']
            and recipes['dense']['query'] == report['running_query'].get('recipe')
            and recipes['lexical'] == report['lexical_recipe']
            and uncalibrated(recipes['retrieval']) in [uncalibrated(r) for r in report['retrieval_recipes']])


def toc(text, extensions):
    kept, seen = [], set()
    allowed = ('SCHEMA ', 'TABLE ', 'TABLE DATA ', 'SEQUENCE ', 'SEQUENCE SET ',
               'SEQUENCE OWNED BY ', 'DEFAULT ', 'CONSTRAINT ', 'FK CONSTRAINT ',
               'INDEX ', 'INDEX ATTACH ', 'STATISTICS ', 'COMMENT ', 'ACL ',
               'DEFAULT ACL ', 'ENCODING ', 'STDSTRINGS ', 'SEARCHPATH ')
    for line in text.splitlines():
        if not line or line.startswith(';'):
            continue
        found = re.fullmatch(r'[0-9]+; [0-9]+ [0-9]+ (.+)', line)
        if not found:
            raise ValueError('invalid dump TOC')
        entry = found[1]
        if entry.startswith('EXTENSION '):
            parts = entry.split()
            if len(parts) < 3 or parts[1] != '-' or parts[2] not in extensions or parts[2] in seen:
                raise ValueError('unlisted or duplicate dump extension')
            seen.add(parts[2])
        elif entry.startswith('COMMENT - EXTENSION '):
            parts = entry.split()
            if len(parts) < 4 or parts[3] not in extensions:
                raise ValueError('unlisted extension comment')
        elif not entry.startswith(allowed) or ' EXTENSION ' in entry:
            raise ValueError('unsafe dump TOC object')
        else:
            kept.append(line)
    if seen != set(extensions):
        raise ValueError('missing dump extensions')
    return '\n'.join(kept) + '\n'
