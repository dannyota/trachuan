import json
from pathlib import Path
import re
import unittest

from updater import deploy_corpus_sql as sql
from updater import deploy_corpus_verify as v
from updater.corpus_verify_test import FIXTURE, description, manifest
from updater.deploy_io import canonical
from updater.verification_test import FIXTURE_POLICY
from updater.world_test import Crash, LawCase


EVIDENCE = dict(findings_sha256='0123456789abcdef' * 4, identity_sha256='fedcba9876543210' * 4,
                changes_sha256='00ff' * 16)
HEADINGS = dict(projection=1, scan_sha256='a' * 64, exclusions_sha256='b' * 64)


class CorpusEvidenceTest(unittest.TestCase):
    def setUp(self):
        self.rules = json.loads(json.dumps(FIXTURE_POLICY))
        self.m = manifest()

    def verified(self, obj):
        return v.manifest(canonical(obj), self.rules)

    def test_valid_evidence_verifies_and_is_returned(self):
        self.assertEqual(self.verified(dict(self.m, evidence=EVIDENCE)), dict(self.m, evidence=EVIDENCE))
        headed = dict(self.m, schema_version=8, heading_evidence=HEADINGS,
                      heading_review=dict(HEADINGS, reviewer='coordinator', date='2099-01-02'), evidence=EVIDENCE)
        self.assertEqual(self.verified(headed), headed)

    def test_manifest_without_evidence_verifies_unchanged(self):
        obj = v.manifest(FIXTURE.read_bytes(), self.rules)
        self.assertEqual(obj, self.m)
        self.assertNotIn('evidence', obj)

    def test_each_refused_shape_names_its_fault(self):
        fields = 'invalid corpus evidence fields'
        partial = {key: value for key, value in EVIDENCE.items() if key != 'changes_sha256'}
        cases = [(None, fields), ([], fields), (list(EVIDENCE.values()), fields), ('a' * 64, fields), (1, fields),
                 ({}, fields), (partial, fields), (dict(EVIDENCE, extra='1' * 64), fields),
                 (dict(EVIDENCE, FINDINGS_SHA256='1' * 64), fields)]
        for field in EVIDENCE:
            for bad in ('A' * 64, 'Ab' * 32, 'sha256:' + '1' * 64, '1' * 63, '1' * 65, 'g' * 64,
                        ' ' + '1' * 63, ' ' + '1' * 64, '1' * 64 + ' ', '1' * 64 + '\n', '\u0661' * 64,
                        int('1' * 64), '', None, 1, True, ['1' * 64], {'sha256': '1' * 64}):
                cases.append((dict(EVIDENCE, **{field: bad}), 'invalid evidence ' + field))
        for value, message in cases:
            with self.subTest(value=value), self.assertRaisesRegex(ValueError, '^' + re.escape(message) + '$'):
                self.verified(dict(self.m, evidence=value))


class EvidenceActivationTest(LawCase, unittest.TestCase):
    def signed(self, m):
        self.m = m
        self.write_manifest()
        self.db.describe.return_value = description(m)

    def activate(self, m):
        self.setUp()
        self.baseline()
        self.signed(m)
        self.db.crashes.add('append:restored')
        with self.assertRaises(Crash):
            self.run_phase()
        crashed = self.phase
        self.assertEqual(self.stored()['operation']['step'], 'restored')
        self.assertTrue(self.again())
        crashed.download.assert_called_once_with()
        self.phase.download.assert_not_called()
        records = {key: record['status'] for key, record in self.state.records('corpus').items()}
        stored = (self.paths.state / self.kind.artifacts / self.digest / 'manifest.json').read_bytes()
        return self.result(), self.db, records, self.stored(), stored

    def test_evidence_survives_journal_and_recovery_and_changes_no_activation_step(self):
        plain = manifest()
        signed = dict(plain, evidence=EVIDENCE)
        expected, plain_db, plain_records, plain_record, _ = self.activate(plain)
        result, db, records, record, stored = self.activate(signed)
        self.assertEqual(result['status'], 'ok')
        self.assertEqual(dict(result, manifest_hash=None), dict(expected, manifest_hash=None))
        self.assertEqual(records, plain_records)
        self.assertEqual(dict(record['active'], manifest_hash=None), dict(plain_record['active'], manifest_hash=None))
        self.assertEqual(record['active']['database'], signed['database'])
        self.assertEqual(record['retired'], plain_record['retired'])
        self.assertEqual(db.pointer_value, plain_db.pointer_value)
        self.assertEqual(db.table_retired, plain_db.table_retired)
        self.assertEqual([r['step'] for r in db.rows], [r['step'] for r in plain_db.rows])
        self.assertEqual([r['step'] for r in db.rows],
                         ['started', 'creating', 'created', 'restoring', 'restored', 'validated', 'checked', 'activated'])
        unbound = ('operation_id', 'manifest_hash', 'manifest')
        self.assertEqual([{k: x for k, x in r.items() if k not in unbound} for r in db.rows],
                         [{k: x for k, x in r.items() if k not in unbound} for r in plain_db.rows])
        self.assertTrue(all(r['manifest'] == signed for r in db.rows))
        self.assertEqual(stored, canonical(signed))
        self.assertIn(json.dumps(signed, separators=(',', ':'), sort_keys=True), sql.journal(db.rows[0], 'started'))
        headed = dict(plain, schema_version=8, heading_evidence=HEADINGS,
                      heading_review=dict(HEADINGS, reviewer='coordinator', date='2099-01-02'))
        for projection in (0, 1):
            for base in (plain, headed):
                with self.subTest(projection=projection, headings='heading_evidence' in base):
                    carried = dict(base, evidence=EVIDENCE)
                    text = sql.activate(dict(db.rows[0], manifest=carried), projection)
                    journaled = json.dumps(carried, separators=(',', ':'))
                    self.assertEqual(text.count(journaled), 1)
                    self.assertEqual(text.replace(journaled, json.dumps(base, separators=(',', ':'))),
                                     sql.activate(dict(db.rows[0], manifest=base), projection))

    def test_malformed_evidence_refuses_before_the_sequence_is_bound(self):
        self.baseline()
        bound = dict(self.state.records('corpus'))
        self.signed(dict(manifest(), evidence=dict(EVIDENCE, findings_sha256='A' * 64)))
        self.assertFalse(self.run_phase())
        result = self.result()
        self.assertEqual((result['status'], result['step']), ('failed', 'manifest'))
        self.assertEqual(self.state.records('corpus'), bound)
        self.assertEqual(self.db.rows, [])
        self.phase.download.assert_not_called()
        self.signed(dict(manifest(), evidence=EVIDENCE))
        self.assertTrue(self.again())
        self.assertEqual(self.result()['status'], 'ok')
        self.phase.download.assert_called_once_with()
