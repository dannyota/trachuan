import copy
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys

from corpus_manifest import create, non_regression_contract
from corpus_rules import DECISION_FIELDS
from manifest import canonical

DEVELOPMENT_FIXTURE = 'apps/corpus/internal/evaluation/testdata/vn/development.json'


FAKE_CURL = '''#!{python}
import json
import os
import sys
with open({state!r}) as stream:
    state = json.load(stream)
with open({log!r}, 'a') as stream:
    stream.write(json.dumps(dict(name='curl', args=sys.argv[1:], run='', checksum=None, config='',
                                 environment=sorted(os.environ))) + '\\n')
sys.stdout.buffer.write(bytes.fromhex(state['body']) + state['code'].encode())
sys.stdout.buffer.flush()
sys.exit(state['exit'])
'''


def digest(data):
    return hashlib.sha256(data).hexdigest()


class FakeCurl:
    def __init__(self, directory, log):
        self.state = directory / 'curl-state.json'
        script = directory / 'curl'
        script.write_text(FAKE_CURL.format(python=sys.executable, state=str(self.state), log=str(log)))
        script.chmod(0o755)
        self.answer()

    def answer(self, code='200', body=None, release='20990101_1', exit_code=0):
        data = json.dumps(dict(release=release)).encode() if body is None else body
        self.state.write_text(json.dumps(dict(code=code, body=data.hex(), exit=exit_code)))


def retrieval_recipe():
    return dict(
        abstention=dict(calibration_sha256='49c5e01c3518df3e843e2832d29b1480e63a8b21821fb2683685e2ee5908b905',
                        statistic='first-group-best-member-cosine', status='provisional-old-product',
                        threshold=0.5297041131124263),
        ann=dict(ef_max=1000, fallback='exact', iterative_scan='strict_order', max_scan_tuples=20000,
                 plan_cache_mode='force_custom_plan', pool_min=400, pool_multiplier=24, scan_mem_multiplier=1),
        backfill='demoted-order', context_scope='number-routed/1', default_in_force_only=True,
        dense_depth=50, document_cap=5, evidence='source-spans/1', evidence_rune_cap=14000,
        family_collapse='off-unverified-alignment', group='nearest-khoan', hydrate_multiplier=8,
        lexical_depth=50, non_current_tail=2, query_gate='vn-marks/1', reducer='max', relation_cap=20,
        relation_evidence_rune_cap=1000, routing='vn-number/1',
        rrf=dict(dense_weight=1, k=60, lexical_weight=0.5, routed_lexical_weight=1),
        sibling_merge='article-clauses/1', ties='score-id', validity='document-dates/3', version=3)


def recipe_fixture(retrieval=None, chunk=None):
    recipes = dict(document={'model': 'synthetic', 'version': 1, 'dimensions': 128},
                   query={'model': 'synthetic', 'version': 1, 'dimensions': 128},
                   chunk={'version': 1} if chunk is None else copy.deepcopy(chunk),
                   lexical={'version': 1}, retrieval=retrieval_recipe() if retrieval is None else copy.deepcopy(retrieval))
    inputs = {name: json.dumps(recipe, separators=(',', ':')) for name, recipe in recipes.items()}
    hashes = {name: digest(value.encode()) for name, value in inputs.items()}
    row = dict(dense_recipe=dict(version=1, model='synthetic', dimensions=128,
                                document=recipes['document'], query=recipes['query'],
                                document_hash=hashes['document'], query_hash=hashes['query']),
               chunk_recipe=recipes['chunk'], lexical_recipe=recipes['lexical'],
               retrieval_recipe=recipes['retrieval'], recipe_hash_inputs=inputs)
    return row, hashes


