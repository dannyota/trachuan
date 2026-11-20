import copy
import json
from pathlib import Path

from corpus_test_support import digest, fixture_repository, git, recipe_fixture

PREVIOUS_REL, CANDIDATE_REL = '20990101_1', '20990102_1'
MODES = ('exact', 'ann')
SCORERS = {2: 'agent-eval/5', 3: 'agent-eval/6'}
LEGACY_PATH = 'apps/corpus/internal/evaluation/testdata/vn/legacy.json'
OLD_PRODUCT_PATH = 'apps/corpus/internal/evaluation/testdata/vn/old-product.json'


def fixture_case(case_id, citations=1, *tags, question=None):
    case = dict(tags=list(tags or ('identifier',)), id=case_id, question=question or 'q-' + case_id)
    if citations:
        case['expected_citations'] = [dict(doc_number='doc') for _ in range(citations)]
    else:
        case['expect_abstain'] = True
    return case


def encode(cases):
    return json.dumps(cases, ensure_ascii=False).encode()


LEGACY_FIXTURE = encode([dict(id=case_id, question='q-' + case_id, expected_citations=[dict(doc_number='doc')])
                         for case_id in ('o1', 'o2', 'r0', 'r1', 'r2')])
OLD_PRODUCT_FIXTURE = encode(dict(legacy_sha256=digest(LEGACY_FIXTURE), rejection_ids=['r0', 'r1', 'r2']))
OLD_RANKS = dict(o1=([1], [1]), o2=([1], [1]))


def case_rows(case_id, cohort, r8, r20):
    return [dict(case_id=case_id, cohort=cohort, mode=mode, arms='hybrid', limit=limit, expected=len(ranks),
                 ranks=list(ranks), mapped_identities=[[dict(country='vn', document_key='doc', kind='document')]
                                                       for _ in ranks])
            for mode in MODES for limit, ranks in ((8, r8), (20, r20))]


def cohort_metrics(rows, cohort):
    out = []
    for mode in MODES:
        for limit, name in ((8, 'recall'), (8, 'mrr'), (20, 'recall')):
            selected = [row for row in rows if (row['cohort'], row['mode'], row['limit']) == (cohort, mode, limit)
                        and row['expected']]
            if name == 'mrr':
                numerator, denominator = 0., len(selected)
                for row in selected:
                    best = min((rank for rank in row['ranks'] if 0 < rank <= 8), default=0)
                    if best:
                        numerator += 1 / best
            else:
                numerator = sum(0 < rank <= limit for row in selected for rank in row['ranks'])
                denominator = sum(row['expected'] for row in selected)
            out.append(dict(mode=mode, arms='hybrid', limit=limit, name=name, numerator=numerator,
                            denominator=denominator, value=numerator / denominator if denominator else None))
    return out


def metric_value(report, cohort, mode, name, limit):
    for metric in report['cohorts'][cohort]['metrics']:
        if (metric['mode'], metric['limit'], metric['name']) == (mode, limit, name):
            return metric['value']
    return None


def gated_records(report, cohort):
    out = []
    for mode in MODES:
        for name, limit, gating in (('recall', 20, True), ('mrr', 8, True), ('abstention_balanced_accuracy', 8, False)):
            value = metric_value(report, cohort, mode, name, limit) if gating else 1.
            record = dict(mode=mode, arms='hybrid', limit=limit, name=name, value=value, previous_value=value,
                          gating=gating, status='pass', non_current_tail=None, not_yet_eligible=False, cap_override=None)
            if name == 'mrr':
                record.update(previous=value, high_water=value, tolerance=.005, within_tolerance=False, losses=[])
            out.append(record)
    return out


def period_records(report):
    return [dict(mode=mode, arms='hybrid', limit=limit, name=name, value=metric_value(report, 'development_period', mode, name, limit),
                 previous_value=None, gating=False, status='reported', non_current_tail=None, not_yet_eligible=False,
                 cap_override=None)
            for mode in MODES for name, limit in (('recall', 20), ('recall', 8), ('mrr', 8))]


