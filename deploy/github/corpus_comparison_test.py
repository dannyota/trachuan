import copy
import unittest

from corpus_manifest import validate_report
from corpus_test_support import recipe_fixture


def reports():
    row, hashes = recipe_fixture()
    previous = dict(version=1, scorer='agent-eval/4', country='vn', rel='20990101_1',
                    retrieval_recipe=row['retrieval_recipe'], recipes=hashes,
                    fixture_hashes=dict(Legacy='a', OldProduct='b', Development='c'),
                    rejections=[dict(case_id=f'r{i}', reason='diacritics_required', passed=True) for i in range(3)],
                    cohorts={}, cases=[])
    for cohort in ('old_product', 'development'):
        metrics, comparisons = [], []
        for mode in ('exact', 'ann'):
            for name, limit, value, denominator in [('mrr', 8, .9925, 100), ('recall', 20, 1., 100)]:
                key = dict(mode=mode, arms='hybrid', limit=limit, name=name)
                metrics.append(key | dict(value=value, numerator=value * denominator, denominator=denominator))
                comparisons.append(key | dict(value=value, previous_value=value, gating=True, status='pass'))
            for limit in (8, 20):
                for i in range(100):
                    previous['cases'].append(dict(case_id=str(i), cohort=cohort, mode=mode, arms='hybrid',
                                                 limit=limit, expected=1, ranks=[4 if i == 0 else 1],
                                                 mapped_identities=[[dict(country='vn', document_key='d', kind='document', anchor=None)]]))
        previous['cohorts'][cohort] = dict(case_count=100, positive_count=100, negative_count=0,
                                           rejection_count=0, expected_citations=100,
                                           metrics=metrics, non_regression=comparisons)
    previous['cohorts']['old_product'].update(case_count=103, rejection_count=3)
    previous['cohorts']['legacy'] = dict(case_count=0)
    previous['non_regression'] = copy.deepcopy(previous['cohorts']['old_product']['non_regression'])
    candidate = copy.deepcopy(previous)
    candidate.update(version=2, scorer='agent-eval/5', rel='20990102_1', status='complete', quality_pass=True,
                     release_eligible=False, previous_diff={'previous': dict(country='vn', rel='20990101_1', report_sha256='a' * 64)})
    for case in candidate['cases']:
        if case['case_id'] == '0':
            case['ranks'] = [9]
    for name in ('old_product', 'development'):
        for metric in candidate['cohorts'][name]['metrics']:
            if metric['name'] == 'mrr':
                metric.update(value=.99, numerator=99.)
        for record in candidate['cohorts'][name]['non_regression']:
            if record['name'] == 'mrr':
                record.update(value=.99, previous=.9925, high_water=.9925, tolerance=.005,
                              within_tolerance=True, losses=[dict(case_id='0', previous_rank=4, rank=9)])
    candidate['non_regression'] = copy.deepcopy(candidate['cohorts']['old_product']['non_regression'])
    return previous, candidate


def validate_comparison(candidate, previous=None, release_row=None):
    row = recipe_fixture(candidate.get('retrieval_recipe'))[0] if release_row is None else release_row
    validate_report(candidate, 'vn', candidate['rel'], previous_report=previous, release_row=row)


