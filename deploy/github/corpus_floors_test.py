import copy
from pathlib import Path
import tempfile
import unittest

from corpus_floors import check_floors
from corpus_manifest import validate_report
from corpus_revision_support import (CANDIDATE_REL, CURRENT_CASES, CURRENT_RANKS, PREVIOUS_REL, build_report, encode,
                                     history_for, link_period)
from corpus_test_support import recipe_fixture

FIXTURE = encode(CURRENT_CASES)


def floors(report, cohort):
    return report['cohorts'][cohort]['floors']


def precision(report, cohort, mode):
    for metric in report['cohorts'][cohort]['metrics']:
        if (metric['name'], metric['arms'], metric['limit'], metric['mode']) == ('current_law_precision', 'hybrid', 8, mode):
            return metric
    raise AssertionError('no current_law_precision metric')


def set_precision(report, cohort, mode, numerator, denominator):
    metric = precision(report, cohort, mode)
    metric.update(numerator=numerator, denominator=denominator, value=numerator / denominator)
    for floor in floors(report, cohort):
        if floor['mode'] == mode:
            floor.update(numerator=numerator, denominator=denominator, value=numerator / denominator)
    if cohort == 'old_product':
        report['floors'] = copy.deepcopy(floors(report, 'old_product'))


def empty_floor(mode):
    return dict(non_current_tail=None, not_yet_eligible=False, cap_override=None, mode=mode, arms='hybrid', limit=8,
                name='current_law_precision', numerator=0, denominator=0, value=None, floor=.99, status='not_run')


def edit_floor(report, cohort, mode, **fields):
    for floor in floors(report, cohort):
        if floor['mode'] == mode:
            floor.update(fields)
    if cohort == 'old_product':
        report['floors'] = copy.deepcopy(floors(report, 'old_product'))


def empty_development(report):
    development = report['cohorts']['development']
    development['case_count'] = 0
    development['metrics'] = [metric for metric in development['metrics'] if metric['name'] != 'current_law_precision']
    development['floors'] = [empty_floor(mode) for mode in ('exact', 'ann')]


class PrecisionFloorTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name)
        self.repositories = 0

    def validate(self, candidate, previous=None):
        self.repositories += 1
        history = history_for(self.path / str(self.repositories), FIXTURE)
        validate_report(candidate, 'vn', candidate['rel'], previous_report=previous,
                        release_row=recipe_fixture(candidate['retrieval_recipe'])[0], history=history)

    def refused(self, candidate, previous=None, message='floor'):
        with self.assertRaisesRegex(ValueError, message):
            self.validate(candidate, previous)

    def pair(self):
        previous = build_report(3, PREVIOUS_REL, FIXTURE, CURRENT_RANKS)
        candidate = build_report(3, CANDIDATE_REL, FIXTURE, CURRENT_RANKS)
        link_period(candidate, previous)
        return previous, candidate

    def test_passing_floors_are_accepted(self):
        previous, candidate = self.pair()
        self.assertEqual(candidate['floors'], floors(candidate, 'old_product'))
        self.assertEqual([floor['status'] for floor in floors(candidate, 'development')], ['pass', 'pass'])
        self.validate(candidate, previous)
        self.validate(candidate)

    def test_a_failed_floor_with_quality_pass_true_is_refused(self):
        for cohort in ('old_product', 'development'):
            for mode in ('exact', 'ann'):
                with self.subTest(cohort=cohort, mode=mode):
                    previous, candidate = self.pair()
                    set_precision(candidate, cohort, mode, 1, 2)
                    edit_floor(candidate, cohort, mode, status='fail')
                    self.refused(candidate, previous)
                    self.refused(candidate)

    def test_a_value_below_the_floor_is_refused_whatever_the_stated_status_and_floor(self):
        for fields in (dict(), dict(floor=.5), dict(floor=.5, status='pass')):
            with self.subTest(fields=fields):
                previous, candidate = self.pair()
                set_precision(candidate, 'old_product', 'exact', 6, 10)
                edit_floor(candidate, 'old_product', 'exact', **fields)
                self.refused(candidate, previous)

    def test_the_floor_constant_is_the_validators_own(self):
        for value in (.5, 1, .98, None, True, '0.99'):
            with self.subTest(value=value):
                previous, candidate = self.pair()
                edit_floor(candidate, 'development', 'ann', floor=value)
                self.refused(candidate, previous)

    def test_each_status_other_than_pass_is_refused(self):
        for status in ('fail', 'unavailable', 'not_run', None, 'PASS'):
            with self.subTest(status=status):
                previous, candidate = self.pair()
                edit_floor(candidate, 'old_product', 'ann', status=status)
                self.refused(candidate, previous)

    def test_a_floor_must_equal_its_cohort_metric(self):
        for fields in (dict(value=.995), dict(numerator=9), dict(denominator=11), dict(mapping_incomplete_count=1)):
            with self.subTest(fields=fields):
                previous, candidate = self.pair()
                edit_floor(candidate, 'development', 'exact', **fields)
                self.refused(candidate, previous)

    def test_a_metric_value_must_be_its_numerator_over_its_denominator(self):
        for numerator, denominator, value in ((9, 10, 1.0), (10, 0, 1.0), (10, 10, float('nan')), (10, 10, True)):
            with self.subTest(numerator=numerator, denominator=denominator, value=value):
                previous, candidate = self.pair()
                for target in (precision(candidate, 'old_product', 'exact'), *floors(candidate, 'old_product')[:1]):
                    target.update(numerator=numerator, denominator=denominator, value=value)
                candidate['floors'] = copy.deepcopy(floors(candidate, 'old_product'))
                self.refused(candidate, previous, message='' if value != value else 'floor')

    def test_floor_records_are_exactly_the_two_go_writes(self):
        cases = {
            'missing': lambda report: floors(report, 'development').pop(),
            'extra': lambda report: floors(report, 'development').append(copy.deepcopy(floors(report, 'development')[0])),
            'swapped modes': lambda report: floors(report, 'development').reverse(),
            'wrong limit': lambda report: floors(report, 'development')[0].update(limit=20),
            'wrong arms': lambda report: floors(report, 'development')[0].update(arms='dense'),
            'wrong name': lambda report: floors(report, 'development')[0].update(name='recall'),
            'no floors': lambda report: report['cohorts']['development'].pop('floors'),
            'not a list': lambda report: report['cohorts']['development'].update(floors={}),
        }
        for name, change in cases.items():
            with self.subTest(case=name):
                previous, candidate = self.pair()
                change(candidate)
                self.refused(candidate, previous)

    def test_top_level_floors_are_the_old_product_floors(self):
        previous, candidate = self.pair()
        candidate['floors'][0]['value'] = .995
        self.refused(candidate, previous)
        previous, candidate = self.pair()
        candidate['floors'].pop()
        self.refused(candidate, previous)

    def test_a_populated_development_cohort_cannot_have_not_run_floors(self):
        previous, candidate = self.pair()
        for floor in floors(candidate, 'development'):
            floor['status'] = 'not_run'
        self.refused(candidate, previous)

    def test_an_empty_development_cohort_has_go_not_run_floors(self):
        _, report = self.pair()
        empty_development(report)
        check_floors(report)
        for value in (0, 0.0):
            for floor in floors(report, 'development'):
                floor['numerator'] = value
            check_floors(report)
        floors(report, 'development')[1]['status'] = 'pass'
        with self.assertRaisesRegex(ValueError, 'floor'):
            check_floors(report)

    def test_an_empty_development_floor_record_must_be_the_go_zero_record(self):
        changes = {
            'numerator one': dict(numerator=1), 'numerator fraction': dict(numerator=.5),
            'numerator bool': dict(numerator=False), 'numerator text': dict(numerator='0'),
            'numerator null': dict(numerator=None),
            'denominator one': dict(denominator=1), 'denominator float': dict(denominator=0.0),
            'denominator bool': dict(denominator=False), 'denominator null': dict(denominator=None),
            'value zero': dict(value=0), 'value float': dict(value=0.0), 'value one': dict(value=1.0),
            'value nan': dict(value=float('nan')),
        }
        for mode in ('exact', 'ann'):
            for name, fields in changes.items():
                with self.subTest(mode=mode, case=name):
                    _, report = self.pair()
                    empty_development(report)
                    for floor in floors(report, 'development'):
                        if floor['mode'] == mode:
                            floor.update(fields)
                    with self.assertRaisesRegex(ValueError, 'floor'):
                        check_floors(report)
            with self.subTest(mode=mode, case='value missing'):
                _, report = self.pair()
                empty_development(report)
                for floor in floors(report, 'development'):
                    if floor['mode'] == mode:
                        del floor['value']
                with self.assertRaisesRegex(ValueError, 'floor'):
                    check_floors(report)

    def test_an_empty_development_cohort_carries_no_matching_precision_metric(self):
        for mode in ('exact', 'ann'):
            for fields in (dict(numerator=10., denominator=10, value=1.), dict(numerator=0, denominator=0, value=0.),
                           dict(numerator=0, denominator=5, value=None), dict(numerator=0, denominator=0, value=None)):
                with self.subTest(mode=mode, fields=fields):
                    _, report = self.pair()
                    empty_development(report)
                    metric = dict(floors(report, 'development')[0 if mode == 'exact' else 1], **fields)
                    for key in ('floor', 'status'):
                        del metric[key]
                    report['cohorts']['development']['metrics'].append(metric)
                    with self.assertRaisesRegex(ValueError, 'floor'):
                        check_floors(report)

    def test_numeric_encodings_must_keep_the_go_types(self):
        drifts = {'limit float': dict(limit=8.0), 'denominator float': dict(denominator=10.0),
                  'limit bool': dict(limit=True), 'denominator bool': dict(denominator=True),
                  'name not text': dict(name=['current_law_precision']), 'value text': dict(value='1.0'),
                  'numerator text': dict(numerator='10')}
        for cohort in ('old_product', 'development'):
            for name, fields in drifts.items():
                with self.subTest(cohort=cohort, case=name, where='floor and metric'):
                    previous, candidate = self.pair()
                    precision(candidate, cohort, 'exact').update(fields)
                    edit_floor(candidate, cohort, 'exact', **fields)
                    self.refused(candidate, previous, message='')
                    self.refused(candidate, message='')
                with self.subTest(cohort=cohort, case=name, where='floor only'):
                    _, candidate = self.pair()
                    edit_floor(candidate, cohort, 'exact', **fields)
                    self.refused(candidate, message='')
                with self.subTest(cohort=cohort, case=name, where='metric only'):
                    _, candidate = self.pair()
                    precision(candidate, cohort, 'exact').update(fields)
                    self.refused(candidate, message='')

    def test_mapping_incomplete_count_must_be_an_exact_int_when_present(self):
        for value in (1.0, True, '1', None):
            with self.subTest(value=value, where='cohort floor and metric'):
                previous, candidate = self.pair()
                precision(candidate, 'development', 'exact')['mapping_incomplete_count'] = value
                edit_floor(candidate, 'development', 'exact', mapping_incomplete_count=value)
                self.refused(candidate, previous, message='')
            with self.subTest(value=value, where='top level only'):
                previous, candidate = self.pair()
                candidate['floors'][0]['mapping_incomplete_count'] = value
                self.refused(candidate, previous, message='')
            with self.subTest(value=value, where='metric only'):
                _, candidate = self.pair()
                precision(candidate, 'old_product', 'ann')['mapping_incomplete_count'] = value
                self.refused(candidate, message='')
        previous, candidate = self.pair()
        precision(candidate, 'development', 'exact')['mapping_incomplete_count'] = 2
        edit_floor(candidate, 'development', 'exact', mapping_incomplete_count=2)
        self.validate(candidate, previous)

    def consistent(self, candidate, cohort, **fields):
        precision(candidate, cohort, 'exact').update(fields)
        edit_floor(candidate, cohort, 'exact', **fields)

    def test_fields_go_never_writes_on_a_precision_record_are_refused(self):
        extras = {'reason': 'mapping_incomplete', 'reason empty': dict(reason=''), 'reason null': dict(reason=None),
                  'confusion': dict(confusion=None), 'confusion object': dict(confusion=dict(tp=1)),
                  'unknown': dict(note='x'), 'previous_value': dict(previous_value=1.), 'gating': dict(gating=True)}
        extras['reason'] = dict(reason='mapping_incomplete')
        for cohort in ('old_product', 'development'):
            for name, fields in extras.items():
                with self.subTest(cohort=cohort, case=name, where='floor and metric'):
                    previous, candidate = self.pair()
                    self.consistent(candidate, cohort, **fields)
                    self.refused(candidate, previous, message='')
                    self.refused(candidate, message='')
                with self.subTest(cohort=cohort, case=name, where='floor only'):
                    _, candidate = self.pair()
                    edit_floor(candidate, cohort, 'ann', **fields)
                    self.refused(candidate, message='')
                with self.subTest(cohort=cohort, case=name, where='metric only'):
                    _, candidate = self.pair()
                    precision(candidate, cohort, 'ann').update(fields)
                    self.refused(candidate, message='')
        for name, fields in extras.items():
            with self.subTest(case=name, where='top level only'):
                _, candidate = self.pair()
                candidate['floors'][1].update(fields)
                self.refused(candidate, message='')

    def test_a_metric_record_cannot_carry_floor_fields_and_a_floor_needs_them(self):
        _, candidate = self.pair()
        precision(candidate, 'development', 'exact').update(floor=.99)
        self.refused(candidate, message='')
        _, candidate = self.pair()
        precision(candidate, 'development', 'exact').update(status='pass')
        self.refused(candidate, message='')
        _, candidate = self.pair()
        for floor in floors(candidate, 'development'):
            floor.pop('floor')
        self.refused(candidate, message='')

    def test_legitimate_optional_fields_and_the_null_historical_marker_are_accepted(self):
        optional = dict(non_current_tail=None, not_yet_eligible=False, cap_override=None, historical_policy=None,
                        mapping_incomplete_count=2)
        for version in (2, 3):
            with self.subTest(version=version):
                if version == 3:
                    previous, candidate = self.pair()
                else:
                    previous, candidate = VersionTwoFloorTest.pair(self)
                for cohort in ('old_product', 'development'):
                    self.consistent(candidate, cohort, **optional)
                candidate['floors'] = copy.deepcopy(floors(candidate, 'old_product'))
                self.validate(candidate) if version == 3 else VersionTwoFloorTest.validate(self, candidate, previous)

    def test_override_field_types_are_checked(self):
        drifts = {'tail float': dict(non_current_tail=1.0), 'tail bool': dict(non_current_tail=True),
                  'tail text': dict(non_current_tail='1'), 'not yet text': dict(not_yet_eligible='no'),
                  'not yet int': dict(not_yet_eligible=0), 'cap text': dict(cap_override='x'),
                  'article cap text': dict(article_cap_override='x'), 'policy text': dict(historical_policy='agent')}
        for name, fields in drifts.items():
            with self.subTest(case=name, where='floor and metric'):
                _, candidate = self.pair()
                self.consistent(candidate, 'old_product', **fields)
                candidate['floors'] = copy.deepcopy(floors(candidate, 'old_product'))
                self.refused(candidate, message='')
            with self.subTest(case=name, where='top level only'):
                _, candidate = self.pair()
                candidate['floors'][0].update(fields)
                self.refused(candidate, message='')
            with self.subTest(case=name, where='metric only'):
                _, candidate = self.pair()
                precision(candidate, 'development', 'exact').update(fields)
                self.refused(candidate, message='')

    def test_top_level_floor_type_drift_alone_is_refused(self):
        for fields in (dict(limit=8.0), dict(denominator=10.0), dict(numerator='10'), dict(arms=None)):
            with self.subTest(fields=fields):
                previous, candidate = self.pair()
                candidate['floors'][0].update(fields)
                self.refused(candidate, previous, message='')
                self.refused(candidate, message='')

    def test_integral_float_numerators_are_accepted(self):
        previous, candidate = self.pair()
        for target in (precision(candidate, 'old_product', 'exact'), floors(candidate, 'old_product')[0]):
            target['numerator'] = 10.0
        candidate['floors'] = copy.deepcopy(floors(candidate, 'old_product'))
        self.validate(candidate, previous)
        self.validate(candidate)

    def test_the_previous_reports_floors_are_checked(self):
        previous, candidate = self.pair()
        set_precision(previous, 'old_product', 'exact', 1, 2)
        edit_floor(previous, 'old_product', 'exact', status='fail')
        self.refused(candidate, previous)

    def test_override_fields_must_be_null_or_false_in_the_predecessor_and_the_candidate(self):
        drifts = {'cap_override': dict(cap_override={}), 'article_cap_override': dict(article_cap_override={}),
                  'historical_policy': dict(historical_policy='agent'), 'non_current_tail': dict(non_current_tail=1),
                  'not_yet_eligible': dict(not_yet_eligible=True)}
        for name, fields in drifts.items():
            for side in ('previous', 'candidate'):
                for where in ('top level', 'cohort floor', 'metric'):
                    with self.subTest(field=name, side=side, where=where):
                        previous, candidate = self.pair()
                        report = previous if side == 'previous' else candidate
                        if where == 'top level':
                            report['floors'][0].update(fields)
                        elif where == 'cohort floor':
                            floors(report, 'old_product')[0].update(fields)
                        else:
                            precision(report, 'old_product', 'exact').update(fields)
                        with self.assertRaises(ValueError):
                            check_floors(report)
                        self.refused(candidate, previous, message='')
                with self.subTest(field=name, side=side, where='all three copies'):
                    previous, candidate = self.pair()
                    report = previous if side == 'previous' else candidate
                    self.consistent(report, 'old_product', **fields)
                    report['floors'] = copy.deepcopy(floors(report, 'old_product'))
                    with self.assertRaises(ValueError):
                        check_floors(report)
                    self.refused(candidate, previous, message='')

    def test_null_and_false_override_fields_are_accepted_in_the_predecessor(self):
        null = dict(cap_override=None, article_cap_override=None, historical_policy=None, non_current_tail=None,
                    not_yet_eligible=False, mapping_incomplete_count=1)
        previous, candidate = self.pair()
        self.consistent(previous, 'old_product', **null)
        self.consistent(previous, 'development', **null)
        previous['floors'] = copy.deepcopy(floors(previous, 'old_product'))
        self.validate(candidate, previous)

    def test_rejection_counts_follow_the_cohorts(self):
        for cohort, count in (('old_product', 2), ('development', 1), ('development_period', 1), ('legacy', 1)):
            with self.subTest(cohort=cohort):
                previous, candidate = self.pair()
                candidate['cohorts'][cohort]['rejection_count'] = count
                self.refused(candidate, previous, message='rejection')


