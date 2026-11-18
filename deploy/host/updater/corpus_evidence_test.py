import json
from pathlib import Path
import re
import unittest
from unittest import mock

from updater import activation_test as fixture
from updater import deploy_corpus as c
from updater import deploy_corpus_sql as sql
from updater import deploy_corpus_verify as v
from updater.corpus_activation_test import Crash, Database
from updater.corpus_verify_test import FIXTURE, description, manifest
from updater.deploy_io import canonical
from updater.deploy_verify import policy


EVIDENCE = dict(findings_sha256='0123456789abcdef' * 4, identity_sha256='fedcba9876543210' * 4,
                changes_sha256='00ff' * 16)
HEADINGS = dict(projection=1, scan_sha256='a' * 64, exclusions_sha256='b' * 64)


class CorpusEvidenceTest(unittest.TestCase):
    def setUp(self):
        self.rules = policy(Path(__file__).parents[1] / 'deploy-policy.json')
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
                        ' ' + '1' * 63, '', None, 1, True, ['1' * 64], {'sha256': '1' * 64}):
                cases.append((dict(EVIDENCE, **{field: bad}), 'invalid evidence ' + field))
        for value, message in cases:
            with self.subTest(value=value), self.assertRaisesRegex(ValueError, '^' + re.escape(message) + '$'):
                self.verified(dict(self.m, evidence=value))


class EvidenceActivationTest(fixture.ActivationFixture, unittest.TestCase):
    def activate(self, m):
        fixture.ActivationFixture.setUp(self)
        directory = self.paths.state / 'corpus-fixture'
        directory.mkdir()
        directory.joinpath('manifest.json').write_bytes(canonical(m))
        directory.joinpath('manifest.sigstore.json').write_bytes(b'{}')
        db = Database()
        db.check_extensions = mock.Mock()
        db.describe = mock.Mock(return_value=description(m))
        db.after = 'restored'
        registry = mock.Mock()
        registry.fetch.return_value = directory
        phase = c.Corpus(self.state, self.store, self.host, self.rules, registry, database=db,
                         prepare=mock.Mock(return_value=True), clock=lambda: 100000)
        phase.resources = mock.Mock()
        phase.download = mock.Mock(return_value=Path('/synthetic/dump'))
        with self.assertRaises(Crash):
            phase.run(self.paths.state / 'run', 'activate')
        self.assertTrue(phase.run(self.paths.state / 'run', 'activate'))
        phase.download.assert_called_once_with()
        records = {key: record['status'] for key, record in self.state.records('corpus').items()}
        return json.loads(phase.result.path.read_bytes()), db, records

    def test_evidence_survives_journal_and_recovery_and_changes_no_activation_step(self):
        plain = manifest()
        signed = dict(plain, evidence=EVIDENCE)
        expected, plain_db, plain_records = self.activate(plain)
        result, db, records = self.activate(signed)
        self.assertEqual(result['status'], 'ok')
        self.assertEqual(dict(result, manifest_hash=None), dict(expected, manifest_hash=None))
        self.assertEqual(records, plain_records)
        self.assertEqual(db.active, plain_db.active)
        self.assertEqual(db.retired, plain_db.retired)
        self.assertEqual([r['step'] for r in db.rows], [r['step'] for r in plain_db.rows])
        unbound = ('operation_id', 'manifest_hash', 'manifest')
        self.assertEqual([{k: x for k, x in r.items() if k not in unbound} for r in db.rows],
                         [{k: x for k, x in r.items() if k not in unbound} for r in plain_db.rows])
        self.assertTrue(all(r['manifest'] == signed for r in db.rows))
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
