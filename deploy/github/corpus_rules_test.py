from pathlib import Path
import re
import unittest
from unittest.mock import patch

import corpus_rules


class CorpusRulesTest(unittest.TestCase):
    def test_cutoff_literal_stays_on_one_line(self):
        text = Path(corpus_rules.__file__).read_text()
        self.assertIsNotNone(re.search(r"^LEGACY_CUTOFF = \{'vn': '20261118_1'\}$", text, re.MULTILINE))
        self.assertEqual(corpus_rules.LEGACY_CUTOFF, {'vn': '20261118_1'})

    def test_rel_order_is_date_then_numeric_suffix(self):
        ordered = ['20990101', '20990101_1', '20990101_2', '20990101_10', '20990102']
        self.assertEqual(sorted(reversed(ordered), key=corpus_rules.rel_order), ordered)
        self.assertEqual(corpus_rules.rel_order('20990101'), corpus_rules.rel_order('20990101_0'))

    def test_rel_order_refuses_other_shapes(self):
        for value in ('2099-01-01', '20990101_', '20990101_a', '', None, 20990101):
            with self.subTest(value=value), self.assertRaises(ValueError):
                corpus_rules.rel_order(value)

    @patch.dict('corpus_rules.LEGACY_CUTOFF', {'vn': '20990102_2'}, clear=True)
    def test_upload_takes_the_older_shape_only_at_the_cutoff(self):
        allowed = {rel: corpus_rules.older_shape_allowed('vn', rel, signing=False)
                   for rel in ('20990102_1', '20990102_2', '20990102_3')}
        self.assertEqual(allowed, {'20990102_1': False, '20990102_2': True, '20990102_3': False})

    @patch.dict('corpus_rules.LEGACY_CUTOFF', {'vn': '20990102_2'}, clear=True)
    def test_signing_takes_the_older_shape_at_or_below_the_cutoff(self):
        allowed = {rel: corpus_rules.older_shape_allowed('vn', rel, signing=True)
                   for rel in ('20990101', '20990102_2', '20990102_10', '20990103_1')}
        self.assertEqual(allowed, {'20990101': True, '20990102_2': True, '20990102_10': False, '20990103_1': False})

    def test_country_without_cutoff_never_takes_the_older_shape(self):
        for signing in (False, True):
            self.assertFalse(corpus_rules.older_shape_allowed('us', '20000101_1', signing=signing))

    @patch.dict('corpus_rules.LEGACY_CUTOFF', {'vn': '20990102_2'}, clear=True)
    def test_first_signing_without_a_decision_needs_exactly_the_cutoff(self):
        allowed = {rel: corpus_rules.first_signing_allowed('vn', rel, {})
                   for rel in ('20990101_1', '20990102_1', '20990102_2', '20990102_3')}
        self.assertEqual(allowed, {'20990101_1': False, '20990102_1': False, '20990102_2': True, '20990102_3': False})
        self.assertFalse(corpus_rules.first_signing_allowed('us', '20990102_2', {}))

    def test_first_signing_with_a_decision_is_not_limited_by_the_cutoff(self):
        for field in corpus_rules.DECISION_FIELDS:
            self.assertTrue(corpus_rules.first_signing_allowed('vn', '20990103_1', {field: 'a' * 64}))

    def test_decision_fields_are_refused_until_the_sidecar_gate(self):
        corpus_rules.refuse_until_sidecar_gate(dict(run={}))
        for field in corpus_rules.DECISION_FIELDS:
            with self.subTest(field=field), self.assertRaisesRegex(ValueError, 'sidecar gate'):
                corpus_rules.refuse_until_sidecar_gate(dict(run={field: 'a' * 64}))

    def test_status_endpoint_serves_only_vietnam(self):
        corpus_rules.require_status_country('vn')
        with self.assertRaises(ValueError):
            corpus_rules.require_status_country('us')


if __name__ == '__main__':
    unittest.main()
