import re

LEGACY_CUTOFF = {'vn': '20261118_2'}
DECISION_FIELDS = ('eval_report_sha256', 'previous_report_sha256', 'findings_sha256', 'identity_sha256', 'changes_sha256')


def rel_order(rel):
    match = re.fullmatch(r'([0-9]{8})(?:_([0-9]+))?', rel) if isinstance(rel, str) else None
    if match is None:
        raise ValueError('invalid rel')
    return int(match[1]), int(match[2] or 0)


def older_shape_allowed(country, rel, signing):
    cutoff = LEGACY_CUTOFF.get(country)
    if cutoff is None:
        return False
    return rel_order(rel) <= rel_order(cutoff) if signing else rel == cutoff


def has_decision(run):
    return any(field in run for field in DECISION_FIELDS)


def first_signing_allowed(country, rel, run):
    return has_decision(run) or rel == LEGACY_CUTOFF.get(country)


def require_status_country(country):
    if country != 'vn':
        raise ValueError('public status does not serve country ' + str(country))
