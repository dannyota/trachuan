import re

LEGACY_CUTOFF = {'vn': '20261118_1'}


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


def require_status_country(country):
    if country != 'vn':
        raise ValueError('public status does not serve country ' + str(country))