class CorpusComparisonTest(unittest.TestCase):
    def validate(self, candidate, previous, release_row=None):
        validate_comparison(candidate, previous, release_row=release_row)

    def test_tolerates_a_verified_corpus_only_loss(self):
        previous, candidate = reports()
        validate_comparison(candidate)
        self.validate(candidate, previous)

    def test_recipe_non_current_tail_is_not_a_research_override(self):
        previous, candidate = reports()
        for report in (previous, candidate):
            report['retrieval_recipe']['non_current_tail'] = 2
        self.validate(candidate, previous)
        candidate['cases'][0]['non_current_tail'] = 2
        with self.assertRaises(ValueError):
            self.validate(candidate, previous)

    def test_refuses_forged_fields_and_nonfinite_values(self):
        for field, value in [('high_water', .99), ('status', 'fail'), ('previous', .99),
                             ('tolerance', .01), ('within_tolerance', False), ('losses', []),
                             ('value', float('nan'))]:
            with self.subTest(field=field):
                previous, candidate = reports()
                candidate['cohorts']['old_product']['non_regression'][0][field] = value
                with self.assertRaises(ValueError):
                    self.validate(candidate, previous)
        for field in ('high_water', 'previous', 'tolerance', 'within_tolerance', 'losses'):
            previous, candidate = reports()
            candidate['cohorts']['development']['non_regression'][0].pop(field)
            with self.subTest(missing=field), self.assertRaises(ValueError):
                self.validate(candidate, previous)

    def test_each_condition_fails_closed(self):
        mutations = [lambda p, r: r['recipes'].update(chunk='changed'),
                     lambda p, r: p.update(scorer='agent-eval/3'),
                     lambda p, r: r['fixture_hashes'].update(Development='changed'),
                     lambda p, r: r['cases'][0].update(case_id='other'),
                     lambda p, r: r['cases'][0].update(mapped_identities=[]),
                     lambda p, r: p['cases'][0].pop('mapped_identities'),
                     lambda p, r: r['rejections'][0].update(case_id='other'),
                     lambda p, r: r['cohorts']['old_product'].update(positive_count=101),
                     lambda p, r: r['cohorts']['old_product'].update(case_count=0)]
        for index, mutate in enumerate(mutations):
            previous, candidate = reports()
            mutate(previous, candidate)
            with self.subTest(index=index), self.assertRaises(ValueError):
                self.validate(candidate, previous)

    def test_version_one_cannot_claim_tolerance(self):
        previous, candidate = reports()
        candidate.update(version=1, scorer='agent-eval/4')
        candidate.pop('retrieval_recipe')
        with self.assertRaisesRegex(ValueError, 'recomputed non_regression did not pass'):
            self.validate(candidate, previous)

    def test_provenance_does_not_remove_an_alternative(self):
        previous, candidate = reports()
        candidate['cases'][0]['mapped_identities'][0][0]['document_number'] = 'changed'
        self.validate(candidate, previous)

    def test_refuses_metrics_forged_to_hide_case_regression(self):
        previous, candidate = reports()
        for row in candidate['cases']:
            if row['case_id'] == '1':
                row['ranks'] = [9]
        for name in ('old_product', 'development'):
            for record in candidate['cohorts'][name]['non_regression']:
                if record['name'] == 'mrr':
                    record['losses'].append(dict(case_id='1', previous_rank=1, rank=9))
        candidate['non_regression'] = copy.deepcopy(candidate['cohorts']['old_product']['non_regression'])
        with self.assertRaises(ValueError):
            self.validate(candidate, previous)

    def test_refuses_overflowing_json_numbers(self):
        from corpus_manifest import decode
        previous, candidate = reports()
        candidate['unused'] = decode('1e999')
        with self.assertRaises(ValueError):
            self.validate(candidate, previous)

    def test_high_water_boundary_and_missing_previous_fields(self):
        import math
        for high_water, allowed in [(.995, True), (math.nextafter(.995, 1.), False)]:
            previous, candidate = reports()
            previous.update(version=2, scorer='agent-eval/5')
            for name in ('old_product', 'development'):
                for record in previous['cohorts'][name]['non_regression']:
                    if record['name'] == 'mrr':
                        record.update(previous=high_water, high_water=high_water, tolerance=.005,
                                      within_tolerance=True, losses=[])
                for record in candidate['cohorts'][name]['non_regression']:
                    if record['name'] == 'mrr':
                        record['high_water'] = high_water
            candidate['non_regression'] = copy.deepcopy(candidate['cohorts']['old_product']['non_regression'])
            if allowed:
                self.validate(candidate, previous)
                previous['cohorts']['development']['non_regression'][0].pop('high_water')
                with self.assertRaises(ValueError):
                    self.validate(candidate, previous)
            else:
                with self.assertRaises(ValueError):
                    self.validate(candidate, previous)

    def test_three_releases_preserve_watermark_and_recipe_change_restarts_it(self):
        previous, middle = reports()
        self.validate(middle, previous)
        last = copy.deepcopy(middle)
        for name in ('old_product', 'development'):
            for record in last['cohorts'][name]['non_regression']:
                if record['name'] == 'mrr':
                    record.update(previous=.99, previous_value=.99, within_tolerance=False, losses=[])
        last['non_regression'] = copy.deepcopy(last['cohorts']['old_product']['non_regression'])
        middle['rel'] = previous['rel']
        self.validate(last, middle)
        for name in ('old_product', 'development'):
            for record in last['cohorts'][name]['non_regression']:
                if record['name'] == 'mrr':
                    record.update(high_water=.99, tolerance=0)
        row, hashes = recipe_fixture(last['retrieval_recipe'], chunk={'version': 2})
        last['recipes']['chunk'] = hashes['chunk']
        last['non_regression'] = copy.deepcopy(last['cohorts']['old_product']['non_regression'])
        self.validate(last, middle, release_row=row)

    def test_unknown_versions_and_scorers_are_refused(self):
        for version, scorer in [(3, 'agent-eval/5'), (2, 'agent-eval/6'), (1, 'agent-eval/5')]:
            previous, candidate = reports()
            candidate.update(version=version, scorer=scorer)
            with self.subTest(version=version, scorer=scorer), self.assertRaises(ValueError):
                self.validate(candidate, previous)

    def test_loss_ranks_keep_removed_expectations_and_limit_eight_absence(self):
        from corpus_comparison import cases, losses
        for rank20, expected, wanted in [(9, 1, 9), (4, 1, 0), (0, 0, 0)]:
            previous, candidate = reports()
            candidate['cases'][0].update(expected=expected, ranks=[0] if expected else [], mapped_identities=[])
            candidate['cases'][100]['ranks'] = [rank20]
            lost, preserved = losses(cases(candidate, 'old_product'), cases(previous, 'old_product'), 'exact')
            with self.subTest(rank20=rank20, expected=expected):
                self.assertEqual(lost, [dict(case_id='0', previous_rank=4, rank=wanted)])
                self.assertFalse(preserved)

    def test_one_ulp_forgery_cannot_cross_the_tolerance_boundary(self):
        import math
        previous, candidate = reports()
        previous.update(version=2, scorer='agent-eval/5')
        high_water = math.nextafter(.995, 1.)
        for name in ('old_product', 'development'):
            for record in previous['cohorts'][name]['non_regression']:
                if record['name'] == 'mrr':
                    record.update(previous=high_water, high_water=high_water, tolerance=.005,
                                  within_tolerance=True, losses=[])
            for record in candidate['cohorts'][name]['non_regression']:
                if record['name'] == 'mrr':
                    record.update(value=high_water - .005, high_water=high_water)
            for metric in candidate['cohorts'][name]['metrics']:
                if metric['name'] == 'mrr':
                    metric.update(value=high_water - .005, numerator=math.nextafter(99., 100.))
        candidate['non_regression'] = copy.deepcopy(candidate['cohorts']['old_product']['non_regression'])
        with self.assertRaises(ValueError):
            self.validate(candidate, previous)

    def test_mrr_sums_in_go_case_order(self):
        from corpus_comparison import verify_metric_cases
        rows = {(str(i), 'exact', 'hybrid', 8): dict(expected=1, ranks=[3]) for i in range(100)}
        verify_metric_cases(dict(value=.3333333333333329, numerator=33.33333333333329, denominator=100),
                            ('exact', 'hybrid', 8, 'mrr'), rows)

    def test_calibration_hash_alone_allows_tolerance_but_threshold_does_not(self):
        for threshold, calibration, allowed in [(0.5297041131124263, 'new', True), (.54, 'old', False), (.54, 'new', False)]:
            previous, candidate = reports()
            previous['retrieval_recipe']['abstention'].update(calibration_sha256='old')
            candidate['retrieval_recipe']['abstention'].update(threshold=threshold, calibration_sha256=calibration)
            previous['recipes']['retrieval'] = recipe_fixture(previous['retrieval_recipe'])[1]['retrieval']
            candidate['recipes']['retrieval'] = recipe_fixture(candidate['retrieval_recipe'])[1]['retrieval']
            with self.subTest(threshold=threshold, calibration=calibration):
                if allowed:
                    self.validate(candidate, previous)
                else:
                    with self.assertRaises(ValueError):
                        self.validate(candidate, previous)

    def test_calibration_change_preserves_the_watermark(self):
        previous, candidate = reports()
        previous.update(version=2, scorer='agent-eval/5')
        previous['retrieval_recipe']['abstention'].update(calibration_sha256='old')
        candidate['retrieval_recipe']['abstention'].update(calibration_sha256='new')
        previous['recipes']['retrieval'] = recipe_fixture(previous['retrieval_recipe'])[1]['retrieval']
        candidate['recipes']['retrieval'] = recipe_fixture(candidate['retrieval_recipe'])[1]['retrieval']
        for name in ('old_product', 'development'):
            for record in previous['cohorts'][name]['non_regression']:
                if record['name'] == 'mrr':
                    record.update(previous=.995, high_water=.995, tolerance=.005, within_tolerance=True,
                                  losses=[dict(case_id='0', previous_rank=2, rank=4)])
            for record in candidate['cohorts'][name]['non_regression']:
                if record['name'] == 'mrr':
                    record['high_water'] = .9925
        candidate['non_regression'] = copy.deepcopy(candidate['cohorts']['old_product']['non_regression'])
        with self.assertRaises(ValueError):
            self.validate(candidate, previous)


if __name__ == '__main__':
    unittest.main()
