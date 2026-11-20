from collections import namedtuple

from corpus_comparison import (TOLERANCE, cases, check_mrr_fields, compatible, indexed, losses, metric, number,
                               record_key, same, same_recipes, validate_comparisons, verify_metric_cases, versions)
from corpus_fixture_history import cohort_of, content, development_cases
from corpus_period import check_period, fixture_hash, no_historical_policy
from corpus_watermark import MRR_FIELDS, revised_high_water, validate_previous_mrr

Revision = namedtuple('Revision', 'comparable previous_order revised')
Comparable = namedtuple('Comparable', 'value numerator denominator')
COUNT_FIELDS = ('numerator', 'denominator', 'previous_numerator', 'previous_denominator')


def previous_cohorts(previous):
    rows = previous.get('cases')
    if not isinstance(rows, list):
        raise ValueError('missing case records')
    out = {}
    for row in rows:
        if not isinstance(row, dict) or not isinstance(row.get('case_id'), str) or not row['case_id']:
            raise ValueError('invalid case identity or cohort')
        cohort = row.get('cohort')
        if cohort not in ('development', 'development_period'):
            continue
        if out.setdefault(row['case_id'], cohort) != cohort:
            raise ValueError(f"previous report holds case {row['case_id']} in two development cohorts")
    return out


def compare_fixtures(previous, current, cohorts):
    before = {}
    for case in previous:
        if case['id'] not in cohorts:
            raise ValueError(f"previous report has no case records for development case {case['id']}")
        before[case['id']] = case
    if len(cohorts) != len(before):
        raise ValueError('previous report holds development cases outside its development fixture')
    after = {case['id'] for case in current}
    for case in previous:
        if case['id'] not in after:
            raise ValueError(f"development fixture revision removes case {case['id']}")
    comparable, revised = [], []
    for case in current:
        cohort, old = cohort_of(case), before.get(case['id'])
        if old is None:
            reason = 'added'
        elif cohorts[case['id']] != cohort:
            reason = 'cohort'
        else:
            reason = None if content(old) == content(case) else 'changed'
        if reason is None:
            if cohort == 'development':
                comparable.append(case['id'])
        elif cohort == 'development' or cohorts.get(case['id']) == 'development':
            revised.append(dict(case_id=case['id'], reason=reason))
    chosen = set(comparable)
    return Revision(comparable, [case['id'] for case in previous if case['id'] in chosen],
                    sorted(revised, key=lambda item: item['case_id']))


def resolve_revision(report, previous, history, fixture):
    cohorts = previous_cohorts(previous)
    current, prior = fixture_hash(report), fixture_hash(previous)
    history.require_order(current, prior)
    if not cohorts:
        return None
    if current == prior:
        for case in fixture:
            if cohorts.get(case['id'], cohort_of(case)) != cohort_of(case):
                raise ValueError(f"case {case['id']} changed development cohort under an unchanged development fixture")
        return None
    prior_fixture = development_cases(history.fixture(prior))
    counts = {case['id']: len(case.get('expected_citations') or ()) for case in prior_fixture}
    for row in previous['cases']:
        if cohorts.get(row['case_id']) == row.get('cohort') and row['case_id'] in counts:
            count = counts[row['case_id']]
            if type(row.get('expected')) is not int or row['expected'] != count or \
                    not isinstance(row.get('ranks'), list) or len(row['ranks']) != count:
                raise ValueError(f"previous report case {row['case_id']} differs from its development fixture")
    return compare_fixtures(prior_fixture, fixture, cohorts)


def comparable_metric(rows, ids, mode, name, limit):
    matched = total = positive = 0
    mrr = 0.
    for case_id in ids:
        row = rows.get((case_id, mode, 'hybrid', limit))
        if row is None or row.get('mapping_issues'):
            return None
        if row['expected'] == 0:
            continue
        positive += 1
        best = 0
        for rank in row['ranks']:
            if 0 < rank <= limit:
                matched += 1
            if 0 < rank <= 8 and (best == 0 or rank < best):
                best = rank
        total += len(row['ranks'])
        if best > 0:
            mrr += 1 / best
    if name == 'mrr' and positive > 0:
        return Comparable(mrr / positive, matched, total)
    if name == 'recall' and total > 0:
        return Comparable(matched / total, matched, total)
    return None


