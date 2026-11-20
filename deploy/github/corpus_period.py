import re

from corpus_fixture_history import cohort_of, development_cases
from corpus_floors import check_floors
from corpus_old_product import old_product_partition
from corpus_watermark import MRR_FIELDS

PERIOD = [dict(mode=mode, arms='hybrid', limit=limit, name=name, gating=False)
          for mode in ('exact', 'ann') for name, limit in (('recall', 20), ('recall', 8), ('mrr', 8))]
PERIOD_REQUIRED = {'mode', 'arms', 'limit', 'name', 'value', 'previous_value', 'gating', 'status'}
PERIOD_OPTIONAL = {'non_current_tail', 'not_yet_eligible', 'cap_override', 'article_cap_override', 'reason',
                   'mapping_incomplete_count'}
KEY = ('mode', 'arms', 'limit', 'name')


def fixture_hash(report):
    hashes = report.get('fixture_hashes')
    value = hashes.get('Development') if isinstance(hashes, dict) else None
    if not isinstance(value, str) or not re.fullmatch(r'[0-9a-f]{64}', value):
        raise ValueError('invalid development fixture hash')
    return value


def no_historical_policy(value):
    if isinstance(value, dict):
        for key, child in value.items():
            if key == 'historical_policy' and child is not None:
                raise ValueError('historical_policy research override was removed and is not releasable')
            no_historical_policy(child)
    elif isinstance(value, list):
        for child in value:
            no_historical_policy(child)


def check_record_fields(report):
    for name, cohort in report['cohorts'].items():
        if name == 'development_period' or cohort['case_count'] == 0:
            continue
        for record in cohort.get('non_regression', []):
            gated_mrr = record['gating'] and record['name'] == 'mrr'
            revisable = record['gating'] and name == 'development'
            if (not gated_mrr and any(field in record for field in MRR_FIELDS)) or \
                    (not revisable and 'revision' in record) or \
                    ((not revisable or record['name'] != 'recall') and
                     any(field in record for field in ('numerator', 'denominator', 'previous_numerator', 'previous_denominator'))):
                raise ValueError('non_regression record carries fields of another comparison')


def check_cohorts(report, wanted, counts, label, membership):
    found = {name: set() for name in wanted}
    hybrid = {(name, mode, limit): set() for name in wanted for mode in ('exact', 'ann') for limit in (8, 20)}
    for row in report['cases']:
        if row['cohort'] in found:
            found[row['cohort']].add(row['case_id'])
            if type(row.get('expected')) is not int or row['expected'] != counts.get(row['case_id']):
                raise ValueError(label + ' case records differ from the fixture partition')
            if row.get('arms') == 'hybrid' and row.get('mode') in ('exact', 'ann') and row.get('limit') in (8, 20):
                hybrid[row['cohort'], row['mode'], row['limit']].add(row['case_id'])
    if found != wanted:
        raise ValueError(membership)
    if any(ids != wanted[key[0]] for key, ids in hybrid.items()):
        raise ValueError('a fixture case lacks a hybrid row in a mode and limit of the fixture partition')


def check_partition(report, history):
    fixture = development_cases(history.fixture(fixture_hash(report)))
    wanted = {'development': set(), 'development_period': set()}
    for case in fixture:
        wanted[cohort_of(case)].add(case['id'])
    counts = {case['id']: len(case.get('expected_citations') or ()) for case in fixture}
    check_cohorts(report, wanted, counts, 'development',
                  'development cohorts differ from the fixture partition by the historical tag')
    check_old_product(report, history.root)
    return fixture


def check_old_product(report, root):
    scored, counts = old_product_partition(report, root)
    check_cohorts(report, {'old_product': scored}, counts, 'old_product',
                  'old_product cohort differs from the legacy fixture without its rejections')
    check_floors(report)


def metric_value(report, key):
    cohort = ((report.get('cohorts') or {}).get('development_period') or {})
    for metric in cohort.get('metrics') or []:
        if isinstance(metric, dict) and tuple(metric.get(field) for field in KEY) == key:
            return metric.get('value')
    return None


def same_value(left, right):
    if left is None or right is None:
        return left is None and right is None
    return type(left) is not bool and type(right) is not bool and left == right


def check_period(report, previous=None):
    cohort, not_run = report['cohorts']['development_period'], report.get('not_run')
    if not isinstance(not_run, dict) or 'floors' in cohort or 'targets' in cohort:
        raise ValueError('period cohort has floors, targets or no not_run map')
    if cohort['case_count'] == 0:
        if not_run.get('development_period') != 'not_run' or 'non_regression' in cohort:
            raise ValueError('an empty period cohort needs not_run.development_period and no records')
        return
    if 'development_period' in not_run:
        raise ValueError('a populated period cohort cannot be not_run')
    records = cohort.get('non_regression')
    if not isinstance(records, list) or len(records) != len(PERIOD) or \
            any(not isinstance(record, dict) for record in records):
        raise ValueError('period records differ from the contract')
    keys = {tuple(record.get(field) for field in KEY) for record in records}
    if keys != {tuple(entry[field] for field in KEY) for entry in PERIOD}:
        raise ValueError('period records differ from the contract')
    for record in records:
        key = tuple(record[field] for field in KEY)
        if not PERIOD_REQUIRED <= set(record) <= PERIOD_REQUIRED | PERIOD_OPTIONAL or \
                record['gating'] is not False or record['status'] != 'reported':
            raise ValueError('period record is not a reported, non-gating record')
        if not same_value(record['value'], metric_value(report, key)):
            raise ValueError('period record value differs from the period metric')
        if previous is not None and not same_value(record['previous_value'], metric_value(previous, key)):
            raise ValueError('period record previous_value differs from the previous period metric')
