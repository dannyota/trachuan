import copy
from pathlib import Path
import tempfile
import unittest

from corpus_manifest import validate_report
from corpus_revision_support import (CANDIDATE_MRR, CANDIDATE_REL, CURRENT_CASES, CURRENT_RANKS, PREVIOUS_CASES,
                                     PREVIOUS_RANKS, PREVIOUS_REL, PRIOR_MRR, REVISED, base_revision_records,
                                     build_report, encode, fixture_case, history_for, link_period, revision_pair,
                                     set_records)
from corpus_test_support import digest, recipe_fixture


class RevisionTest(unittest.TestCase):
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

    def test_tag_only_change_and_cohort_move_keep_comparable_cases(self):
        previous, candidate, before, after = revision_pair()
        base_revision_records(candidate)
        self.validate(candidate, previous, before, after)

    def test_comparable_set_and_reasons_follow_the_evaluator(self):
        from corpus_revision import compare_fixtures
        previous = [fixture_case('a', question='qa'), fixture_case('b', question='qb'), fixture_case('c', question='qc'),
                    fixture_case('d', 0, 'negative', question='qd'), fixture_case('e', question='qe'),
                    fixture_case('f', question='qf'), fixture_case('i', 1, 'historical', question='qi'),
                    fixture_case('j', 1, 'historical', question='qj')]
        current = [fixture_case('g', question='qg'), fixture_case('a', 1, 'identifier', 'transition', question='qa'),
                   fixture_case('b', question='qb changed'), fixture_case('c', 2, question='qc'),
                   fixture_case('d', 0, 'negative', question='qd') | dict(notes='notes are ignored'),
                   fixture_case('e', 1, 'historical', question='qe'),
                   fixture_case('f', 1, 'historical', question='qf changed'),
                   fixture_case('h', 1, 'historical', question='qh'), fixture_case('i', question='qi'),
                   fixture_case('j', 1, 'historical', 'multi_document', question='qj')]
        cohorts = dict.fromkeys('abcdef', 'development') | dict.fromkeys('ij', 'development_period')
        revision = compare_fixtures(previous, current, cohorts)
        self.assertEqual(revision.comparable, ['a', 'd'])
        self.assertEqual(revision.previous_order, ['a', 'd'])
        self.assertEqual(revision.revised, [dict(case_id='b', reason='changed'), dict(case_id='c', reason='changed'),
                                            dict(case_id='e', reason='cohort'), dict(case_id='f', reason='cohort'),
                                            dict(case_id='g', reason='added'), dict(case_id='i', reason='cohort')])
        absent = fixture_case('a', question='qa') | dict(expected_citations=[dict(doc_number='doc', alt_doc_numbers=[],
                                                                                   relation_ok=False, article=None)])
        self.assertEqual(compare_fixtures([previous[0]], [absent], dict(a='development')).comparable, ['a'])

    def test_changed_case_leaves_the_comparable_set(self):
        current = copy.deepcopy(CURRENT_CASES)
        current[2]['question'] = 'changed'
        previous, candidate, before, after = revision_pair(current_cases=current)
        base_revision_records(candidate)
        self.refused(candidate, previous, before, after)
        revised = [dict(case_id='m', reason='cohort'), dict(case_id='p3', reason='changed')]
        set_records(candidate, 'development', 'recall', previous_value=2 / 3, numerator=3, denominator=3,
                    previous_numerator=2, previous_denominator=3, revision=dict(cases=3, value=1., revised=revised))
        prior, value = (1 / 2 + 1 / 3) / 2, (1 + 1 / 4) / 2
        set_records(candidate, 'development', 'mrr', previous_value=prior, previous=prior, high_water=prior,
                    losses=[dict(case_id='p2', previous_rank=3, rank=4)],
                    revision=dict(cases=3, value=value, revised=revised))
        self.validate(candidate, previous, before, after)

    def test_added_case_is_revised_and_a_new_period_case_is_not(self):
        for case, cohort_value in ((fixture_case('g'), (1 + 1 / 4 + 1 / 6 + 1) / 4), (fixture_case('h', 1, 'historical'), None)):
            with self.subTest(case=case['id']):
                ranks = CURRENT_RANKS | {case['id']: ([1], [1])}
                previous, candidate, before, after = revision_pair(current_cases=CURRENT_CASES + [case], current_ranks=ranks)
                base_revision_records(candidate)
                if cohort_value is not None:
                    self.assertEqual(candidate['cohorts']['development']['non_regression'][1]['value'], cohort_value)
                    self.refused(candidate, previous, before, after, message='revised comparison differs')
                    revised = [dict(case_id='g', reason='added'), *REVISED]
                    for name in ('recall', 'mrr'):
                        for record in candidate['cohorts']['development']['non_regression']:
                            if record['name'] == name:
                                record['revision']['revised'] = revised
                self.validate(candidate, previous, before, after)

    def test_removal_or_rename_is_refused(self):
        renamed = [case if case['id'] != 'p3' else case | dict(id='p4') for case in CURRENT_CASES]
        for current in ([case for case in CURRENT_CASES if case['id'] != 'p3'], renamed):
            ranks = {key: value for key, value in CURRENT_RANKS.items() if key != 'p3'} | dict(p4=([6], [6]))
            previous, candidate, before, after = revision_pair(current_cases=current, current_ranks=ranks)
            with self.subTest(ids=[case['id'] for case in current]):
                self.refused(candidate, previous, before, after, message='removes case p3')

    def test_values_are_summed_in_each_fixture_order(self):
        cases = [fixture_case(case_id) for case_id in 'abc']
        ranks = dict(a=([1], [1]), b=([2], [2]), c=([6], [6]))
        previous, candidate, before, after = revision_pair(cases, cases[::-1], ranks, ranks)
        prior, value = (1 + 1 / 2 + 1 / 6) / 3, (1 / 6 + 1 / 2 + 1) / 3
        self.assertNotEqual(prior, value)
        revision = dict(cases=3, revised=[])
        set_records(candidate, 'development', 'recall', previous_value=1., numerator=3, denominator=3,
                    previous_numerator=3, previous_denominator=3, revision=revision | dict(value=1.))
        set_records(candidate, 'development', 'mrr', value=value, previous_value=prior, previous=prior,
                    high_water=prior, within_tolerance=True, revision=revision | dict(value=value))
        self.validate(candidate, previous, before, after)
        set_records(candidate, 'development', 'mrr', previous_value=value, previous=value, high_water=value,
                    within_tolerance=False)
        self.refused(candidate, previous, before, after)

    def test_recall_compares_integer_counts(self):
        previous_cases = [fixture_case('a', 2), fixture_case('b'), fixture_case('x')]
        current_cases = previous_cases[:2] + [fixture_case('x', question='changed')]
        previous_ranks = dict(a=([1, 2], [1, 2]), b=([0], [9]), x=([1], [1]))
        revised = [dict(case_id='x', reason='changed')]
        for b, passes in (([3], True), ([0], False)):
            with self.subTest(b=b):
                ranks = previous_ranks | dict(b=(b, b))
                previous, candidate, before, after = revision_pair(previous_cases, current_cases, previous_ranks, ranks)
                matched = 3 if passes else 2
                set_records(candidate, 'development', 'recall', previous_value=1., numerator=matched, denominator=3,
                            previous_numerator=3, previous_denominator=3,
                            revision=dict(cases=2, value=matched / 3, revised=revised))
                value = (1 + (1 / 3 if passes else 0)) / 2
                set_records(candidate, 'development', 'mrr', previous_value=.5, previous=.5, high_water=.5,
                            revision=dict(cases=2, value=value, revised=revised))
                if passes:
                    self.validate(candidate, previous, before, after)
                    set_records(candidate, 'development', 'recall', numerator=4)
                self.refused(candidate, previous, before, after)

    def test_high_water_carries_the_tolerance_the_previous_record_used(self):
        for name, fields, used in (('plain', dict(value=.7, high_water=.703), .703 - .7),
                                   ('revision', dict(value=.71, high_water=.703, revision=dict(cases=1, value=.7, revised=[])), .703 - .7),
                                   ('revision above watermark', dict(value=.71, high_water=.703, revision=dict(cases=1, value=.705, revised=[])), 0)):
            with self.subTest(name=name):
                previous, candidate, before, after = revision_pair(previous_version=3)
                set_records(previous, 'development', 'mrr', **fields)
                base_revision_records(candidate)
                set_records(candidate, 'development', 'mrr', high_water=PRIOR_MRR + used)
                self.validate(candidate, previous, before, after)
                set_records(candidate, 'development', 'mrr', high_water=PRIOR_MRR if used else PRIOR_MRR + .003)
                self.refused(candidate, previous, before, after, message='revised comparison differs')

    def test_recipe_change_drops_the_watermark_of_a_revision(self):
        previous, candidate, before, after = revision_pair()
        set_records(previous, 'development', 'mrr', value=.7, high_water=.703)
        candidate['retrieval_recipe']['dense_depth'] = 60
        candidate['recipes']['retrieval'] = recipe_fixture(candidate['retrieval_recipe'])[1]['retrieval']
        base_revision_records(candidate)
        set_records(candidate, 'development', 'mrr', tolerance=0)
        set_records(candidate, 'old_product', 'mrr', tolerance=0)
        self.validate(candidate, previous, before, after)

    def test_unrevised_high_water_after_a_revision_or_plain_previous_record(self):
        fixture = encode(CURRENT_CASES)
        for name, fields, expected in (
                ('revision', dict(high_water=CANDIDATE_MRR + .002, revision=dict(cases=1, value=CANDIDATE_MRR - .001, revised=[])),
                 CANDIDATE_MRR + ((CANDIDATE_MRR + .002) - (CANDIDATE_MRR - .001))),
                ('plain', dict(high_water=CANDIDATE_MRR + .002), CANDIDATE_MRR + .002),
                ('plain below its value', dict(high_water=CANDIDATE_MRR - .002), CANDIDATE_MRR)):
            with self.subTest(name=name):
                previous = build_report(3, PREVIOUS_REL, fixture, CURRENT_RANKS)
                candidate = build_report(3, CANDIDATE_REL, fixture, CURRENT_RANKS)
                link_period(candidate, previous)
                set_records(previous, 'development', 'mrr', **fields)
                set_records(candidate, 'development', 'mrr', high_water=expected)
                self.validate(candidate, previous, fixture)
                set_records(candidate, 'development', 'mrr', high_water=fields['high_water'] if 'below' in name else
                            max(CANDIDATE_MRR, fields['high_water']) if 'revision' in fields else CANDIDATE_MRR)
                self.refused(candidate, previous, fixture, message='MRR tolerance fields differ')

    def test_previous_record_without_high_water_follows_the_evaluator(self):
        fixture = encode(CURRENT_CASES)
        for status, accepted in (('no_previous', True), ('pass', False)):
            with self.subTest(status=status):
                previous = build_report(3, PREVIOUS_REL, fixture, CURRENT_RANKS)
                candidate = build_report(3, CANDIDATE_REL, fixture, CURRENT_RANKS)
                link_period(candidate, previous)
                set_records(previous, 'development', 'mrr', previous=None, high_water=None, status=status)
                if accepted:
                    self.validate(candidate, previous, fixture)
                    set_records(candidate, 'development', 'mrr', high_water=CANDIDATE_MRR + .001)
                    self.refused(candidate, previous, fixture, message='MRR tolerance fields differ')
                else:
                    self.refused(candidate, previous, fixture, message='invalid previous MRR previous')
                previous, candidate, before, after = revision_pair(previous_version=3)
                base_revision_records(candidate)
                set_records(previous, 'development', 'mrr', previous=None, high_water=None, status=status)
                if accepted:
                    self.validate(candidate, previous, before, after)
                else:
                    self.refused(candidate, previous, before, after, message='invalid previous MRR previous')

    def test_old_product_condition_three_ignores_the_development_fixture(self):
        previous, candidate, before, after = revision_pair()
        base_revision_records(candidate)
        self.assertEqual(candidate['cohorts']['old_product']['non_regression'][1]['tolerance'], .005)
        self.validate(candidate, previous, before, after)
        for key in ('Legacy', 'OldProduct'):
            changed = copy.deepcopy(candidate)
            changed['fixture_hashes'][key] = 'other'
            with self.subTest(key=key):
                self.refused(changed, previous, before, after, message='incompatible previous legacy or old product fixture')

    def test_tampered_revision_fields_are_refused(self):
        mutations = [('mrr', lambda r: r['revision'].update(value=r['revision']['value'] + 1e-9)),
                     ('mrr', lambda r: r['revision'].update(cases=5)),
                     ('mrr', lambda r: r['revision'].update(revised=[])),
                     ('mrr', lambda r: r['revision'].update(revised=[dict(case_id='m', reason='changed')])),
                     ('mrr', lambda r: r['revision'].update(revised=REVISED + [dict(case_id='z', reason='added')])),
                     ('mrr', lambda r: r.pop('revision')),
                     ('mrr', lambda r: r.update(high_water=r['high_water'] + 1e-9)),
                     ('mrr', lambda r: r.update(previous_value=CANDIDATE_MRR)),
                     ('mrr', lambda r: r.update(previous=CANDIDATE_MRR)),
                     ('mrr', lambda r: r.update(losses=[])),
                     ('mrr', lambda r: r.update(within_tolerance=True)),
                     ('mrr', lambda r: r.update(tolerance=0)),
                     ('mrr', lambda r: r.update(numerator=1)),
                     ('mrr', lambda r: r.update(status='fail')),
                     ('recall', lambda r: r['revision'].update(value=.75)),
                     ('recall', lambda r: r.update(numerator=3)),
                     ('recall', lambda r: r.update(denominator=5)),
                     ('recall', lambda r: r.update(previous_numerator=2)),
                     ('recall', lambda r: r.update(previous_denominator=5)),
                     ('recall', lambda r: r.update(previous_value=.8)),
                     ('recall', lambda r: r.update(high_water=1.)),
                     ('recall', lambda r: r.pop('numerator'))]
        for index, (name, mutate) in enumerate(mutations):
            with self.subTest(index=index):
                previous, candidate, before, after = revision_pair()
                base_revision_records(candidate)
                for record in candidate['cohorts']['development']['non_regression']:
                    if record['name'] == name and record['mode'] == 'exact':
                        mutate(record)
                self.refused(candidate, previous, before, after)

    def test_inconsistent_previous_reports_are_refused(self):
        previous_fixture = encode(PREVIOUS_CASES)
        cases = [('two cohorts', 'two development cohorts'), ('citation count', 'differs from its development fixture'),
                 ('missing', 'no case records'), ('extra', 'outside its development fixture')]
        for name, message in cases:
            with self.subTest(name=name):
                previous, candidate, before, after = revision_pair(previous_version=3)
                if name == 'two cohorts':
                    previous['cases'].append(dict(previous['cases'][-1], cohort='development_period'))
                elif name == 'citation count':
                    previous = build_report(3, PREVIOUS_REL, previous_fixture, PREVIOUS_RANKS | dict(p2=([0], [3])))
                else:
                    cases = PREVIOUS_CASES[:-1] if name == 'missing' else PREVIOUS_CASES + [fixture_case('z')]
                    previous = build_report(3, PREVIOUS_REL, encode(cases), PREVIOUS_RANKS | dict(z=([1], [1])))
                    previous['fixture_hashes']['Development'] = digest(before)
                self.refused(candidate, previous, before, after, message=message)

    def test_cohort_move_under_an_unchanged_fixture_is_refused(self):
        fixture = encode(CURRENT_CASES)
        previous = build_report(3, PREVIOUS_REL, fixture, CURRENT_RANKS, cohorts=dict(m='development'))
        candidate = build_report(3, CANDIDATE_REL, fixture, CURRENT_RANKS)
        self.refused(candidate, previous, fixture, message='unchanged development fixture')

    def test_fixture_history_order_and_presence(self):
        previous, candidate, before, after = revision_pair()
        base_revision_records(candidate)
        self.refused(candidate, previous, after, before, message='older')
        self.refused(candidate, previous, after, message='not in the history')
        self.refused(candidate, previous, before, message='not in the history')


if __name__ == '__main__':
    unittest.main()