def write_fixture(path):
    dump = b'synthetic corpus dump\n'
    manifest = (digest(dump) + '  corpus.dump\n').encode()
    row, hashes = recipe_fixture()
    row.update(country='vn', rel='20990102_1', schema_version=6,
               extensions={'vector': '0.8.1', 'ltree': '1.3'},
               run=dict(complete=True, expected_items=5, incomplete_items=0,
                        unaccepted_gaps=0, build_artifact_sha256=digest(manifest)))
    comparisons = [entry | {'status': 'pass', 'value': 1., 'previous_value': 1.} for entry in non_regression_contract()['old_product']]
    for record in comparisons:
        if record['name'] == 'mrr':
            record.update(previous=1., high_water=1., tolerance=.005, within_tolerance=False, losses=[])
    report = dict(version=2, scorer='agent-eval/5', retrieval_recipe=copy.deepcopy(row['retrieval_recipe']),
                  country=row['country'], rel=row['rel'], status='complete',
                  quality_pass=True, release_eligible=False, dump_sha256=digest(dump),
                  build_artifact_sha256=digest(manifest),
                  previous_diff={'previous': dict(country='vn', rel='20990101_1', report_sha256='b' * 64)},
                  recipes=hashes,
                  non_regression=[item.copy() for item in comparisons],
                  cohorts={'legacy': {'case_count': 5},
                           'old_product': {'case_count': 5, 'non_regression': [item.copy() for item in comparisons]},
                           'development': {'case_count': 2, 'non_regression': [item.copy() for item in comparisons]}})
    report['fixture_hashes'] = dict(Legacy='legacy', OldProduct='old', Development='development')
    report['rejections'] = [dict(case_id=f'r{i}', passed=True) for i in range(3)]
    report['cases'] = []
    for name in ('old_product', 'development'):
        cohort = report['cohorts'][name]
        count = cohort['case_count'] - (3 if name == 'old_product' else 0)
        cohort.update(positive_count=count, negative_count=0, rejection_count=3 if name == 'old_product' else 0, expected_citations=count)
        cohort['metrics'] = [dict(mode=mode, arms='hybrid', limit=limit, name=metric,
                                   value=1., numerator=count, denominator=count)
                             for mode in ('exact', 'ann') for metric, limit in (('mrr', 8), ('recall', 20))]
        report['cases'] += [dict(case_id=str(i), cohort=name, mode=mode, arms='hybrid', limit=limit,
                                 expected=1, ranks=[1], mapped_identities=[[dict(country='vn', document_key='d', kind='document')]])
                            for mode in ('exact', 'ann') for limit in (8, 20) for i in range(count)]
    report['cases'] += [case | {'cohort': 'legacy'} for case in report['cases'] if case['cohort'] == 'old_product']
    report['cases'] += [dict(case_id=rejection['case_id'], cohort='legacy', mode=mode, arms='hybrid', limit=limit,
                              expected=1, ranks=[1])
                        for mode in ('exact', 'ann') for limit in (8, 20) for rejection in report['rejections']]
    prior = copy.deepcopy(report)
    prior['rel'] = '20990101_1'
    previous_data = json.dumps(prior).encode()
    (path / 'previous-report.json').write_bytes(previous_data)
    report['previous_diff']['previous']['report_sha256'] = digest(previous_data)
    (path / 'corpus.dump').write_bytes(dump)
    (path / 'MANIFEST.sha256').write_bytes(manifest)
    save_fixture(path, row, report)
    manifest = json.loads(create('a' * 40, 1, 'vn', row['rel'], path, path / 'report.json'))
    manifest.pop('evidence', None)
    manifest.update(rel='20990101_1', database='corpus_vn_20990101_1', previous='20981231_1')
    manifest['dump']['object'] = 'vn/20990101_1/corpus.dump'
    manifest['report']['sha256'] = digest(previous_data)
    (path / 'previous-manifest.json').write_bytes(canonical(manifest))
    return row, report


def evidence_bindings(path, row, report_bytes):
    return {'rel': row.get('rel'), 'dump_sha256': 'sha256:' + digest((path / 'corpus.dump').read_bytes()),
            'build_artifact_sha256': 'sha256:' + digest((path / 'MANIFEST.sha256').read_bytes()),
            'report_sha256': 'sha256:' + digest(report_bytes)}


def previous_rel(report):
    return ((report.get('previous_diff') or {}).get('previous') or {}).get('rel', 'none')


def findings_fixture(path, row, report_bytes, report, predecessor='none', detectors=None, findings=()):
    bindings = evidence_bindings(path, row, report_bytes) | {
        'detector_registry_sha256': 'sha256:' + 'f' * 64, 'predecessor_rel': previous_rel(report),
        'predecessor_findings_sha256': predecessor}
    if detectors is None:
        detectors = {'unmatched_operative_claim': dict(complete=True, severity='review', observed=3, not_observable=0, excluded=0),
                     'partial_without_provision_effect': dict(complete=True, severity='info', observed=2, not_observable=1, excluded=0)}
    return dict(schema=2, bindings=bindings, detectors=detectors, findings=list(findings),
                new=[], gone=[], carried=[], unmatched=[])


