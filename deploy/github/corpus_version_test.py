import copy
from pathlib import Path
import tempfile
import unittest

from corpus_comparison import compatible, versions
from corpus_comparison_test import reports
from corpus_fixture_history import REPOSITORY, FixtureHistory
from corpus_manifest import validate_report
from corpus_revision_support import (CANDIDATE_REL, CURRENT_CASES, CURRENT_RANKS, PREVIOUS_REL, base_revision_records,
                                     build_report, encode, history_for, link_period, revision_pair)
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
            for value, allowed in ((None, True), ('agent', False)):
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

    def test_default_history_reads_the_checkout_that_holds_the_scripts(self):
        self.assertEqual(REPOSITORY, Path(__file__).resolve().parents[2])
        self.assertEqual(FixtureHistory().root, REPOSITORY)


if __name__ == '__main__':
    unittest.main()
