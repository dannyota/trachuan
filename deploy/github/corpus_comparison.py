import math


TOLERANCE = .005


def versions(report):
    if type(report.get('version')) is not int or report['version'] not in (1, 2):
        raise ValueError('invalid report version')
    allowed = ('agent-eval/1', 'agent-eval/2', 'agent-eval/3', 'agent-eval/4') if report['version'] == 1 else ('agent-eval/5',)
    if report.get('scorer') not in allowed:
        raise ValueError('invalid scorer version')
    if report['version'] == 2 and (not isinstance(report.get('retrieval_recipe'), dict) or not isinstance(report['retrieval_recipe'].get('abstention'), dict)):
        raise ValueError('version 2 requires the full retrieval recipe')


def finite_values(value):
    if isinstance(value, float) and not math.isfinite(value):
        raise ValueError('non-finite report value')
    if isinstance(value, dict):
        for child in value.values():
            finite_values(child)
    elif isinstance(value, list):
        for child in value:
            finite_values(child)


def number(value):
    if type(value) not in (float, int) or not math.isfinite(value):
        raise ValueError('missing or non-finite comparison value')
    return value


def indexed(records, fields):
    if not isinstance(records, list):
        raise ValueError('missing report records')
    out = {}
    for record in records:
        if not isinstance(record, dict) or any(field not in record for field in fields):
            raise ValueError('invalid report record')
        key = tuple(record[field] for field in fields)
        if key in out:
            raise ValueError('duplicate report record')
        out[key] = record
    return out


def record_key(record):
    return tuple(record[field] for field in ('mode', 'arms', 'limit', 'name'))


def metric(report, cohort, key):
    metrics = indexed(report['cohorts'][cohort].get('metrics'), ('mode', 'arms', 'limit', 'name'))
    if key not in metrics:
        raise ValueError('missing gated metric')
    result = metrics[key]
    number(result.get('value'))
    if type(result.get('denominator')) is not int or result['denominator'] < 1:
        raise ValueError('invalid gated denominator')
    return result


def cases(report, cohort):
    rows = report.get('cases')
    if not isinstance(rows, list) or any(not isinstance(row, dict) for row in rows):
        raise ValueError('missing case records')
    result = indexed([row for row in rows if row.get('cohort') == cohort], ('case_id', 'mode', 'arms', 'limit'))
    for row in result.values():
        if type(row.get('expected')) is not int or row['expected'] < 0 or not isinstance(row.get('ranks'), list):
            raise ValueError('invalid case counts or ranks')
        if len(row['ranks']) != row['expected'] or any(type(rank) is not int or rank < 0 for rank in row['ranks']):
            raise ValueError('invalid case ranks')
    return result


def same_recipes(a, b):
    if not all(isinstance(a.get('recipes', {}).get(key), str) and a['recipes'][key] and
               a['recipes'][key] == b.get('recipes', {}).get(key)
               for key in ('query', 'document', 'chunk', 'lexical')):
        return False
    current, previous = a.get('retrieval_recipe'), b.get('retrieval_recipe')
    retrieval_hash = a.get('recipes', {}).get('retrieval')
    if ('retrieval_recipe' not in b and b.get('version') == 1 and isinstance(retrieval_hash, str) and
            retrieval_hash and retrieval_hash == b.get('recipes', {}).get('retrieval')):
        previous = current
    if not all(isinstance(recipe, dict) and isinstance(recipe.get('abstention'), dict) for recipe in (current, previous)):
        return False
    def comparable(recipe):
        return recipe | {'abstention': {key: value for key, value in recipe['abstention'].items() if key != 'calibration_sha256'}}
    def equal(left, right):
        if isinstance(left, dict) and isinstance(right, dict):
            return left.keys() == right.keys() and all(equal(value, right[key]) for key, value in left.items())
        if isinstance(left, list) and isinstance(right, list):
            return len(left) == len(right) and all(equal(x, y) for x, y in zip(left, right))
        if type(left) is bool or type(right) is bool:
            return type(left) is type(right) and left == right
        return left == right
    return equal(comparable(current), comparable(previous))


def compatible(a, b):
    return a == b or {a, b} == {'agent-eval/4', 'agent-eval/5'}


def same_inputs(a, b, cohort, current, previous):
    for key in ('Legacy', 'OldProduct', 'Development'):
        if not a.get('fixture_hashes', {}).get(key) or a['fixture_hashes'][key] != b.get('fixture_hashes', {}).get(key):
            return False
    ac, bc = a['cohorts'][cohort], b['cohorts'][cohort]
    for key in ('case_count', 'positive_count', 'negative_count', 'rejection_count', 'expected_citations'):
        if type(ac.get(key)) is not int or ac[key] != bc.get(key):
            return False
    if not current or current.keys() != previous.keys():
        return False
    if any(row['expected'] != previous[key]['expected'] for key, row in current.items()):
        return False
    fields = ('mode', 'arms', 'limit', 'name')
    am, bm = indexed(ac.get('metrics'), fields), indexed(bc.get('metrics'), fields)
    if am.keys() != bm.keys() or any(row.get('denominator') != bm[key].get('denominator') for key, row in am.items()):
        return False
    ar, br = indexed(a.get('rejections'), ('case_id',)), indexed(b.get('rejections'), ('case_id',))
    return ar.keys() == br.keys()


