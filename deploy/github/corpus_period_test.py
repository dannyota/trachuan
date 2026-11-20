import copy
from pathlib import Path
import tempfile
import unittest

from corpus_manifest import non_regression_contract, validate_report
from corpus_revision_support import (CANDIDATE_REL, CURRENT_CASES, CURRENT_RANKS, PREVIOUS_CASES, PREVIOUS_RANKS,
                                     PREVIOUS_REL, base_revision_records, build_report, cohort_metrics, encode,
                                     fixture_case, history_for, link_period, metric_value, revision_pair)
from corpus_test_support import recipe_fixture


class PeriodCohortTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name)
        self.repositories = 0

    def validate(self, candidate, previous, *fixtures):
        self.repositories += 1
        history = history_for(self.path / str(self.repositories), *fixtures)
        validate_report(candidate, 'vn', candidate['rel'], previous_report=previous,
                        release_row=recipe_fixture(candidate['retrieval_recipe'])[0], history=history)

    def refused(self, candidate, previous, *fixtures, message=''):
        with self.assertRaisesRegex(ValueError, message):
            self.validate(candidate, previous, *fixtures)

    def same_fixture_pair(self, previous_ranks=None):
        fixture = encode(CURRENT_CASES)
        previous = build_report(3, PREVIOUS_REL, fixture, previous_ranks or CURRENT_RANKS)
        candidate = build_report(3, CANDIDATE_REL, fixture, CURRENT_RANKS)
        link_period(candidate, previous)
        return previous, candidate, fixture

    def test_contract_lists_six_non_gating_period_records(self):
        self.assertEqual(non_regression_contract()['development_period'],
                         [dict(mode=mode, arms='hybrid', limit=limit, name=name, gating=False)
                          for mode in ('exact', 'ann') for name, limit in (('recall', 20), ('recall', 8), ('mrr', 8))])

    def test_period_fall_never_gates(self):
        previous, candidate, fixture = self.same_fixture_pair(CURRENT_RANKS | dict(m=([1], [1])))
        records = candidate['cohorts']['development_period']['non_regression']
        self.assertEqual({(record['value'], record['previous_value']) for record in records}, {(0., 1.)})
        self.validate(candidate, previous, fixture)

    def test_period_records_keep_their_contract(self):
        mutations = [lambda records: records[0].update(gating=True),
                     lambda records: records[0].update(status='pass'),
                     lambda records: records[0].update(revision=dict(cases=1, value=0., revised=[])),
                     lambda records: records[0].update(high_water=0.),
                     lambda records: records[0].update(numerator=0),
                     lambda records: records.pop(),
                     lambda records: records.append(copy.deepcopy(records[0])),
                     lambda records: records[0].update(limit=8),
                     lambda records: records[0].pop('previous_value')]
        for index, mutate in enumerate(mutations):
            with self.subTest(index=index):
                previous, candidate, fixture = self.same_fixture_pair()
                mutate(candidate['cohorts']['development_period']['non_regression'])
                self.refused(candidate, previous, fixture)

    def test_tampered_period_values_are_refused(self):
        for field, value in (('value', 1.), ('value', None), ('value', False), ('previous_value', 1.), ('previous_value', None),
                             ('previous_value', False)):
            with self.subTest(field=field, value=value):
                previous, candidate, fixture = self.same_fixture_pair()
                candidate['cohorts']['development_period']['non_regression'][2][field] = value
                self.refused(candidate, previous, fixture, message='period')

    def test_previous_value_is_null_without_a_previous_period_cohort(self):
        previous, candidate, before, after = revision_pair()
        base_revision_records(candidate)
        self.validate(candidate, previous, before, after)
        candidate['cohorts']['development_period']['non_regression'][0]['previous_value'] = 0.
        self.refused(candidate, previous, before, after, message='period')

    def test_period_cohort_has_no_floors_or_targets(self):
        for field in ('floors', 'targets'):
            previous, candidate, fixture = self.same_fixture_pair()
            candidate['cohorts']['development_period'][field] = []
            with self.subTest(field=field):
                self.refused(candidate, previous, fixture, message='period')

    def test_empty_period_cohort_needs_not_run(self):
        fixture = encode(PREVIOUS_CASES)
        previous = build_report(3, PREVIOUS_REL, fixture, PREVIOUS_RANKS)
        candidate = build_report(3, CANDIDATE_REL, fixture, PREVIOUS_RANKS)
        self.assertEqual(candidate['not_run'], dict(development_period='not_run'))
        self.validate(candidate, previous, fixture)
        for mutate in (lambda r: r['not_run'].pop('development_period'),
                       lambda r: r['cohorts']['development_period'].update(non_regression=[])):
            report = copy.deepcopy(candidate)
            mutate(report)
            with self.subTest(report=report['not_run']):
                self.refused(report, previous, fixture, message='period')
        populated = build_report(3, CANDIDATE_REL, encode(CURRENT_CASES), CURRENT_RANKS)
        populated['not_run']['development_period'] = 'not_run'
        with self.assertRaisesRegex(ValueError, 'period'):
            validate_report(populated, 'vn', CANDIDATE_REL, release_row=recipe_fixture()[0],
                            history=history_for(self.path / 'populated', encode(CURRENT_CASES)))


class PartitionTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name)

    def check(self, report, *fixtures):
        validate_report(report, 'vn', report['rel'], release_row=recipe_fixture()[0],
                        history=history_for(self.path / str(len(list(self.path.iterdir()))), *fixtures))

    def test_cohorts_follow_the_historical_tag_of_the_fixture(self):
        fixture = encode(CURRENT_CASES)
        self.check(build_report(3, CANDIDATE_REL, fixture, CURRENT_RANKS), fixture)
        for cohorts in (dict(m='development'), dict(p1='development_period')):
            report = build_report(3, CANDIDATE_REL, fixture, CURRENT_RANKS, cohorts=cohorts)
            with self.subTest(cohorts=cohorts), self.assertRaisesRegex(ValueError, 'partition'):
                self.check(report, fixture)

    def test_every_fixture_case_needs_its_cohort(self):
        fixture = encode(CURRENT_CASES)
        report = build_report(3, CANDIDATE_REL, encode(CURRENT_CASES[:-1]), CURRENT_RANKS)
        report['fixture_hashes']['Development'] = build_report(3, CANDIDATE_REL, fixture, CURRENT_RANKS)['fixture_hashes']['Development']
        with self.assertRaisesRegex(ValueError, 'partition'):
            self.check(report, fixture)
        extra = build_report(3, CANDIDATE_REL, encode(CURRENT_CASES + [fixture_case('x')]), CURRENT_RANKS | dict(x=([1], [1])))
        extra['fixture_hashes']['Development'] = report['fixture_hashes']['Development']
        with self.assertRaisesRegex(ValueError, 'partition'):
            self.check(extra, fixture)

    def test_every_case_holds_a_hybrid_row_in_each_mode_and_limit(self):
        fixture = encode(CURRENT_CASES)
        previous = build_report(3, PREVIOUS_REL, fixture, CURRENT_RANKS)
        for mode, limit in (('exact', 8), ('exact', 20), ('ann', 8), ('ann', 20), (None, None)):
            with self.subTest(mode=mode, limit=limit):
                candidate = build_report(3, CANDIDATE_REL, fixture, CURRENT_RANKS | dict(p3=([0], [0]) if mode is None else ([6], [6])))
                link_period(candidate, previous)
                dropped = [row for row in candidate['cases'] if row['case_id'] == 'p3' and
                           (mode is None or (row['mode'], row['limit']) == (mode, limit))]
                candidate['cases'] = [row for row in candidate['cases'] if not any(row is drop for drop in dropped)]
                candidate['cohorts']['development']['metrics'] = cohort_metrics(candidate['cases'], 'development')
                candidate['cases'].append(dropped[0] | dict(arms='dense'))
                for record in candidate['cohorts']['development']['non_regression']:
                    if not record['gating']:
                        continue
                    key = (record['mode'], record['name'], record['limit'])
                    value, prior = metric_value(candidate, 'development', *key), metric_value(previous, 'development', *key)
                    record.update(value=value, previous_value=prior)
                    if record['name'] == 'mrr':
                        record.update(previous=prior, high_water=prior, tolerance=0, within_tolerance=value < prior, losses=[])
                with self.assertRaisesRegex(ValueError, 'partition'):
                    validate_report(candidate, 'vn', CANDIDATE_REL, previous_report=previous,
                                    release_row=recipe_fixture()[0],
                                    history=history_for(self.path / f'hybrid-{mode}-{limit}', fixture))

    def test_every_period_case_holds_a_hybrid_row_in_each_mode_and_limit(self):
        fixture = encode(CURRENT_CASES)
        previous = build_report(3, PREVIOUS_REL, fixture, CURRENT_RANKS)
        candidate = build_report(3, CANDIDATE_REL, fixture, CURRENT_RANKS)
        dropped = next(row for row in candidate['cases'] if (row['case_id'], row['mode'], row['limit']) == ('m', 'ann', 20))
        candidate['cases'].remove(dropped)
        candidate['cohorts']['development_period']['metrics'] = cohort_metrics(candidate['cases'], 'development_period')
        candidate['cases'].append(dropped | dict(arms='lexical'))
        link_period(candidate, previous)
        for record in candidate['cohorts']['development_period']['non_regression']:
            record['value'] = metric_value(candidate, 'development_period', record['mode'], record['name'], record['limit'])
        with self.assertRaisesRegex(ValueError, 'hybrid row'):
            validate_report(candidate, 'vn', CANDIDATE_REL, previous_report=previous, release_row=recipe_fixture()[0],
                            history=history_for(self.path / 'period-hybrid', fixture))

    def test_case_records_hold_the_fixture_citation_count(self):
        fixture = encode(CURRENT_CASES)
        previous = build_report(3, PREVIOUS_REL, fixture, CURRENT_RANKS)
        candidate = build_report(3, CANDIDATE_REL, fixture, CURRENT_RANKS | dict(p2=([4], [4])))
        link_period(candidate, previous)
        for record in candidate['cohorts']['development']['non_regression']:
            if record['gating']:
                record['previous_value'] = metric_value(previous, 'development', record['mode'], record['name'], record['limit'])
            if record['gating'] and record['name'] == 'mrr':
                record.update(previous=record['previous_value'], high_water=record['previous_value'], tolerance=0)
        with self.assertRaisesRegex(ValueError, 'development case records differ from the fixture partition'):
            validate_report(candidate, 'vn', CANDIDATE_REL, previous_report=previous, release_row=recipe_fixture()[0],
                            history=history_for(self.path / 'citations', fixture))

    def test_report_fixture_must_be_in_main_history(self):
        report = build_report(3, CANDIDATE_REL, encode(CURRENT_CASES), CURRENT_RANKS)
        with self.assertRaisesRegex(ValueError, 'not in the history'):
            self.check(report, encode(PREVIOUS_CASES))


if __name__ == '__main__':
    unittest.main()
