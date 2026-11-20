import hashlib
from pathlib import Path

from .deploy_frameworks_sql import RELEASE
from .deploy_io import canonical, decode
from .deploy_verify import accepted, flags, match, positive


FIELDS = {'kind', 'version', 'corpus_kind', 'repository', 'commit', 'rel', 'database', 'sequence', 'previous',
          'dump', 'report', 'build_artifact_sha256', 'schema_version', 'extensions', 'recipes', 'counts'}
REPORT = ('schema_versions', 'retrieval_recipe_versions', 'lexical_recipe_versions', 'query_recipe_hash')


def manifest(data, rules):
    obj = decode(data, 'frameworks manifest')
    if not isinstance(obj, dict) or set(obj) != FIELDS:
        raise ValueError('invalid frameworks manifest fields')
    accepted(obj['repository'], rules, 'frameworks repository')
    for key, value in dict(kind='trachuan.corpus-release', version=1, corpus_kind='frameworks').items():
        if type(obj[key]) is not type(value) or obj[key] != value:
            raise ValueError('invalid frameworks ' + key)
    match('[0-9a-f]{40}', obj['commit'], 'frameworks commit')
    match(RELEASE, obj['rel'], 'frameworks release')
    if obj['previous'] is not None:
        match(RELEASE, obj['previous'], 'frameworks previous release')
    if obj['database'] != 'frameworks_' + obj['rel']:
        raise ValueError('frameworks database mismatch')
    for field in ('sequence', 'schema_version'):
        positive(obj[field], 'frameworks ' + field)
    dump = obj['dump']
    if not isinstance(dump, dict) or set(dump) != {'object', 'sha256', 'size'}:
        raise ValueError('invalid frameworks dump fields')
    if dump['object'] != 'frameworks/' + obj['rel'] + '/frameworks.dump':
        raise ValueError('frameworks dump object mismatch')
    match('[0-9a-f]{64}', dump['sha256'], 'dump hash')
    positive(dump['size'], 'dump size')
    report = obj['report']
    if not isinstance(report, dict) or set(report) != {'sha256', 'quality_pass'} or report['quality_pass'] is not True:
        raise ValueError('frameworks report quality_pass required')
    match('[0-9a-f]{64}', report['sha256'], 'report hash')
    match('[0-9a-f]{64}', obj['build_artifact_sha256'], 'build artifact hash')
    if not isinstance(obj['extensions'], dict) or set(obj['extensions']) != set(rules['extensions']):
        raise ValueError('unlisted or missing frameworks extension')
    for version in obj['extensions'].values():
        match(r'[0-9]+(?:\.[0-9]+)+', version, 'extension version')
    counts = obj['counts']
    if not isinstance(counts, dict) or set(counts) != {'versions', 'controls'}:
        raise ValueError('invalid frameworks counts')
    for field in ('versions', 'controls'):
        positive(counts[field], 'frameworks ' + field + ' count')
    recipes = obj['recipes']
    if not isinstance(recipes, dict) or set(recipes) != {'dense', 'chunk', 'lexical', 'retrieval'}:
        raise ValueError('invalid frameworks recipes')
    if any(not isinstance(recipe, dict) or not recipe for recipe in recipes.values()):
        raise ValueError('invalid frameworks recipe')
    match('[0-9a-f]{64}', recipes['dense'].get('query_hash'), 'dense query hash')
    for name in ('lexical', 'retrieval'):
        positive(recipes[name].get('version'), 'frameworks ' + name + ' recipe version')
    if canonical(obj) != data:
        raise ValueError('frameworks manifest is not canonical JSON')
    return obj


def verify(directory, rules, runner):
    path = Path(directory) / 'manifest.json'
    data = path.read_bytes()
    obj = manifest(data, rules)
    identity = dict(rules, frameworks_identity=rules['frameworks']['identity'])
    runner(['/usr/local/libexec/trachuan/bin/cosign', 'verify-blob', '--bundle',
            str(Path(directory) / 'manifest.sigstore.json'),
            *flags(identity, 'frameworks', obj['repository'], 'refs/heads/main', obj['commit']), str(path)])
    return hashlib.sha256(data).hexdigest(), obj


def compatible(obj, report):
    if not isinstance(report, dict) or set(report) != set(REPORT):
        raise ValueError('invalid frameworks release-check output')
    for field in REPORT[:3]:
        if not isinstance(report[field], list) or any(type(item) is not int for item in report[field]):
            raise ValueError('invalid frameworks release-check output')
    match('[0-9a-f]{64}', report['query_recipe_hash'], 'frameworks release-check output')
    recipes = obj['recipes']
    return (obj['schema_version'] in report['schema_versions']
            and recipes['retrieval']['version'] in report['retrieval_recipe_versions']
            and recipes['lexical']['version'] in report['lexical_recipe_versions']
            and recipes['dense']['query_hash'] == report['query_recipe_hash'])
