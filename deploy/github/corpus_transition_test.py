import copy
import unittest

from corpus_comparison_test import reports
from corpus_manifest import validate_report


class CorpusTransitionTest(unittest.TestCase):
    def test_case_counts_cannot_disable_development_gates(self):
        for count in (0, 99, 101):
            previous, candidate = reports()
            candidate['cohorts']['development']['case_count'] = count
            with self.subTest(count=count), self.assertRaises(ValueError):
                validate_report(candidate, 'vn', candidate['rel'], previous_report=previous)

    def test_populated_previous_cohort_cannot_be_emptied_or_removed(self):
        for absent in (False, True):
            previous, candidate = reports()
            candidate['cases'] = [case for case in candidate['cases'] if case['cohort'] != 'development']
            candidate['cohorts']['development'].update(case_count=0, positive_count=0, expected_citations=0,
                                                       metrics=[], non_regression=[])
            if absent:
                candidate['cohorts'].pop('development')
            with self.subTest(absent=absent), self.assertRaises(ValueError):
                validate_report(candidate, 'vn', candidate['rel'], previous_report=previous)

    def test_version_one_cannot_follow_version_two(self):
        previous, middle = reports()
        validate_report(middle, 'vn', middle['rel'], previous_report=previous)
        candidate = copy.deepcopy(middle)
        candidate.update(version=1, scorer='agent-eval/4')
        middle['rel'] = previous['rel']
        for name in ('old_product', 'development'):
            for record in candidate['cohorts'][name]['non_regression']:
                if record['name'] == 'mrr':
                    record['previous_value'] = .99
                    for field in ('previous', 'high_water', 'tolerance', 'within_tolerance', 'losses'):
                        record.pop(field)
        candidate['non_regression'] = copy.deepcopy(candidate['cohorts']['old_product']['non_regression'])
        with self.assertRaises(ValueError):
            validate_report(candidate, 'vn', candidate['rel'], previous_report=middle)

    def test_version_two_chain_keeps_the_original_watermark(self):
        _, base = reports()

        def release(rel, matched, previous_value, losses):
            report = copy.deepcopy(base)
            report.update(rel=rel, cases=[])
            for name in ('old_product', 'development'):
                cohort = report['cohorts'][name]
                cohort.update(case_count=503 if name == 'old_product' else 500,
                              positive_count=500, expected_citations=500)
                for mode in ('exact', 'ann'):
                    for limit in (8, 20):
                        report['cases'] += [dict(case_id=str(i), cohort=name, mode=mode, arms='hybrid',
                                                  limit=limit, expected=1, ranks=[1 if i < matched else 9],
                                                  mapped_identities=[[dict(country='vn', document_key='d', kind='document', anchor=None)]])
                                             for i in range(500)]
                for metric in cohort['metrics']:
                    is_mrr = metric['name'] == 'mrr'
                    metric.update(numerator=matched if is_mrr else 500, denominator=500,
                                  value=matched / 500 if is_mrr else 1.)
                for record in cohort['non_regression']:
                    if record['name'] == 'mrr':
                        record.update(value=matched / 500, previous_value=previous_value, previous=previous_value,
                                      high_water=.8, tolerance=.005, within_tolerance=matched / 500 < previous_value,
                                      losses=[dict(case_id=str(i), previous_rank=1, rank=9) for i in losses])
            report['non_regression'] = copy.deepcopy(report['cohorts']['old_product']['non_regression'])
            return report

        first = release('20990101_1', 400, .8, [])
        middle = release('20990102_1', 398, .8, [398, 399])
        validate_report(middle, 'vn', middle['rel'], previous_report=first)
        last = release('20990103_1', 396, .796, [396, 397])
        last['previous_diff']['previous']['rel'] = middle['rel']
        with self.assertRaises(ValueError):
            validate_report(last, 'vn', last['rel'], previous_report=middle)

    def test_empty_development_in_both_reports_is_allowed(self):
        previous, candidate = reports()
        for report in (previous, candidate):
            report['cases'] = [row for row in report['cases'] if row['cohort'] != 'development']
            report['cohorts']['development'].update(case_count=0, positive_count=0,
                                                    expected_citations=0, metrics=[], non_regression=[])
        validate_report(candidate, 'vn', candidate['rel'], previous_report=previous)
