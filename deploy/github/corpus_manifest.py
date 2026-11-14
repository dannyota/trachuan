import argparse
import hashlib
import json
import os
from pathlib import Path
import re

from manifest import canonical, positive, require_match


def unique_object(pairs):
    obj = {}
    for key, value in pairs:
        if key in obj:
            raise ValueError('duplicate JSON field')
        obj[key] = value
    return obj


def decode(data):
    def invalid_constant(value):
        raise ValueError('invalid JSON constant')
    return json.loads(data, object_pairs_hook=unique_object, parse_constant=invalid_constant)


def sha256(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def identity(country, rel):
    require_match(r'[a-z]{2}', country, 'country')
    require_match(r'[0-9]{8}_[0-9]+', rel, 'rel')


def no_research_override(value):
    if isinstance(value, dict):
        for key, child in value.items():
            if key in {'cap_override', 'article_cap_override', 'historical_policy', 'non_current_tail'} and child is not None:
                raise ValueError('research override is not releasable')
            if key == 'not_yet_eligible' and child is not False:
                raise ValueError('research override is not releasable')
            no_research_override(child)
    elif isinstance(value, list):
        for child in value:
            no_research_override(child)
    elif value == 'research_override':
        raise ValueError('research override is not releasable')


def non_regression_contract():
    comparisons = [dict(mode=mode, arms='hybrid', limit=limit, name=name, gating=gating)
                   for mode in ('exact', 'ann')
                   for name, limit, gating in (('recall', 20, True), ('mrr', 8, True),
                                               ('abstention_balanced_accuracy', 8, False))]
    return {'legacy': [], 'old_product': comparisons, 'development': comparisons}


def check_comparisons(records, expected):
    if not isinstance(records, list):
        raise ValueError('invalid non_regression')
    seen = {}
    for record in records:
        if not isinstance(record, dict) or type(record.get('gating')) is not bool:
            raise ValueError('invalid non_regression gating')
        if any(not isinstance(record.get(field), str) for field in ('mode', 'arms', 'name')) or type(record.get('limit')) is not int:
            raise ValueError('invalid non_regression identity')
        key = tuple(record[field] for field in ('mode', 'arms', 'limit', 'name'))
        if key in seen:
            raise ValueError('duplicate non_regression entry')
        seen[key] = record
        if record['gating'] and record.get('status') != 'pass':
            raise ValueError('gating non_regression did not pass')
    for entry in expected:
        if entry['gating']:
            key = tuple(entry[field] for field in ('mode', 'arms', 'limit', 'name'))
            if key not in seen or seen[key]['gating'] is not True or seen[key].get('status') != 'pass':
                raise ValueError('required non_regression gate missing or not passing')


def validate_report(report, country, rel):
    if not isinstance(report, dict):
        raise ValueError('invalid report')
    if type(report.get('version')) is not int or report['version'] != 1:
        raise ValueError('invalid report version')
    if report.get('country') != country or report.get('rel') != rel:
        raise ValueError('report identity differs from inputs')
    if report.get('status') != 'complete' or report.get('quality_pass') is not True or report.get('release_eligible') is not False:
        raise ValueError('report must have completed quality checks and release_eligible false')
    no_research_override(report)
    diff = report.get('previous_diff')
    if not isinstance(diff, dict) or 'reason' in diff or not isinstance(diff.get('previous'), dict):
        raise ValueError('report must compare a previous release')
    previous = diff['previous']
    if previous.get('country') != country:
        raise ValueError('previous country differs from inputs')
    require_match(r'[0-9]{8}(?:_[0-9]+)?', previous.get('rel'), 'previous rel')
    require_match(r'[0-9a-f]{64}', previous.get('report_sha256'), 'previous report hash')
    contract = non_regression_contract()
    check_comparisons(report.get('non_regression'), contract['old_product'])
    cohorts = report.get('cohorts')
    if not isinstance(cohorts, dict) or set(cohorts) != set(contract):
        raise ValueError('missing report cohorts')
    for name, cohort in cohorts.items():
        if not isinstance(cohort, dict) or type(cohort.get('case_count')) is not int or cohort['case_count'] < 0:
            raise ValueError('invalid cohort')
        if cohort['case_count'] > 0:
            check_comparisons(cohort.get('non_regression', []), contract[name])


def validate_recipes(row, report):
    dense = row.get('dense_recipe')
    if not isinstance(dense, dict):
        raise ValueError('invalid dense recipe')
    positive(dense.get('dimensions'), 'dense dimensions')
    if type(dense.get('version')) is not int or dense['version'] != 1 or not isinstance(dense.get('model'), str) or not dense['model']:
        raise ValueError('invalid dense recipe version or model')
    objects = dict(query=dense.get('query'), document=dense.get('document'),
                   chunk=row.get('chunk_recipe'), lexical=row.get('lexical_recipe'),
                   retrieval=row.get('retrieval_recipe'))
    inputs, hashes = row.get('recipe_hash_inputs'), report.get('recipes')
    if not isinstance(inputs, dict) or set(inputs) != set(objects) or not isinstance(hashes, dict) or set(hashes) != set(objects):
        raise ValueError('missing recipe hash inputs')
    for name, obj in objects.items():
        raw = inputs[name]
        if not isinstance(obj, dict) or not isinstance(raw, str) or canonical(decode(raw)) != canonical(obj):
            raise ValueError(f'{name} recipe differs from hash input')
        digest = hashlib.sha256(raw.encode()).hexdigest()
        if digest != hashes.get(name):
            raise ValueError(f'{name} recipe differs from report')
        if name in {'query', 'document'} and dense.get(name + '_hash') != digest:
            raise ValueError(f'{name} recipe hash differs from release row')
        if name in {'query', 'document'} and (obj.get('model') != dense['model'] or obj.get('dimensions') != dense['dimensions']):
            raise ValueError(f'{name} recipe model or dimensions differ from dense recipe')


def extensions(value):
    if not isinstance(value, dict) or set(value) != {'vector', 'ltree'}:
        raise ValueError('expected exactly vector and ltree extensions')
    if any(not isinstance(version, str) or not version for version in value.values()):
        raise ValueError('invalid extension version')


def validate_inputs(country, rel, directory, report_path):
    identity(country, rel)
    row = decode((directory / 'release.json').read_bytes())
    report = decode(report_path.read_bytes())
    validate_report(report, country, rel)
    if not isinstance(row, dict) or row.get('country') != country or row.get('rel') != rel:
        raise ValueError('release row identity differs from inputs')
    run = row.get('run')
    if not isinstance(run, dict) or run.get('complete') is not True or run.get('incomplete_items') != 0 or run.get('unaccepted_gaps') != 0:
        raise ValueError('run must be complete with every gap accepted')
    positive(run.get('expected_items'), 'expected items')
    extensions(row.get('extensions'))
    positive(row.get('schema_version'), 'schema version')
    validate_recipes(row, report)
    dump_hash = sha256(directory / 'corpus.dump')
    manifest_path = directory / 'MANIFEST.sha256'
    build_hash = sha256(manifest_path)
    if report.get('dump_sha256') != dump_hash:
        raise ValueError('report dump hash differs from local dump')
    if report.get('build_artifact_sha256') != build_hash or run.get('build_artifact_sha256') != build_hash:
        raise ValueError('build manifest hash differs from report or release run')
    dump_lines = [line for line in manifest_path.read_text().splitlines()
                  if re.fullmatch(r'[0-9a-f]{64}  corpus\.dump', line)]
    if dump_lines != [dump_hash + '  corpus.dump']:
        raise ValueError('build manifest must name the dump hash exactly once')
    positive((directory / 'corpus.dump').stat().st_size, 'dump size')
    return row, report


def validate_manifest(obj):
    keys = {'kind', 'version', 'repository', 'commit', 'country', 'rel', 'database',
            'sequence', 'previous', 'dump', 'report', 'build_artifact_sha256',
            'schema_version', 'extensions', 'recipes'}
    if not isinstance(obj, dict) or set(obj) != keys:
        raise ValueError('invalid corpus manifest fields')
    for key, value in dict(kind='trachuan.corpus-release', version=1, repository='dannyota/trachuan').items():
        if type(obj[key]) is not type(value) or obj[key] != value:
            raise ValueError(f'invalid {key}')
    identity(obj['country'], obj['rel'])
    require_match(r'[0-9a-f]{40}', obj['commit'], 'commit')
    require_match(r'[0-9]{8}(?:_[0-9]+)?', obj['previous'], 'previous rel')
    positive(obj['sequence'], 'sequence')
    positive(obj['schema_version'], 'schema version')
    if obj['database'] != f"corpus_{obj['country']}_{obj['rel']}":
        raise ValueError('database differs from release identity')
    dump, report = obj['dump'], obj['report']
    if not isinstance(dump, dict) or set(dump) != {'object', 'sha256', 'size'}:
        raise ValueError('invalid dump fields')
    if dump['object'] != f"{obj['country']}/{obj['rel']}/corpus.dump":
        raise ValueError('invalid dump object')
    require_match(r'[0-9a-f]{64}', dump['sha256'], 'dump hash')
    positive(dump['size'], 'dump size')
    if not isinstance(report, dict) or set(report) != {'sha256', 'quality_pass'} or report['quality_pass'] is not True:
        raise ValueError('invalid report fields')
    require_match(r'[0-9a-f]{64}', report['sha256'], 'report hash')
    require_match(r'[0-9a-f]{64}', obj['build_artifact_sha256'], 'build artifact hash')
    extensions(obj['extensions'])
    recipes = obj['recipes']
    if not isinstance(recipes, dict) or set(recipes) != {'dense', 'chunk', 'lexical', 'retrieval'} or any(not isinstance(value, dict) for value in recipes.values()):
        raise ValueError('invalid recipes')
    return obj


def read_manifest(data):
    obj = validate_manifest(decode(data))
    if canonical(obj) != data:
        raise ValueError('corpus manifest is not canonical JSON')
    return obj


def create(commit, sequence, country, rel, directory, report_path):
    row, report = validate_inputs(country, rel, directory, report_path)
    obj = dict(kind='trachuan.corpus-release', version=1, repository='dannyota/trachuan',
               commit=commit, country=country, rel=rel, database=f'corpus_{country}_{rel}',
               sequence=sequence, previous=report['previous_diff']['previous']['rel'],
               dump=dict(object=f'{country}/{rel}/corpus.dump', sha256=report['dump_sha256'],
                         size=(directory / 'corpus.dump').stat().st_size),
               report=dict(sha256=sha256(report_path), quality_pass=True),
               build_artifact_sha256=report['build_artifact_sha256'],
               schema_version=row['schema_version'], extensions=row['extensions'],
               recipes={name: row[name + '_recipe'] for name in ('dense', 'chunk', 'lexical', 'retrieval')})
    return canonical(validate_manifest(obj))


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('command', choices=['validate', 'create'])
    parser.add_argument('country')
    parser.add_argument('rel')
    parser.add_argument('directory', type=Path)
    parser.add_argument('report', type=Path)
    args = parser.parse_args()
    if args.command == 'validate':
        validate_inputs(args.country, args.rel, args.directory, args.report)
    else:
        if os.environ['GITHUB_REF'] != 'refs/heads/main':
            raise ValueError('dispatch corpus release from main')
        (args.directory / 'manifest.json').write_bytes(create(
            os.environ['GITHUB_SHA'], int(os.environ['GITHUB_RUN_NUMBER']),
            args.country, args.rel, args.directory, args.report))


if __name__ == '__main__':
    main()