class VersionTwoFloorTest(unittest.TestCase):
    def pair(self):
        return (build_report(2, PREVIOUS_REL, FIXTURE, CURRENT_RANKS),
                build_report(2, CANDIDATE_REL, FIXTURE, CURRENT_RANKS))

    def validate(self, candidate, previous=None):
        validate_report(candidate, 'vn', candidate['rel'], previous_report=previous,
                        release_row=recipe_fixture(candidate['retrieval_recipe'])[0])

    def refused(self, candidate, previous=None, message='floor|rejection'):
        for given in (None, previous):
            with self.assertRaisesRegex(ValueError, message):
                self.validate(candidate, given)

    def test_passing_floors_are_accepted_with_and_without_a_previous_report(self):
        previous, candidate = self.pair()
        self.validate(candidate)
        self.validate(candidate, previous)

    def test_a_failed_or_low_floor_is_refused_without_a_previous_report_check(self):
        for cohort in ('old_product', 'development'):
            for mode in ('exact', 'ann'):
                with self.subTest(cohort=cohort, mode=mode):
                    previous, candidate = self.pair()
                    set_precision(candidate, cohort, mode, 1, 2)
                    edit_floor(candidate, cohort, mode, status='fail')
                    self.refused(candidate, previous)
                    previous, candidate = self.pair()
                    set_precision(candidate, cohort, mode, 6, 10)
                    self.refused(candidate, previous)

    def test_floor_records_and_top_level_floors_are_checked(self):
        cases = {'no floors': lambda report: report['cohorts']['old_product'].pop('floors'),
                 'top level differs': lambda report: report['floors'][0].update(value=.995),
                 'top level float limit': lambda report: report['floors'][0].update(limit=8.0),
                 'float denominator': lambda report: edit_floor(report, 'old_product', 'ann', denominator=10.0),
                 'not run populated': lambda report: edit_floor(report, 'development', 'exact', status='not_run')}
        for name, change in cases.items():
            with self.subTest(case=name):
                previous, candidate = self.pair()
                change(candidate)
                self.refused(candidate, previous, message='')

    def test_rejection_counts_follow_the_cohorts(self):
        for cohort, count in (('old_product', 2), ('development', 1), ('legacy', 1), ('old_product', 3.0),
                              ('old_product', True)):
            with self.subTest(cohort=cohort, count=count):
                previous, candidate = self.pair()
                candidate['cohorts'][cohort]['rejection_count'] = count
                self.refused(candidate, previous, message='rejection')

    def test_the_version_two_report_has_no_period_cohort_to_check(self):
        _, candidate = self.pair()
        self.assertNotIn('development_period', candidate['cohorts'])
        self.validate(candidate)


if __name__ == '__main__':
    unittest.main()
