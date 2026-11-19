import math

MRR_FIELDS = ('previous', 'high_water', 'tolerance', 'within_tolerance', 'losses')
REASONS = ('added', 'changed', 'cohort')


def finite(value):
    return type(value) in (int, float) and math.isfinite(value)


def previous_mrr_record(previous, cohort, key):
    records = ((previous.get('cohorts') or {}).get(cohort) or {}).get('non_regression') or []
    for old in records:
        if isinstance(old, dict) and tuple(old.get(field) for field in ('mode', 'arms', 'limit', 'name')) == key \
                and finite(old.get('high_water')):
            return old
    return None


def tolerance_used(old):
    judged = old.get('value')
    if old.get('revision') is not None:
        judged = old['revision'].get('value')
    if not finite(judged) or old['high_water'] <= judged:
        return 0
    return old['high_water'] - judged


def carried_high_water(previous, cohort, key, prior):
    old = previous_mrr_record(previous, cohort, key)
    if old is None:
        return prior
    if old.get('revision') is not None:
        return prior + tolerance_used(old)
    return max(prior, old['high_water'])


def revised_high_water(previous, key, prior):
    old = previous_mrr_record(previous, 'development', key)
    return prior if old is None else prior + tolerance_used(old)


def validate_previous_revision(value):
    if not isinstance(value, dict) or not finite(value.get('cases')):
        raise ValueError('invalid previous revision cases')
    if 'value' not in value or (value['value'] is not None and not finite(value['value'])):
        raise ValueError('invalid previous revision value')
    revised = value.get('revised')
    if not isinstance(revised, list):
        raise ValueError('invalid previous revision revised')
    for item in revised:
        if not isinstance(item, dict) or not isinstance(item.get('case_id'), str) or not item['case_id'] or \
                item.get('reason') not in REASONS:
            raise ValueError('invalid previous revised case')


def validate_previous_mrr(value):
    if isinstance(value, dict):
        if value.get('name') == 'mrr' and value.get('gating') is True:
            if any(key not in value for key in MRR_FIELDS):
                raise ValueError('previous MRR comparison missing its fields')
            for key in ('previous', 'high_water', 'tolerance'):
                if value[key] is None and value.get('status') != 'pass' and key != 'tolerance':
                    continue
                if not finite(value[key]):
                    raise ValueError('invalid previous MRR ' + key)
            if type(value['within_tolerance']) is not bool or not isinstance(value['losses'], list):
                raise ValueError('invalid previous MRR tolerance fields')
        if 'revision' in value:
            validate_previous_revision(value['revision'])
        for child in value.values():
            validate_previous_mrr(child)
    elif isinstance(value, list):
        for child in value:
            validate_previous_mrr(child)
