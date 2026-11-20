import copy
from pathlib import Path
import tempfile
import unittest

from corpus_manifest import validate_report
from corpus_revision import compare_fixtures
from corpus_revision_support import (CANDIDATE_MRR, PRIOR_MRR, REVISED, base_revision_records, encode, fixture_case,
                                     history_for, revision_pair, set_records)
from corpus_test_support import recipe_fixture

FIVE = [fixture_case(case_id) for case_id in 'abcde']
MOVED = FIVE[:4] + [fixture_case('e', 1, 'historical')]
SEVEN = dict(a=([7], [7]), b=([1], [1]), c=([1], [1]), d=([1], [1]), e=([1], [1]))
EIGHT = SEVEN | dict(a=([8], [8]))
FIVE_PRIOR = (1 / 7 + 1 + 1 + 1) / 4
FIVE_CANDIDATE = (1 / 8 + 1 + 1 + 1) / 4


class RevisionGateTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name)
        self.repositories = 0

    def validate(self, candidate, previous, *fixtures):
        self.repositories += 1
        validate_report(candidate, 'vn', candidate['rel'], previous_report=previous,
                        release_row=recipe_fixture(candidate['retrieval_recipe'])[0],
                        history=history_for(self.path / str(self.repositories), *fixtures))

    def refused(self, candidate, previous, *fixtures, message=''):
        with self.assertRaisesRegex(ValueError, message):
            self.validate(candidate, previous, *fixtures)

    def five_case_pair(self, previous_fields):
        previous, candidate, before, after = revision_pair(FIVE, MOVED, SEVEN, EIGHT, previous_version=3)
        set_records(previous, 'development', 'mrr', **previous_fields)
        revision = dict(cases=4, revised=[dict(case_id='e', reason='cohort')])
        set_records(candidate, 'development', 'recall', previous_value=1., numerator=4, denominator=4,
                    previous_numerator=4, previous_denominator=4, revision=revision | dict(value=1.))
        set_records(candidate, 'development', 'mrr', previous_value=FIVE_PRIOR, previous=FIVE_PRIOR,
                    within_tolerance=True, losses=[dict(case_id='a', previous_rank=7, rank=8)],
                    revision=revision | dict(value=FIVE_CANDIDATE))
        return previous, candidate, before, after

    def test_revised_mrr_fall_beyond_tolerance_fails(self):
        current = dict(p1=([3], [3]), p2=([0, 4], [12, 4]), p3=([6], [6]), m=([0], [0]), n=([], []))
        previous, candidate, before, after = revision_pair(current_ranks=current)
        base_revision_records(candidate)
        value = (1 / 3 + 1 / 4 + 1 / 6) / 3
        set_records(candidate, 'development', 'mrr', within_tolerance=True,
                    revision=dict(cases=4, value=value, revised=REVISED),
                    losses=[dict(case_id='p1', previous_rank=2, rank=3), dict(case_id='p2', previous_rank=3, rank=4),
                            dict(case_id='p3', previous_rank=5, rank=6)])
        self.refused(candidate, previous, before, after, message='did not pass')

    def test_revised_mrr_threshold_starts_from_the_carried_watermark(self):
        previous, candidate, before, after = self.five_case_pair(dict(value=.7, high_water=.703))
        set_records(candidate, 'development', 'mrr', high_water=FIVE_PRIOR + (.703 - .7), tolerance=.005)
        self.assertGreater(FIVE_CANDIDATE, FIVE_PRIOR - .005)
        self.refused(candidate, previous, before, after, message='did not pass')

    def test_strict_revised_mrr_threshold_is_the_previous_value(self):
        previous, candidate, before, after = revision_pair(FIVE, MOVED, SEVEN | dict(b=([2], [2])), EIGHT, previous_version=3)
        set_records(previous, 'development', 'mrr', value=.7, high_water=.9)
        for row in candidate['cases']:
            if row['case_id'] == 'a':
                row['mapped_identities'] = [[dict(country='vn', document_key='other', kind='document')]]
        prior = (1 / 7 + 1 / 2 + 1 + 1) / 4
        revision = dict(cases=4, revised=[dict(case_id='e', reason='cohort')])
        set_records(candidate, 'development', 'recall', previous_value=1., numerator=4, denominator=4,
                    previous_numerator=4, previous_denominator=4, revision=revision | dict(value=1.))
        set_records(candidate, 'development', 'mrr', previous_value=prior, previous=prior, high_water=prior + (.9 - .7),
                    tolerance=0, within_tolerance=False, losses=[dict(case_id='a', previous_rank=7, rank=8)],
                    revision=revision | dict(value=FIVE_CANDIDATE))
        self.assertTrue(prior <= FIVE_CANDIDATE < prior + (.9 - .7))
        self.validate(candidate, previous, before, after)

    def test_a_loss_with_a_changed_expectation_drops_the_tolerance(self):
        previous, candidate, before, after = self.five_case_pair(dict(value=.7, high_water=.7))
        set_records(candidate, 'development', 'mrr', high_water=FIVE_PRIOR, tolerance=.005)
        self.validate(candidate, previous, before, after)
        for row in candidate['cases']:
            if row['case_id'] == 'a':
                row['mapped_identities'] = [[dict(country='vn', document_key='other', kind='document')]]
        self.refused(candidate, previous, before, after)
        set_records(candidate, 'development', 'mrr', tolerance=0)
        self.refused(candidate, previous, before, after, message='did not pass')

    def test_comparable_rows_that_differ_drop_the_tolerance(self):
        previous, candidate, before, after = revision_pair()
        base_revision_records(candidate)
        row = next(row for row in candidate['cases'] if row['case_id'] == 'p1')
        candidate['cases'].append(row | dict(arms='dense'))
        self.refused(candidate, previous, before, after, message='revised comparison differs')
        set_records(candidate, 'development', 'mrr', tolerance=0)
        self.validate(candidate, previous, before, after)

    def test_previous_rejections_that_differ_from_the_fixture_are_refused(self):
        previous, candidate, before, after = revision_pair()
        base_revision_records(candidate)
        previous['rejections'][0]['case_id'] = 'r9'
        set_records(candidate, 'old_product', 'mrr', tolerance=0)
        set_records(candidate, 'development', 'mrr', tolerance=0)
        self.refused(candidate, previous, before, after, message='report rejections differ')

    def test_old_product_records_are_checked_in_a_revised_report(self):
        for field, value in (('high_water', 1.001), ('tolerance', 0), ('status', 'fail'), ('previous_value', .5)):
            with self.subTest(field=field):
                previous, candidate, before, after = revision_pair()
                base_revision_records(candidate)
                set_records(candidate, 'old_product', 'mrr', **{field: value})
                self.refused(candidate, previous, before, after)

    def test_revised_value_and_metric_come_from_the_case_records(self):
        mutations = [lambda report: set_records(report, 'development', 'mrr', value=.9),
                     lambda report: metric_and_record(report, 'mrr', CANDIDATE_MRR + 1e-9),
                     lambda report: metric_and_record(report, 'recall', .9)]
        for index, mutate in enumerate(mutations):
            with self.subTest(index=index):
                previous, candidate, before, after = revision_pair()
                base_revision_records(candidate)
                mutate(candidate)
                self.refused(candidate, previous, before, after)

    def test_a_comparable_case_with_mapping_issues_has_no_comparable_value(self):
        for side in ('previous', 'candidate'):
            with self.subTest(side=side):
                previous, candidate, before, after = revision_pair()
                base_revision_records(candidate)
                for row in (previous if side == 'previous' else candidate)['cases']:
                    if row['case_id'] == 'p1':
                        row['mapping_issues'] = [dict(kind='mapping_incomplete', citation_index=0, detail='alternatives')]
                self.refused(candidate, previous, before, after, message='no comparable value')

    def test_recall_counts_only_ranks_within_its_limit(self):
        current = dict(p1=([1], [1]), p2=([0, 4], [12, 25]), p3=([6], [6]), m=([0], [0]), n=([], []))
        previous, candidate, before, after = revision_pair(current_ranks=current)
        base_revision_records(candidate)
        set_records(candidate, 'development', 'recall', numerator=3, revision=dict(cases=4, value=.75, revised=REVISED))
        self.validate(candidate, previous, before, after)

    def test_each_compared_field_changes_the_case(self):
        citation = dict(doc_number='d', article='1', clause='2', point='a', alt_doc_numbers=['x'], relation_ok=True)
        base = dict(tags=['identifier'], id='a', question='q', expected_citations=[citation])
        changes = [dict(doc_number='e'), dict(article='9'), dict(clause='9'), dict(point='b'), dict(alt_doc_numbers=['y']),
                   dict(alt_doc_numbers=[]), dict(relation_ok=False)]
        for change in changes:
            with self.subTest(change=change):
                current = copy.deepcopy(base)
                current['expected_citations'][0].update(change)
                self.assertEqual(compare_fixtures([base], [current], dict(a='development')).revised,
                                 [dict(case_id='a', reason='changed')])
        for change in (dict(expect_abstain=True), dict(question='other')):
            with self.subTest(change=change):
                self.assertEqual(compare_fixtures([base], [base | change], dict(a='development')).revised,
                                 [dict(case_id='a', reason='changed')])
        for change in (dict(tags=['clause']), dict(notes='n'), dict(expect_fail=True), dict(expect_abstain=False)):
            with self.subTest(change=change):
                self.assertEqual(compare_fixtures([base], [base | change], dict(a='development')).comparable, ['a'])


def metric_and_record(report, name, value):
    for metric in report['cohorts']['development']['metrics']:
        if (metric['mode'], metric['name']) == ('exact', name) and metric['limit'] == (8 if name == 'mrr' else 20):
            metric['value'] = value
    for record in report['cohorts']['development']['non_regression']:
        if (record['mode'], record['name']) == ('exact', name):
            record['value'] = value


if __name__ == '__main__':
    unittest.main()