def jsonl(records):
    return b''.join(json.dumps(record, ensure_ascii=False, separators=(',', ':')).encode() + b'\n' for record in records)


def write_evidence(path, row, report, findings=None, identity=None, changes=None, predecessor_identity='none'):
    report_bytes = (path / 'report.json').read_bytes()
    base = evidence_bindings(path, row, report_bytes)
    if findings is None:
        findings = findings_fixture(path, row, report_bytes, report)
    if identity is None:
        identity = [dict(schema=1, bindings=base),
                    dict(type='document', key='vn:vbpl:2099001', number_norm='1/2099/TT-NHNN', status_class='in_force',
                         eff_from='2099-01-01', eff_to=None, is_current=True, validity_conflict=False,
                         text_source='source_tree', text_complete=True)]
    if changes is None:
        changes = [dict(schema=1, bindings=base | {'predecessor_rel': previous_rel(report),
                                                   'predecessor_identity_sha256': predecessor_identity},
                        baseline=predecessor_identity == 'none')]
    files = {'findings.json': json.dumps(findings, indent=2).encode() + b'\n',
             'identity.jsonl': identity if isinstance(identity, bytes) else jsonl(identity),
             'changes.jsonl': changes if isinstance(changes, bytes) else jsonl(changes)}
    for name, data in files.items():
        (path / name).write_bytes(data)
    if isinstance(row.get('run'), dict):
        row['run'].update(findings_sha256=digest(files['findings.json']), identity_sha256=digest(files['identity.jsonl']),
                          changes_sha256=digest(files['changes.jsonl']))
    (path / 'release.json').write_text(json.dumps(row))


def save_fixture(path, row, report):
    data = json.dumps(report).encode()
    if isinstance(row.get('run'), dict):
        previous = (report.get('previous_diff') or {}).get('previous') or {}
        row['run'].update(eval_report_sha256=digest(data),
                          previous_report_sha256=previous.get('report_sha256', 'none'))
    (path / 'release.json').write_text(json.dumps(row))
    (path / 'report.json').write_bytes(data)
    if (path / 'corpus.dump').exists() and (path / 'MANIFEST.sha256').exists():
        write_evidence(path, row, report)


def save_older_shape(path, row):
    older = copy.deepcopy(row)
    for field in DECISION_FIELDS:
        older['run'].pop(field, None)
    (path / 'release.json').write_text(json.dumps(older))


def add_heading_build(path, row, report):
    data = (Path(__file__).parent / 'testdata/heading-build.json').read_bytes()
    (path / 'build.json').write_bytes(data)
    manifest = (digest((path / 'corpus.dump').read_bytes()) + '  corpus.dump\n' +
                digest(data) + '  build.json\n').encode()
    (path / 'MANIFEST.sha256').write_bytes(manifest)
    row['schema_version'] = 8
    row['run']['build_artifact_sha256'] = report['build_artifact_sha256'] = digest(manifest)
    save_fixture(path, row, report)
    return json.loads(data)


def git(root, *args):
    env = {key: value for key, value in os.environ.items() if not key.startswith('GIT_')}
    env.update(GIT_CONFIG_GLOBAL=os.devnull, GIT_CONFIG_NOSYSTEM='1', GIT_AUTHOR_NAME='Test',
               GIT_AUTHOR_EMAIL='test@example.invalid', GIT_COMMITTER_NAME='Test',
               GIT_COMMITTER_EMAIL='test@example.invalid')
    return subprocess.run(['git', '-c', 'commit.gpgsign=false', '-C', str(root), *args],
                          check=True, capture_output=True, env=env).stdout


def fixture_repository(root, versions, branch='main'):
    root = Path(root)
    root.mkdir(parents=True, exist_ok=True)
    git(root, 'init', '--quiet', '--initial-branch', branch)
    path = root / DEVELOPMENT_FIXTURE
    for index, data in enumerate(versions):
        if data is None:
            git(root, 'rm', '--quiet', DEVELOPMENT_FIXTURE)
        else:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(data)
            git(root, 'add', DEVELOPMENT_FIXTURE)
        git(root, 'commit', '--quiet', '--allow-empty', '-m', f'version {index}')
    return root