def build_report(version, rel, fixture, ranks, cohorts=None, old_ranks=None):
    row, hashes = recipe_fixture()
    report = dict(version=version, scorer=SCORERS[version], country='vn', rel=rel,
                  retrieval_recipe=row['retrieval_recipe'], recipes=hashes, status='complete', quality_pass=True,
                  release_eligible=False,
                  previous_diff={'previous': dict(country='vn', rel=PREVIOUS_REL, report_sha256='a' * 64)},
                  fixture_hashes=dict(Legacy=digest(LEGACY_FIXTURE), OldProduct=digest(OLD_PRODUCT_FIXTURE),
                                      Development=digest(fixture)),
                  rejections=[dict(case_id=f'r{i}', reason='diacritics_required', passed=True) for i in range(3)],
                  not_run={}, cases=[])
    for case_id, case_ranks in (OLD_RANKS | (old_ranks or {})).items():
        report['cases'] += case_rows(case_id, 'old_product', *case_ranks)
    for case in json.loads(fixture):
        cohort = 'development_period' if version >= 3 and 'historical' in case['tags'] else 'development'
        report['cases'] += case_rows(case['id'], (cohorts or {}).get(case['id'], cohort), *ranks[case['id']])
    names = ['legacy', 'old_product', 'development'] + (['development_period'] if version >= 3 else [])
    report['cohorts'] = {'legacy': dict(case_count=0, metrics=[])}
    for name in names[1:]:
        rows = [row for row in report['cases'] if row['cohort'] == name and row['mode'] == 'exact' and row['limit'] == 8]
        rejections = 3 if name == 'old_product' else 0
        report['cohorts'][name] = dict(case_count=len(rows) + rejections, rejection_count=rejections,
                                       positive_count=sum(row['expected'] > 0 for row in rows),
                                       negative_count=sum(row['expected'] == 0 for row in rows),
                                       expected_citations=sum(row['expected'] for row in rows),
                                       metrics=cohort_metrics(report['cases'], name))
    for name in ('old_product', 'development'):
        if report['cohorts'][name]['case_count']:
            report['cohorts'][name]['non_regression'] = gated_records(report, name)
    if version >= 3:
        if report['cohorts']['development_period']['case_count']:
            report['cohorts']['development_period']['non_regression'] = period_records(report)
        else:
            report['not_run']['development_period'] = 'not_run'
    report['non_regression'] = copy.deepcopy(report['cohorts']['old_product']['non_regression'])
    return report


def set_records(report, cohort, name, **fields):
    for record in report['cohorts'][cohort]['non_regression']:
        if record['name'] == name and record['gating'] == (cohort != 'development_period'):
            record.update(copy.deepcopy(fields))
    if cohort == 'old_product':
        report['non_regression'] = copy.deepcopy(report['cohorts']['old_product']['non_regression'])


def link_period(candidate, previous):
    for record in candidate['cohorts']['development_period'].get('non_regression', []):
        record['previous_value'] = metric_value(previous, 'development_period', record['mode'], record['name'], record['limit'])


def drop_fields(report, cohort, name, *fields):
    for record in report['cohorts'][cohort]['non_regression']:
        if record['name'] == name:
            for field in fields:
                record.pop(field, None)


PREVIOUS_CASES = [fixture_case('p1'), fixture_case('p2', 2), fixture_case('p3'), fixture_case('m'),
                  fixture_case('n', 0, 'negative')]
PREVIOUS_RANKS = dict(p1=([2], [2]), p2=([0, 3], [0, 3]), p3=([5], [5]), m=([1], [1]), n=([], []))
CURRENT_CASES = [fixture_case('p1', 1, 'identifier', 'transition'), fixture_case('p2', 2), fixture_case('p3'),
                 fixture_case('m', 1, 'historical'), fixture_case('n', 0, 'negative')]
CURRENT_RANKS = dict(p1=([1], [1]), p2=([0, 4], [12, 4]), p3=([6], [6]), m=([0], [0]), n=([], []))
PRIOR_MRR = (1 / 2 + 1 / 3 + 1 / 5) / 3
CANDIDATE_MRR = (1 + 1 / 4 + 1 / 6) / 3
REVISED = [dict(case_id='m', reason='cohort')]


def revision_pair(previous_cases=None, current_cases=None, previous_ranks=None, current_ranks=None, previous_version=2):
    previous_fixture = encode(PREVIOUS_CASES if previous_cases is None else previous_cases)
    current_fixture = encode(CURRENT_CASES if current_cases is None else current_cases)
    previous = build_report(previous_version, PREVIOUS_REL, previous_fixture,
                            PREVIOUS_RANKS if previous_ranks is None else previous_ranks)
    candidate = build_report(3, CANDIDATE_REL, current_fixture, CURRENT_RANKS if current_ranks is None else current_ranks)
    return previous, candidate, previous_fixture, current_fixture


