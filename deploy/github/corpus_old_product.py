import json
import re

from corpus_fixture_history import CASE_FIELDS, CITATION_FIELDS, FixtureHistory, unique_object

LEGACY = 'apps/corpus/internal/evaluation/testdata/vn/legacy.json'
OLD_PRODUCT = 'apps/corpus/internal/evaluation/testdata/vn/old-product.json'
REJECTION_FIELDS = {'case_id', 'reason', 'passed'}


def decode(data, name):
    def invalid_constant(value):
        raise ValueError(f'invalid JSON constant in {name} fixture')
    return json.loads(data, object_pairs_hook=unique_object, parse_constant=invalid_constant)


def blank(value):
    return not isinstance(value, str) or not value.strip()


def optional(value, kind):
    return value is None or type(value) is kind


def check_citation(citation, case_id):
    if not isinstance(citation, dict) or not set(citation) <= CITATION_FIELDS or blank(citation.get('doc_number')) or \
            any(not optional(citation.get(field), str) for field in ('article', 'clause', 'point')) or \
            not optional(citation.get('alt_doc_numbers'), list) or \
            any(blank(number) for number in citation.get('alt_doc_numbers') or ()) or \
            not optional(citation.get('relation_ok'), bool):
        raise ValueError('invalid legacy fixture citation: ' + case_id)


def legacy_cases(data):
    cases = decode(data, 'legacy')
    if not isinstance(cases, list) or not cases:
        raise ValueError('legacy fixture is not a non-empty list')
    seen = set()
    for case in cases:
        if not isinstance(case, dict) or not set(case) <= CASE_FIELDS:
            raise ValueError('invalid legacy fixture case fields')
        case_id, tags, citations = case.get('id'), case.get('tags'), case.get('expected_citations')
        if blank(case_id) or case_id in seen or blank(case.get('question')):
            raise ValueError('invalid or duplicate legacy fixture id or empty question')
        seen.add(case_id)
        if not optional(tags, list) or any(not isinstance(tag, str) for tag in tags or ()) or \
                not optional(citations, list) or not optional(case.get('notes'), str) or \
                any(not optional(case.get(field), bool) for field in ('expect_abstain', 'expect_fail')):
            raise ValueError('invalid legacy fixture case: ' + case_id)
        if bool(case.get('expect_abstain')) == bool(citations):
            raise ValueError('contradictory or missing legacy fixture expectations: ' + case_id)
        for citation in citations or ():
            check_citation(citation, case_id)
    return cases


def rejection_ids(data, legacy_hash, cases):
    old = decode(data, 'old product')
    if not isinstance(old, dict) or set(old) != {'legacy_sha256', 'rejection_ids'}:
        raise ValueError('invalid old product fixture fields')
    if old['legacy_sha256'] != legacy_hash:
        raise ValueError('old product fixture legacy_sha256 differs from the legacy fixture hash')
    ids, legacy = old['rejection_ids'], {case['id'] for case in cases}
    if not isinstance(ids, list) or len(ids) != 3 or \
            any(not isinstance(case_id, str) or case_id not in legacy for case_id in ids) or len(set(ids)) != 3:
        raise ValueError('old product fixture rejection_ids are not three distinct legacy cases')
    return ids


def fixture_hashes(report):
    hashes = report.get('fixture_hashes')
    values = [hashes.get(key) if isinstance(hashes, dict) else None for key in ('Legacy', 'OldProduct')]
    if any(not isinstance(value, str) or not re.fullmatch(r'[0-9a-f]{64}', value) for value in values):
        raise ValueError('invalid legacy or old product fixture hash')
    return values


def old_product_partition(report, root):
    legacy_hash, old_hash = fixture_hashes(report)
    cases = legacy_cases(FixtureHistory(root, LEGACY).fixture(legacy_hash))
    rejected = rejection_ids(FixtureHistory(root, OLD_PRODUCT).fixture(old_hash), legacy_hash, cases)
    rows = report['rejections']
    if sorted(row['case_id'] for row in rows) != sorted(rejected):
        raise ValueError('report rejections differ from the old product fixture rejection_ids')
    if any(set(row) != REJECTION_FIELDS or row['passed'] is not True or row['reason'] != 'diacritics_required'
           for row in rows):
        raise ValueError('rejection record is not a passed diacritics_required refusal')
    counts = {case['id']: len(case.get('expected_citations') or ()) for case in cases}
    return {case_id for case_id in counts if case_id not in rejected}, counts
