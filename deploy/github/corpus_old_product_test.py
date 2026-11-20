import copy
import json
from pathlib import Path
import tempfile
import unittest

from corpus_fixture_history import REPOSITORY
from corpus_manifest import validate_report
from corpus_old_product import LEGACY, OLD_PRODUCT, legacy_cases, rejection_ids
from corpus_revision_support import (CANDIDATE_REL, CURRENT_CASES, CURRENT_RANKS, LEGACY_FIXTURE, OLD_PRODUCT_FIXTURE,
                                     PREVIOUS_REL, build_report, cohort_metrics, encode, history_for, link_period,
                                     metric_value)
from corpus_test_support import digest, recipe_fixture

FIXTURE = encode(CURRENT_CASES)


def recompute(candidate, previous):
    candidate['cohorts']['old_product']['metrics'] = cohort_metrics(candidate['cases'], 'old_product')
    for record in candidate['cohorts']['old_product']['non_regression']:
        if not record['gating']:
            continue
        key = (record['mode'], record['name'], record['limit'])
        value, prior = metric_value(candidate, 'old_product', *key), metric_value(previous, 'old_product', *key)
        record.update(value=value, previous_value=prior)
        if record['name'] == 'mrr':
            record.update(previous=prior, high_water=prior, tolerance=0, within_tolerance=value < prior, losses=[])
    candidate['non_regression'] = copy.deepcopy(candidate['cohorts']['old_product']['non_regression'])


def forge(candidate, previous, drop, add=(), keep=False):
    dropped = [row for row in candidate['cases'] if drop(row)]
    candidate['cases'] = [row for row in candidate['cases'] if not drop(row)] + [copy.deepcopy(row) for row in add]
    recompute(candidate, previous)
    if keep:
        candidate['cases'].append(dropped[0] | dict(arms='dense'))


def old_rows(report, case_id):
    return [row for row in report['cases'] if (row['cohort'], row['case_id']) == ('old_product', case_id)]


def old_product(legacy_sha256=None, rejection_ids=('r0', 'r1', 'r2')):
    return encode(dict(legacy_sha256=legacy_sha256 or digest(LEGACY_FIXTURE), rejection_ids=list(rejection_ids)))


class OldProductPartitionTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name)
        self.repositories = 0

    def validate(self, candidate, previous=None, legacy=LEGACY_FIXTURE, old=OLD_PRODUCT_FIXTURE):
        self.repositories += 1
        history = history_for(self.path / str(self.repositories), FIXTURE, legacy=legacy, old_product=old)
        validate_report(candidate, 'vn', candidate['rel'], previous_report=previous,
                        release_row=recipe_fixture(candidate['retrieval_recipe'])[0], history=history)

    def refused(self, candidate, previous=None, legacy=LEGACY_FIXTURE, old=OLD_PRODUCT_FIXTURE, message=''):
        with self.assertRaisesRegex(ValueError, message):
            self.validate(candidate, previous, legacy, old)

    def pair(self, old_ranks=None):
        previous = build_report(3, PREVIOUS_REL, FIXTURE, CURRENT_RANKS)
        candidate = build_report(3, CANDIDATE_REL, FIXTURE, CURRENT_RANKS, old_ranks=old_ranks)
        link_period(candidate, previous)
        return previous, candidate

    def test_rejected_cases_absent_from_old_product_are_accepted(self):
        previous, candidate = self.pair()
        self.assertEqual({row['case_id'] for row in candidate['cases'] if row['cohort'] == 'old_product'}, {'o1', 'o2'})
        self.validate(candidate, previous)
        self.validate(candidate)

    def test_dropping_a_case_hybrid_rows_behind_a_dense_row_is_refused(self):
        previous, candidate = self.pair(old_ranks=dict(o2=([0], [0])))
        forge(candidate, previous, lambda row: row['cohort'] == 'old_product' and row['case_id'] == 'o2', keep=True)
        self.assertEqual([(row['arms'], row['mode'], row['limit']) for row in old_rows(candidate, 'o2')],
                         [('dense', 'exact', 8)])
        self.assertEqual({record['value'] for record in candidate['non_regression'] if record['gating']}, {1.})
        self.refused(candidate, previous, message='hybrid row')

    def test_each_missing_hybrid_row_of_an_old_product_case_is_refused(self):
        for mode, limit in (('exact', 8), ('exact', 20), ('ann', 8), ('ann', 20)):
            with self.subTest(mode=mode, limit=limit):
                previous, candidate = self.pair()
                forge(candidate, previous, lambda row: (row['cohort'], row['case_id'], row['mode'], row['limit']) ==
                      ('old_product', 'o2', mode, limit))
                self.refused(candidate, previous, message='hybrid row')

    def test_old_product_holds_exactly_the_legacy_cases_without_rejections(self):
        cases = [('rejected present', lambda row: False, lambda report: [row | dict(case_id='r0') for row in old_rows(report, 'o1')]),
                 ('case removed', lambda row: row['cohort'] == 'old_product' and row['case_id'] == 'o2', lambda report: [])]
        for name, drop, add in cases:
            with self.subTest(name=name):
                previous, candidate = self.pair()
                forge(candidate, previous, drop, add(candidate))
                if name == 'case removed':
                    cohort = candidate['cohorts']['old_product']
                    cohort.update(case_count=4, positive_count=1, expected_citations=1)
                self.refused(candidate, previous, message='old_product cohort')

    def test_report_rejections_are_the_fixture_rejections(self):
        changes = [('scored case', lambda rows: rows[2].update(case_id='o2'), 4),
                   ('unknown case', lambda rows: rows[2].update(case_id='x'), 5),
                   ('one popped', lambda rows: rows.pop(), 4),
                   ('one extra', lambda rows: rows.append(dict(case_id='x', passed=True)), 6),
                   ('one duplicated', lambda rows: rows.append(dict(rows[2])), 5)]
        for name, change, count in changes:
            with self.subTest(name=name):
                previous, candidate = self.pair()
                for report in (previous, candidate):
                    change(report['rejections'])
                    report['cohorts']['old_product']['case_count'] = count
                self.refused(candidate, message='report rejections differ')
                self.refused(candidate, previous, message='report rejections differ')

    def test_each_rejection_is_a_passed_diacritics_refusal(self):
        changes = [('failed', lambda row: row.update(passed=False)), ('passed one', lambda row: row.update(passed=1)),
                   ('passed text', lambda row: row.update(passed='true')), ('passed null', lambda row: row.update(passed=None)),
                   ('unsupported', lambda row: row.update(reason='unsupported_query')),
                   ('empty reason', lambda row: row.update(reason='')), ('reason null', lambda row: row.update(reason=None)),
                   ('extra field', lambda row: row.update(message='Vui lòng nhập câu hỏi tiếng Việt có dấu.')),
                   ('no reason', lambda row: row.pop('reason')), ('no passed', lambda row: row.pop('passed'))]
        for name, change in changes:
            with self.subTest(name=name):
                previous, candidate = self.pair()
                change(candidate['rejections'][1])
                self.assertIs(candidate['quality_pass'], True)
                self.refused(candidate, message='rejection record is not a passed diacritics_required refusal')
                self.refused(candidate, previous, message='rejection record is not a passed diacritics_required refusal')
        previous, candidate = self.pair()
        candidate['rejections'][1].pop('case_id')
        self.refused(candidate, previous, message='invalid rejection identity')

    def test_previous_old_product_partition_is_checked(self):
        changes = [('failed rejection', lambda report: report['rejections'][0].update(passed=False), 'rejection record'),
                   ('other rejections', lambda report: report['rejections'][0].update(case_id='r9'),
                    'report rejections differ'),
                   ('rejected case scored', lambda report: report['cases'].append(old_rows(report, 'o1')[0] |
                                                                               dict(case_id='r0', arms='dense')),
                    'old_product cohort differs')]
        for name, change, message in changes:
            with self.subTest(name=name):
                previous, candidate = self.pair()
                change(previous)
                self.validate(candidate)
                self.refused(candidate, previous, message=message)

    def test_old_product_rows_hold_the_legacy_citation_count(self):
        ranks = dict(o2=([1, 1], [1, 1]))
        previous = build_report(3, PREVIOUS_REL, FIXTURE, CURRENT_RANKS, old_ranks=ranks)
        candidate = build_report(3, CANDIDATE_REL, FIXTURE, CURRENT_RANKS, old_ranks=ranks)
        link_period(candidate, previous)
        self.refused(candidate, previous, message='old_product case records differ from the fixture partition')

    def test_old_product_rows_follow_each_legacy_citation_count(self):
        legacy = encode(json.loads(LEGACY_FIXTURE) + [
            dict(id='n', question='q-n', expect_abstain=True),
            dict(id='t', question='q-t', expected_citations=[dict(doc_number='a'), dict(doc_number='b')])])
        old = old_product(legacy_sha256=digest(legacy))
        report = build_report(3, CANDIDATE_REL, FIXTURE, CURRENT_RANKS, old_ranks=dict(n=([], []), t=([1, 0], [1, 2])))
        report['fixture_hashes'].update(Legacy=digest(legacy), OldProduct=digest(old))
        self.assertEqual({row['expected'] for row in report['cases'] if row['cohort'] == 'old_product'}, {0, 1, 2})
        self.validate(report, legacy=legacy, old=old)

    def test_every_case_row_holds_the_integer_fixture_count(self):
        for cohort, case_id in (('development', 'p1'), ('old_product', 'o1')):
            for name in ('true on a hybrid row', 'wrong on a dense row'):
                with self.subTest(cohort=cohort, name=name):
                    report = build_report(3, CANDIDATE_REL, FIXTURE, CURRENT_RANKS)
                    row = next(row for row in report['cases'] if (row['cohort'], row['case_id']) == (cohort, case_id))
                    if name.startswith('true'):
                        row['expected'] = True
                    else:
                        report['cases'].append(row | dict(arms='dense', expected=2, ranks=[1, 0]))
                    self.refused(report, message=cohort + ' case records differ from the fixture partition')

    def test_report_legacy_fixture_is_decoded_strictly(self):
        cases = json.loads(LEGACY_FIXTURE)
        cases[0]['Notes'] = 'unknown field'
        legacy = encode(cases)
        old = old_product(legacy_sha256=digest(legacy))
        report = build_report(3, CANDIDATE_REL, FIXTURE, CURRENT_RANKS)
        report['fixture_hashes'].update(Legacy=digest(legacy), OldProduct=digest(old))
        self.refused(report, legacy=legacy, old=old, message='invalid legacy fixture case fields')

    def test_old_product_fixture_names_the_legacy_fixture(self):
        wrong = old_product(legacy_sha256='f' * 64)
        previous, candidate = self.pair()
        for report in (previous, candidate):
            report['fixture_hashes']['OldProduct'] = digest(wrong)
        self.refused(candidate, previous, old=wrong, message='legacy_sha256')
        self.refused(candidate, old=wrong, message='legacy_sha256')

    def test_legacy_and_old_product_fixtures_come_from_main_history(self):
        previous, candidate = self.pair()
        self.refused(candidate, previous, old=None, message='old-product fixture .* not in the history')
        self.refused(candidate, previous, legacy=None, message='legacy fixture .* not in the history')
        self.refused(candidate, previous, old=old_product(rejection_ids=('r0', 'r1', 'o2')),
                     message='old-product fixture .* not in the history')
        for key in ('Legacy', 'OldProduct'):
            for value in ('legacy', None):
                with self.subTest(key=key, value=value):
                    report = copy.deepcopy(candidate)
                    report['fixture_hashes'][key] = value
                    self.refused(report, message='fixture hash')