def base_revision_records(candidate):
    revision = dict(cases=4, revised=REVISED)
    set_records(candidate, 'development', 'recall', previous_value=.75, numerator=4, denominator=4,
                previous_numerator=3, previous_denominator=4, revision=revision | dict(value=1.))
    set_records(candidate, 'development', 'mrr', previous_value=PRIOR_MRR, previous=PRIOR_MRR, high_water=PRIOR_MRR,
                tolerance=.005, within_tolerance=False, revision=revision | dict(value=CANDIDATE_MRR),
                losses=[dict(case_id='p2', previous_rank=3, rank=4), dict(case_id='p3', previous_rank=5, rank=6)])


def add_old_product(root, legacy=LEGACY_FIXTURE, old_product=OLD_PRODUCT_FIXTURE):
    for name, data in ((LEGACY_PATH, legacy), (OLD_PRODUCT_PATH, old_product)):
        if data is not None:
            (root / name).parent.mkdir(parents=True, exist_ok=True)
            (root / name).write_bytes(data)
            git(root, 'add', name)
    git(root, 'commit', '--quiet', '--allow-empty', '-m', 'old product fixtures')
    return root


def history_for(path, *fixtures, legacy=LEGACY_FIXTURE, old_product=OLD_PRODUCT_FIXTURE):
    from corpus_fixture_history import FixtureHistory
    return FixtureHistory(add_old_product(fixture_repository(path, fixtures), legacy, old_product))


def refuses(report, previous, row, history):
    from corpus_manifest import validate_report
    try:
        validate_report(report, 'vn', report['rel'], previous_report=previous, release_row=row, history=history)
    except ValueError:
        return True
    return False


def validate_go_reports(directory):
    from corpus_fixture_history import FixtureHistory
    from corpus_manifest import decode, validate_report
    directory = Path(directory)
    fixtures = [(directory / name).read_bytes() for name in ('fixture-1.json', 'fixture-2.json')]
    legacy, old_product = ((directory / name).read_bytes() for name in ('legacy.json', 'old-product.json'))
    history = FixtureHistory(add_old_product(fixture_repository(directory / 'repository', fixtures), legacy, old_product))
    row = decode((directory / 'release.json').read_bytes())
    pairs = decode((directory / 'pairs.json').read_bytes())
    reports = {name: decode((directory / name / 'report.json').read_bytes()) for pair in pairs for name in pair}
    if reports['r0']['version'] != 2:
        raise AssertionError('the first previous report is not version 2')
    for name, previous in pairs:
        validate_report(reports[name], 'vn', reports[name]['rel'], previous_report=reports[previous], release_row=row,
                        history=history)
    tampers = [('r1', 'development', 'mrr', lambda r: r.update(within_tolerance=False)),
               ('r1', 'development', 'mrr', lambda r: r.update(losses=[])),
               ('r2', 'development', 'mrr', lambda r: r['revision'].update(value=r['revision']['value'] - 1e-9)),
               ('r2', 'development', 'mrr', lambda r: r['revision'].update(revised=[])),
               ('r2', 'development', 'mrr', lambda r: r.update(high_water=r['previous'])),
               ('r2', 'development', 'recall', lambda r: r.update(previous_numerator=r['previous_numerator'] + 1)),
               ('r3', 'development', 'mrr', lambda r: r.update(high_water=r['high_water'] + 1e-9)),
               ('r4', 'development', 'mrr', lambda r: r.update(high_water=r['previous'])),
               ('r4', 'development_period', 'mrr', lambda r: r.update(previous_value=None))]
    previous = dict(pairs)
    for name, cohort, metric, mutate in tampers:
        report = copy.deepcopy(reports[name])
        for record in report['cohorts'][cohort]['non_regression']:
            if (record['mode'], record['name']) == ('exact', metric):
                mutate(record)
        if not refuses(report, reports[previous[name]], row, history):
            raise AssertionError(f'tampered {name} {cohort} {metric} record accepted')
    for tampered in ('r1', 'r0'):
        pair = {name: copy.deepcopy(reports[name]) for name in ('r1', 'r0')}
        pair[tampered]['rejections'][0]['passed'] = False
        if not refuses(pair['r1'], pair['r0'], row, history):
            raise AssertionError(f'{tampered} rejection that did not pass accepted')
    return ' '.join(name for name, _ in pairs)
