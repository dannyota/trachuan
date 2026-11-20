import math

PRECISION_FLOOR = 0.99
MODES = ('exact', 'ann')
REJECTION_COUNTS = {'legacy': 0, 'old_product': 3, 'development': 0, 'development_period': 0}


def number(value):
    return type(value) in (int, float) and math.isfinite(value)


METRIC_REQUIRED = {'mode', 'arms', 'limit', 'name', 'numerator', 'denominator', 'value'}
METRIC_OPTIONAL = {'non_current_tail', 'not_yet_eligible', 'cap_override', 'article_cap_override',
                   'historical_policy', 'mapping_incomplete_count'}
FLOOR_FIELDS = {'floor', 'status'}
OVERRIDES = ('cap_override', 'article_cap_override', 'historical_policy', 'non_current_tail')


def check_types(record, floor=False):
    fields = set(record)
    allowed = METRIC_REQUIRED | METRIC_OPTIONAL | (FLOOR_FIELDS if floor else set())
    required = METRIC_REQUIRED | (FLOOR_FIELDS if floor else set())
    if not required <= fields <= allowed or \
            type(record['limit']) is not int or type(record['denominator']) is not int or \
            not number(record['numerator']) or record['value'] is not None and not number(record['value']) or \
            any(not isinstance(record[key], str) for key in ('name', 'arms', 'mode')) or \
            any(record.get(key) is not None for key in OVERRIDES) or \
            record.get('not_yet_eligible', False) is not False or \
            type(record.get('mapping_incomplete_count', 0)) is not int:
        raise ValueError('floor record fields or types differ from the evaluator contract')


def matching_metrics(cohort, mode):
    return [metric for metric in cohort.get('metrics') or [] if isinstance(metric, dict) and
            (metric.get('name'), metric.get('arms'), metric.get('limit'), metric.get('mode')) ==
            ('current_law_precision', 'hybrid', 8, mode)]


def precision_metric(cohort, mode):
    found = matching_metrics(cohort, mode)
    if len(found) == 1:
        check_types(found[0])
    if len(found) != 1:
        raise ValueError('current_law_precision metric missing or repeated for a floor')
    return found[0]


def check_value(metric):
    numerator, denominator, value = metric.get('numerator'), metric.get('denominator'), metric.get('value')
    if not number(numerator) or numerator != int(numerator) or type(denominator) is not int or denominator <= 0 or \
            not 0 <= numerator <= denominator or not number(value) or value != numerator / denominator:
        raise ValueError('current_law_precision floor value is not its numerator over its denominator')
    if value < PRECISION_FLOOR:
        raise ValueError('current_law_precision is below the floor')


def check_empty(metric, cohort, mode):
    numerator = metric.get('numerator')
    if not number(numerator) or numerator != 0 or metric.get('denominator') != 0 or metric.get('value') is not None or \
            matching_metrics(cohort, mode):
        raise ValueError('not_run floor is not an empty current_law_precision record')


def check_cohort_floors(cohort, status):
    floors = cohort.get('floors')
    if not isinstance(floors, list) or len(floors) != len(MODES) or any(not isinstance(floor, dict) for floor in floors):
        raise ValueError('floor records differ from the evaluator contract')
    for floor, mode in zip(floors, MODES):
        stated = floor.get('floor')
        if type(stated) not in (int, float) or stated != PRECISION_FLOOR or floor.get('status') != status:
            raise ValueError(f'floor {mode} is not a {status} current_law_precision floor at {PRECISION_FLOOR}')
        check_types(floor, floor=True)
        metric = {key: value for key, value in floor.items() if key not in ('floor', 'status')}
        key = (metric.get('name'), metric.get('arms'), metric.get('limit'), metric.get('mode'))
        if key != ('current_law_precision', 'hybrid', 8, mode):
            raise ValueError('floor records differ from the evaluator contract')
        if status == 'pass':
            if metric != precision_metric(cohort, mode):
                raise ValueError('floor record differs from its cohort metric')
            check_value(metric)
        else:
            check_empty(metric, cohort, mode)


def check_floors(report):
    cohorts = report['cohorts']
    for name, cohort in cohorts.items():
        count = cohort.get('rejection_count') if isinstance(cohort, dict) else None
        if name in REJECTION_COUNTS and (type(count) is not int or count != REJECTION_COUNTS[name]):
            raise ValueError(f'{name} rejection_count differs from the evaluator')
    old_product = cohorts['old_product']
    check_cohort_floors(old_product, 'pass')
    top = report.get('floors')
    if not isinstance(top, list) or any(not isinstance(floor, dict) for floor in top):
        raise ValueError('top-level floors differ from the old_product floors')
    for floor in top:
        check_types(floor, floor=True)
    if top != old_product.get('floors'):
        raise ValueError('top-level floors differ from the old_product floors')
    development = cohorts['development']
    check_cohort_floors(development, 'pass' if development['case_count'] else 'not_run')