class FixtureDecodeTest(unittest.TestCase):
    def test_legacy_fixture_decodes_like_load_legacy(self):
        self.assertEqual([case['id'] for case in legacy_cases(LEGACY_FIXTURE)], ['o1', 'o2', 'r0', 'r1', 'r2'])
        legacy_cases(b'[{"tags":["any"],"id":"a","question":"q","expect_abstain":true,"expect_fail":false,"notes":"n"},'
                     b'{"id":"b","question":"q","expect_abstain":false,"expected_citations":[{"doc_number":"d",'
                     b'"article":"1","clause":"2","point":"a","alt_doc_numbers":["e"],"relation_ok":true}]},'
                     b'{"id":"c","question":"q","expect_abstain":true,"expected_citations":[],"notes":null}]')
        cited, abstain = b'"id":"a","question":"q","expected_citations":', b'"id":"a","question":"q","expect_abstain":true'
        cases = [b'1', b'null', b'{' + abstain + b',"Question":"x"}', b'{"id":" ","question":"q","expect_abstain":true}',
                 b'{"id":"a","expect_abstain":true}', b'{"id":"a","question":" ","expect_abstain":true}',
                 b'{"id":1,"question":"q","expect_abstain":true}', b'{"id":"a","question":"q"}', b'{' + cited + b'[]}',
                 b'{' + abstain + b',"expected_citations":[{"doc_number":"d"}]}', b'{' + cited + b'[{"doc_number":" "}]}',
                 b'{' + cited + b'[{"article":"1"}]}', b'{' + cited + b'[null]}',
                 b'{' + cited + b'[{"doc_number":"d","alt_doc_numbers":[""]}]}',
                 b'{' + cited + b'[{"doc_number":"d","alt_doc_numbers":"e"}]}',
                 b'{' + cited + b'[{"doc_number":"d","Article":"1"}]}', b'{' + cited + b'[{"doc_number":"d","article":1}]}',
                 b'{' + cited + b'[{"doc_number":"d","clause":1}]}', b'{' + cited + b'[{"doc_number":"d","point":1}]}',
                 b'{' + cited + b'[{"doc_number":"d","relation_ok":"true"}]}', b'{"id":"a","question":"q","expect_abstain":1}',
                 b'{' + abstain + b',"expect_fail":0}', b'{' + abstain + b',"notes":1}', b'{' + abstain + b',"notes":NaN}',
                 b'{' + abstain + b',"tags":"x"}', b'{' + abstain + b',"tags":[1]}', b'{"id":"b",' + abstain + b'}',
                 b'{' + abstain + b'},{"id":"a","question":"r","expect_abstain":true}']
        for data in [b'[' + case + b']' for case in cases] + [b'{}', b'[]', b'null', b'[{' + abstain + b'}] []']:
            with self.subTest(data=data), self.assertRaises(ValueError):
                legacy_cases(data)

    def test_old_product_fixture_names_three_legacy_cases(self):
        cases, legacy = legacy_cases(LEGACY_FIXTURE), digest(LEGACY_FIXTURE)
        self.assertEqual(rejection_ids(OLD_PRODUCT_FIXTURE, legacy, cases), ['r0', 'r1', 'r2'])
        for data in (b'[]', b'{"legacy_sha256":"%s","rejection_ids":["r0","r1","r2"],"extra":1}' % legacy.encode(),
                     b'{"legacy_sha256":"%s"}' % legacy.encode(), b'{"rejection_ids":["r0","r1","r2"]}',
                     b'{"legacy_sha256":"%s","legacy_sha256":"%s","rejection_ids":["r0","r1","r2"]}' % (legacy.encode(), legacy.encode()),
                     old_product(legacy_sha256='f' * 64), old_product(legacy_sha256=legacy.upper()),
                     old_product(rejection_ids=('r0', 'r1')), old_product(rejection_ids=('r0', 'r1', 'r2', 'o1')),
                     old_product(rejection_ids=('r0', 'r1', 'r2', 'r2')),
                     old_product(rejection_ids=('r0', 'r1', 'r1')), old_product(rejection_ids=('r0', 'r1', 'x')),
                     old_product(rejection_ids=('r0', 'r1', 1)), old_product(rejection_ids=('r0', 'r1', ['r2'])),
                     b'{"legacy_sha256":"%s","rejection_ids":null}' % legacy.encode()):
            with self.subTest(data=data), self.assertRaises(ValueError):
                rejection_ids(data, legacy, cases)

    def test_checked_in_fixtures_decode(self):
        legacy = (REPOSITORY / LEGACY).read_bytes()
        cases = legacy_cases(legacy)
        rejected = rejection_ids((REPOSITORY / OLD_PRODUCT).read_bytes(), digest(legacy), cases)
        self.assertEqual(len(set(rejected) & {case['id'] for case in cases}), 3)


if __name__ == '__main__':
    unittest.main()
