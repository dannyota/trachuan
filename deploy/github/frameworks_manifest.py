from frameworks_recipes import decode
from frameworks_release import RELEASE, object_key, release_id
from manifest import canonical, positive, require_match

FIELDS = {'kind', 'version', 'corpus_kind', 'repository', 'commit', 'rel', 'database', 'sequence', 'previous',
          'dump', 'report', 'build_artifact_sha256', 'schema_version', 'extensions', 'recipes', 'counts'}
HASH = r'[0-9a-f]{64}'


def validate_manifest(obj):
    if not isinstance(obj, dict) or set(obj) != FIELDS:
        raise ValueError('invalid frameworks manifest fields')
    for key, value in dict(kind='trachuan.corpus-release', version=1, corpus_kind='frameworks',
                           repository='dannyota/trachuan').items():
        if type(obj[key]) is not type(value) or obj[key] != value:
            raise ValueError('invalid frameworks ' + key)
    require_match(r'[0-9a-f]{40}', obj['commit'], 'frameworks commit')
    release_id(obj['rel'])
    if obj['previous'] is not None:
        require_match(RELEASE, obj['previous'], 'frameworks previous release')
    if obj['database'] != 'frameworks_' + obj['rel']:
        raise ValueError('frameworks database differs from release')
    positive(obj['sequence'], 'frameworks sequence')
    positive(obj['schema_version'], 'frameworks schema version')
    dump, report = obj['dump'], obj['report']
    if not isinstance(dump, dict) or set(dump) != {'object', 'sha256', 'size'}:
        raise ValueError('invalid frameworks dump fields')
    if dump['object'] != object_key(obj['rel'], 'frameworks.dump'):
        raise ValueError('invalid frameworks dump object')
    require_match(HASH, dump['sha256'], 'dump hash')
    positive(dump['size'], 'dump size')
    if not isinstance(report, dict) or set(report) != {'sha256', 'quality_pass'} or report['quality_pass'] is not True:
        raise ValueError('invalid frameworks report fields')
    require_match(HASH, report['sha256'], 'report hash')
    require_match(HASH, obj['build_artifact_sha256'], 'build artifact hash')
    extensions = obj['extensions']
    if not isinstance(extensions, dict) or set(extensions) != {'vector', 'ltree'}:
        raise ValueError('expected exactly vector and ltree extensions')
    for version in extensions.values():
        require_match(r'[0-9]+(?:\.[0-9]+)+', version, 'extension version')
    counts = obj['counts']
    if not isinstance(counts, dict) or set(counts) != {'versions', 'controls'}:
        raise ValueError('invalid frameworks counts')
    positive(counts['versions'], 'frameworks version count')
    positive(counts['controls'], 'frameworks control count')
    recipes = obj['recipes']
    if not isinstance(recipes, dict) or set(recipes) != {'dense', 'chunk', 'lexical', 'retrieval'}:
        raise ValueError('invalid frameworks recipes')
    if any(not isinstance(recipe, dict) or not recipe for recipe in recipes.values()):
        raise ValueError('invalid frameworks recipe')
    require_match(HASH, recipes['dense'].get('query_hash'), 'dense query hash')
    positive(recipes['lexical'].get('version'), 'frameworks lexical recipe version')
    positive(recipes['retrieval'].get('version'), 'frameworks retrieval recipe version')
    return obj


def read_manifest(data):
    obj = validate_manifest(decode(data))
    if canonical(obj) != data:
        raise ValueError('frameworks manifest is not canonical JSON')
    return obj


def document(commit, sequence, rel, facts):
    row, report = facts.row, facts.report
    previous = report['previous']['rel'] if report['previous'] is not None else None
    obj = dict(kind='trachuan.corpus-release', version=1, corpus_kind='frameworks', repository='dannyota/trachuan',
               commit=commit, rel=rel, database='frameworks_' + rel, sequence=sequence, previous=previous,
               dump=dict(object=object_key(rel, 'frameworks.dump'), sha256=facts.dump_sha256, size=facts.dump_size),
               report=dict(sha256=facts.report_sha256, quality_pass=True),
               build_artifact_sha256=facts.build_sha256, schema_version=row.get('schema_version'),
               extensions=row.get('extensions'),
               recipes={name: row.get(name + '_recipe') for name in ('dense', 'chunk', 'lexical', 'retrieval')},
               counts=dict(versions=row.get('version_count'), controls=row.get('control_count')))
    return canonical(validate_manifest(obj))