def same_revised_cohort(report, previous, ids, current, prior):
    if not current or not prior:
        return False
    chosen = {key for key in current if key[0] in ids}
    if chosen != {key for key in prior if key[0] in ids}:
        return False
    if any(current[key]['expected'] != prior[key]['expected'] for key in chosen):
        return False
    return indexed(report.get('rejections'), ('case_id',)).keys() == indexed(previous.get('rejections'), ('case_id',)).keys()


def revision_fields(report, previous, revision, key, current, prior):
    mode, _, limit, name = key
    candidate = comparable_metric(current, revision.comparable, mode, name, limit)
    old = comparable_metric(prior, revision.previous_order, mode, name, limit)
    if candidate is None or old is None:
        raise ValueError('revised comparison has no comparable value')
    fields = dict(previous_value=old.value,
                  revision=dict(cases=len(revision.comparable), value=candidate.value, revised=revision.revised))
    if name == 'recall':
        fields.update(numerator=candidate.numerator, denominator=candidate.denominator,
                      previous_numerator=old.numerator, previous_denominator=old.denominator)
        passed = candidate.denominator == old.denominator and candidate.numerator >= old.numerator
        return fields, passed
    ids = set(revision.comparable)
    equal_recipes = same_recipes(report, previous)
    high_water = revised_high_water(previous, key, old.value) if previous['version'] >= 2 and equal_recipes else old.value
    lost, preserved = losses(current, prior, mode, ids)
    tolerance = TOLERANCE if (equal_recipes and compatible(previous['scorer'], report['scorer']) and
                              same_revised_cohort(report, previous, ids, current, prior) and preserved) else 0
    fields.update(previous=old.value, high_water=high_water, tolerance=tolerance,
                  within_tolerance=candidate.value < old.value, losses=lost)
    threshold = high_water - tolerance if tolerance else old.value
    return fields, candidate.value >= threshold


def check_revision_records(report, previous, revision):
    if report['cohorts']['development']['case_count'] == 0:
        return
    current, prior = cases(report, 'development'), cases(previous, 'development')
    for record in report['cohorts']['development']['non_regression']:
        if not record['gating']:
            continue
        key = record_key(record)
        values = metric(report, 'development', key)
        verify_metric_cases(values, key, current)
        if number(record.get('value')) != values['value']:
            raise ValueError('comparison differs from metrics')
        fields, passed = revision_fields(report, previous, revision, key, current, prior)
        forbidden = MRR_FIELDS if record['name'] == 'recall' else COUNT_FIELDS
        if record['name'] == 'mrr':
            check_mrr_fields(record)
        if any(field in record for field in forbidden) or not isinstance(record.get('revision'), dict) or \
                type(record['revision'].get('cases')) is not int or \
                any(type(record.get(field)) is not int for field in COUNT_FIELDS if field in fields) or \
                any(not same(record.get(field), wanted) for field, wanted in fields.items()):
            raise ValueError('revised comparison differs from reports')
        if not passed or record['status'] != 'pass':
            raise ValueError('recomputed non_regression did not pass')


def validate_version_three(report, previous, history, fixture):
    versions(previous)
    if not compatible(previous['scorer'], report['scorer']):
        raise ValueError('incompatible previous scorer for a version 3 report')
    fixture_hash(previous)
    if any(previous['fixture_hashes'].get(key) != report['fixture_hashes'].get(key) for key in ('Legacy', 'OldProduct')):
        raise ValueError('incompatible previous legacy or old product fixture')
    no_historical_policy(previous)
    validate_previous_mrr(previous)
    revision = resolve_revision(report, previous, history, fixture)
    validate_comparisons(report, previous, revised=revision is not None)
    if revision is not None:
        check_revision_records(report, previous, revision)
    check_period(report, previous)
