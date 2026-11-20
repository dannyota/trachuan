import copy
from pathlib import Path
import tempfile
import unittest

from corpus_comparison import compatible, versions
from corpus_comparison_test import reports
from corpus_fixture_history import REPOSITORY, FixtureHistory
from corpus_manifest import validate_report
from corpus_revision_support import (CANDIDATE_MRR, CANDIDATE_REL, CURRENT_CASES, CURRENT_RANKS, PREVIOUS_REL,
                                     base_revision_records, build_report, encode, history_for, link_period, revision_pair,
                                     set_records)
from corpus_test_support import recipe_fixture


class VersionThreeTest(unittest.TestCase):
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

    def test_version_three_needs_scorer_six_and_the_full_recipe(self):
        _, candidate, _, _ = revision_pair()
        versions(candidate)
        for version, scorer in ((3, 'agent-eval/5'), (2, 'agent-eval/6'), (4, 'agent-eval/6')):
            with self.subTest(version=version, scorer=scorer), self.assertRaises(ValueError):
                versions(candidate | dict(version=version, scorer=scorer))
        with self.assertRaisesRegex(ValueError, 'version 3 requires the full retrieval recipe'):
            versions({key: value for key, value in candidate.items() if key != 'retrieval_recipe'})

    def test_scorer_pairs(self):
        self.assertTrue(compatible('agent-eval/5', 'agent-eval/6'))
        self.assertTrue(compatible('agent-eval/6', 'agent-eval/5'))
        self.assertTrue(compatible('agent-eval/4', 'agent-eval/5'))
        for pair in (('agent-eval/4', 'agent-eval/6'), ('agent-eval/3', 'agent-eval/6'), ('agent-eval/3', 'agent-eval/5')):
            with self.subTest(pair=pair):
                self.assertFalse(compatible(*pair))

    def test_version_three_refuses_an_incompatible_previous_scorer(self):
        previous, candidate, before, after = revision_pair()
        base_revision_records(candidate)
        self.validate(candidate, previous, before, after)
        previous.update(version=1, scorer='agent-eval/4')
        with self.assertRaisesRegex(ValueError, 'incompatible previous scorer'):
            self.validate(candidate, previous, before, after)

    def test_version_three_cannot_downgrade_to_two(self):
        fixture = encode(CURRENT_CASES)
        previous = build_report(3, PREVIOUS_REL, fixture, CURRENT_RANKS)
        candidate = build_report(3, CANDIDATE_REL, fixture, CURRENT_RANKS)
        link_period(candidate, previous)
        self.validate(candidate, previous, fixture)
        _, older = reports()
        self.assertEqual((older['version'], older['rel'], older['previous_diff']['previous']['rel']), (2, CANDIDATE_REL, PREVIOUS_REL))
        with self.assertRaisesRegex(ValueError, 'downgrade from 3 to 2'):
            self.validate(older, previous, fixture)

    def test_historical_policy_is_refused_in_the_candidate_and_the_previous_report(self):
        previous, candidate, before, after = revision_pair()
        base_revision_records(candidate)
        for target, place in ((candidate, 'top'), (candidate, 'record'), (candidate, 'recipe'), (previous, 'top'),
                              (previous, 'record')):
            for value, allowed in ((None, True), ('agent', False), ('', False), (False, False)):
                with self.subTest(target=target['rel'], place=place, value=value):
                    report = copy.deepcopy(target)
                    holder = dict(top=report, record=report['cohorts']['development']['non_regression'][0],
                                  recipe=report['retrieval_recipe'])[place]
                    holder['historical_policy'] = value
                    report['recipes']['retrieval'] = recipe_fixture(report['retrieval_recipe'])[1]['retrieval']
                    pair = (report, previous) if target is candidate else (candidate, report)
                    if place == 'recipe' and allowed:
                        continue
                    if allowed:
                        self.validate(*pair, before, after)
                    else:
                        with self.assertRaisesRegex(ValueError, 'historical_policy|research override'):
                            self.validate(*pair, before, after)

    def test_records_carry_only_their_own_comparison_fields(self):
        mutations = [('old_product', 'recall', dict(revision=dict(cases=1, value=1., revised=[]))),
                     ('old_product', 'recall', dict(numerator=1)),
                     ('development', 'abstention_balanced_accuracy', dict(high_water=1.)),
                     ('development', 'abstention_balanced_accuracy', dict(revision=dict(cases=1, value=1., revised=[]))),
                     ('development', 'recall', dict(losses=[]))]
        for index, (cohort, name, fields) in enumerate(mutations):
            with self.subTest(cohort=cohort, name=name, fields=list(fields)):
                _, candidate, _, after = revision_pair()
                base_revision_records(candidate)
                for record in candidate['cohorts'][cohort]['non_regression']:
                    if record['name'] == name:
                        record.update(fields)
                candidate['non_regression'] = copy.deepcopy(candidate['cohorts']['old_product']['non_regression'])
                with self.assertRaisesRegex(ValueError, 'fields of another comparison'):
                    validate_report(candidate, 'vn', CANDIDATE_REL, release_row=recipe_fixture()[0],
                                    history=history_for(self.path / f'fields-{index}', after))

    def same_fixture_pair(self):
        fixture = encode(CURRENT_CASES)
        previous = build_report(3, PREVIOUS_REL, fixture, CURRENT_RANKS)
        candidate = build_report(3, CANDIDATE_REL, fixture, CURRENT_RANKS)
        link_period(candidate, previous)
        return previous, candidate, fixture

    def test_unrevised_record_cannot_carry_revision_fields(self):
        for name, fields in (('mrr', dict(revision=dict(cases=4, value=1., revised=[]))),
                             ('recall', dict(numerator=4, denominator=4, previous_numerator=4, previous_denominator=4))):
            with self.subTest(name=name):
                previous, candidate, fixture = self.same_fixture_pair()
                set_records(candidate, 'development', name, **fields)
                with self.assertRaisesRegex(ValueError, 'revision fields without'):
                    self.validate(candidate, previous, fixture)

    def test_unrevised_watermark_does_not_cross_a_recipe_change(self):
        previous, candidate, fixture = self.same_fixture_pair()
        set_records(previous, 'development', 'mrr', high_water=CANDIDATE_MRR + .002)
        candidate['retrieval_recipe']['dense_depth'] = 60
        candidate['recipes']['retrieval'] = recipe_fixture(candidate['retrieval_recipe'])[1]['retrieval']
        for cohort in ('old_product', 'development'):
            set_records(candidate, cohort, 'mrr', tolerance=0)
        self.validate(candidate, previous, fixture)
        set_records(candidate, 'development', 'mrr', high_water=CANDIDATE_MRR + .002)
        with self.assertRaisesRegex(ValueError, 'MRR tolerance fields differ'):
            self.validate(candidate, previous, fixture)

    def test_previous_revision_must_have_the_evaluator_shape(self):
        valid = dict(cases=1, value=.7, revised=[dict(case_id='x', reason='added')])
        for revision in (None, [], dict(valid, cases='1'), dict(valid, cases=None), {k: v for k, v in valid.items() if k != 'value'},
                         dict(valid, value='0.7'), dict(valid, revised=None), dict(valid, revised=[dict(case_id='', reason='added')]),
                         dict(valid, revised=[dict(case_id='x', reason='moved')]), dict(valid, revised=['x'])):
            with self.subTest(revision=revision):
                previous, candidate, fixture = self.same_fixture_pair()
                set_records(previous, 'development', 'mrr', high_water=CANDIDATE_MRR + .002, revision=revision)
                with self.assertRaisesRegex(ValueError, 'invalid previous revis'):
                    self.validate(candidate, previous, fixture)
        previous, candidate, fixture = self.same_fixture_pair()
        set_records(previous, 'development', 'mrr', high_water=CANDIDATE_MRR + .002, revision=dict(valid, value=None))
        set_records(candidate, 'development', 'mrr', high_water=CANDIDATE_MRR)
        self.validate(candidate, previous, fixture)

    def test_version_three_record_checks_run_without_a_previous_report(self):
        mutations = [('retrieval recipe object differs', lambda r: r['retrieval_recipe'].update(dense_depth=60)),
                     ('missing MRR tolerance fields', lambda r: set_records(r, 'development', 'mrr', losses=None)),
                     ('missing or non-finite comparison value', lambda r: set_records(r, 'old_product', 'mrr', high_water=None))]
        for message, mutate in mutations:
            with self.subTest(message=message):
                _, candidate, fixture = self.same_fixture_pair()
                row = recipe_fixture(candidate['retrieval_recipe'])[0]
                mutate(candidate)
                with self.assertRaisesRegex(ValueError, message):
                    validate_report(candidate, 'vn', CANDIDATE_REL, release_row=row,
                                    history=history_for(self.path / message.replace(' ', '-'), fixture))

    def test_default_history_reads_the_checkout_that_holds_the_scripts(self):
        self.assertEqual(REPOSITORY, Path(__file__).resolve().parents[2])
        self.assertEqual(FixtureHistory().root, REPOSITORY)


if __name__ == '__main__':
    unittest.main()