def alternative(value):
    if not isinstance(value, dict):
        raise ValueError('invalid mapped identity')
    kind = value.get('kind')
    fields = ['country', 'document_key', 'kind']
    if kind in ('provision', 'relation'):
        fields.append('anchor')
    if kind == 'relation':
        fields.extend(('source_key', 'target_key', 'relation_kind', 'evidence_sha256'))
    if kind not in ('document', 'provision', 'relation'):
        raise ValueError('invalid mapped identity kind')
    return tuple(value.get(field) for field in fields)


def preserves(previous, current):
    if not isinstance(previous, list) or not previous or any(not group for group in previous):
        return False
    if not isinstance(current, list):
        return False
    before = {alternative(value) for group in previous for value in group}
    after = {alternative(value) for group in current for value in group}
    return before <= after


def first_rank(row):
    return min((rank for rank in row.get('ranks', []) if rank > 0), default=0)


def reciprocal(rank):
    return 1 / rank if 1 <= rank <= 8 else 0


def losses(current, previous, mode):
    result, preserved = [], True
    for key, row in current.items():
        if key[1:] != (mode, 'hybrid', 8) or key not in previous:
            continue
        before, rank = first_rank(previous[key]), first_rank(row)
        if reciprocal(rank) >= reciprocal(before):
            continue
        preserved = preserved and preserves(previous[key].get('mapped_identities'), row.get('mapped_identities'))
        if rank == 0:
            later = first_rank(current.get((*key[:3], 20), {}))
            rank = later if later > 8 else 0
        result.append(dict(case_id=key[0], previous_rank=before, rank=rank))
    return sorted(result, key=lambda row: row['case_id']), preserved


def verify_metric_cases(metric, key, rows):
    positives = [row for identity, row in rows.items() if identity[1:] == key[:3] and row['expected'] > 0]
    if key[3] == 'mrr':
        denominator = len(positives)
        numerator = 0.
        for row in positives:
            numerator += reciprocal(first_rank(row))
    else:
        denominator = sum(row['expected'] for row in positives)
        numerator = sum(sum(0 < rank <= key[2] for rank in row['ranks']) for row in positives)
    if denominator != metric['denominator'] or denominator == 0:
        raise ValueError('gated denominator differs from cases')
    if metric['value'] != numerator / denominator:
        raise ValueError('gated value differs from case ranks')
    if number(metric.get('numerator')) != numerator:
        raise ValueError('gated numerator differs from case ranks')


def check_mrr_fields(record):
    for key in ('previous', 'high_water', 'tolerance'):
        number(record.get(key))
    if type(record.get('within_tolerance')) is not bool or not isinstance(record.get('losses'), list):
        raise ValueError('missing MRR tolerance fields')
    for loss in record['losses']:
        if not isinstance(loss, dict) or set(loss) != {'case_id', 'previous_rank', 'rank'}:
            raise ValueError('invalid MRR loss fields')
        if not isinstance(loss['case_id'], str) or any(type(loss[key]) is not int or loss[key] < 0 for key in ('previous_rank', 'rank')):
            raise ValueError('invalid MRR loss identity or rank')


def validate_comparisons(report, previous):
    versions(previous)
    finite_values(previous)
    for cohort in ('old_product', 'development'):
        if cohort == 'development' and report['cohorts'][cohort]['case_count'] == 0:
            continue
        records = report['cohorts'][cohort]['non_regression']
        current_cases, prior_cases = cases(report, cohort), cases(previous, cohort)
        prior_records = indexed(previous['cohorts'][cohort].get('non_regression'), ('mode', 'arms', 'limit', 'name'))
        if previous['version'] == 2:
            for old_record in prior_records.values():
                if old_record.get('name') == 'mrr' and old_record.get('gating'):
                    check_mrr_fields(old_record)
        for record in records:
            if not record['gating']:
                continue
            key = record_key(record)
            current, old = metric(report, cohort, key), metric(previous, cohort, key)
            verify_metric_cases(current, key, current_cases)
            verify_metric_cases(old, key, prior_cases)
            value, prior = current['value'], old['value']
            if number(record.get('value')) != value or number(record.get('previous_value')) != prior:
                raise ValueError('comparison differs from metrics')
            threshold = prior
            if record['name'] == 'mrr' and report['version'] == 2:
                check_mrr_fields(record)
                high_water = prior
                equal_recipes = same_recipes(report, previous)
                if previous['version'] == 2:
                    if key not in prior_records:
                        raise ValueError('missing previous MRR comparison')
                    check_mrr_fields(prior_records[key])
                    if equal_recipes:
                        high_water = max(prior, prior_records[key]['high_water'])
                lost, preserved = losses(current_cases, prior_cases, record['mode'])
                tolerance = TOLERANCE if (equal_recipes and compatible(report['scorer'], previous['scorer']) and
                                          same_inputs(report, previous, cohort, current_cases, prior_cases) and preserved) else 0
                expected = dict(previous=prior, high_water=high_water, tolerance=tolerance,
                                within_tolerance=value < prior, losses=lost)
                if any(record.get(field) != wanted for field, wanted in expected.items()):
                    raise ValueError('MRR tolerance fields differ from reports')
                threshold = high_water - tolerance if tolerance else prior
            status = 'pass' if value >= threshold else 'fail'
            if record['status'] != status or status != 'pass':
                raise ValueError('recomputed non_regression did not pass')
        top = indexed(report['non_regression'], ('mode', 'arms', 'limit', 'name'))
        if cohort == 'old_product' and top != indexed(records, ('mode', 'arms', 'limit', 'name')):
            raise ValueError('top-level non_regression differs from old_product')
