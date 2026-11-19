import copy
import hashlib
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from corpus_comparison_test import reports
from corpus_test_support import recipe_fixture, retrieval_recipe


class CorpusPreviousTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name)
        self.previous, self.report = reports()
        self.data = json.dumps(self.previous).encode()
        self.digest = hashlib.sha256(self.data).hexdigest()
        self.report['previous_diff']['previous']['report_sha256'] = self.digest
        self.manifest = dict(country='vn', rel='20990101_1', report={'sha256': self.digest}, recipes={'retrieval': self.previous['retrieval_recipe']})
        self.calls = []

    def client(self, args):
        self.calls.append(args)
        if args[0] == 'head-object':
            return dict(ContentLength=len(self.data), ETag='opaque')
        Path(args[-1]).write_bytes(self.data)
        return dict(ETag='opaque', ContentRange=f'bytes 0-{len(self.data) - 1}/{len(self.data)}')

    def verify(self, report=None, manifest=None, client=None, run=None, sidecars=None):
        from corpus_previous import verified_previous
        with patch('corpus_registry.release', return_value=manifest or self.manifest):
            candidate = report or self.report
            row = recipe_fixture(candidate['retrieval_recipe'])[0]
            if run is not None:
                row['run'] = run
            return verified_previous(candidate, self.path, client or self.client, release_row=row, sidecars=sidecars)

    def test_decision_release_compares_its_predecessor_with_the_signed_evidence(self):
        run = dict(eval_report_sha256='e' * 64)
        sidecars = dict(predecessor=dict(rel='20990101_1', findings_sha256='none', identity_sha256='none'))
        self.assertEqual(self.verify(run=run, sidecars=sidecars), self.previous)
        with self.assertRaisesRegex(ValueError, 'evidence checked'):
            self.verify(run=run)
        evidence = dict(findings_sha256='a' * 64, identity_sha256='b' * 64, changes_sha256='c' * 64)
        with self.assertRaisesRegex(ValueError, 'previous signed manifest'):
            self.verify(manifest=self.manifest | dict(evidence=evidence), run=run, sidecars=sidecars)
        named = dict(predecessor=dict(rel='20990101_1', findings_sha256='a' * 64, identity_sha256='b' * 64))
        self.assertEqual(self.verify(manifest=self.manifest | dict(evidence=evidence), run=run, sidecars=named), self.previous)

    def test_older_shape_skips_the_evidence_comparison(self):
        self.assertEqual(self.verify(run=dict(complete=True)), self.previous)

    def test_downloads_bounded_previous_report_and_checks_both_hashes(self):
        self.assertEqual(self.verify(), self.previous)
        self.assertEqual(self.calls[1][self.calls[1].index('--key') + 1], 'vn/20990101_1/report.json')
        self.assertIn('--if-match', self.calls[1])
        self.assertIn('--range', self.calls[1])

    def test_refuses_mismatch_against_either_hash_or_identity(self):
        report = copy.deepcopy(self.report)
        report['previous_diff']['previous']['report_sha256'] = '0' * 64
        with self.assertRaises(ValueError):
            self.verify(report=report)
        for manifest in (self.manifest | {'report': {'sha256': '0' * 64}},
                         self.manifest | {'rel': '20990101_2'}):
            with self.subTest(manifest=manifest), self.assertRaises(ValueError):
                self.verify(manifest=manifest)

    def test_refuses_missing_file_signature_failure_and_oversize(self):
        from corpus_previous import verified_previous
        for error in (FileNotFoundError('missing report'), ValueError('signature')):
            with patch('corpus_registry.release', side_effect=error), self.assertRaises(type(error)):
                verified_previous(self.report, self.path, self.client)
        with self.assertRaises(ValueError):
            self.verify(client=lambda args: dict(ContentLength=16 * 1024**2 + 1, ETag='opaque'))

    def test_old_report_uses_verified_manifest_recipe_for_calibration_comparison(self):
        self.previous.pop('retrieval_recipe', None)
        previous_recipe = retrieval_recipe()
        previous_recipe['abstention']['calibration_sha256'] = 'old'
        self.manifest['recipes'] = {'retrieval': previous_recipe}
        self.report['retrieval_recipe']['abstention']['calibration_sha256'] = 'new'
        self.report['recipes']['retrieval'] = recipe_fixture(self.report['retrieval_recipe'])[1]['retrieval']
        self.data = json.dumps(self.previous).encode()
        digest = hashlib.sha256(self.data).hexdigest()
        self.report['previous_diff']['previous']['report_sha256'] = digest
        self.manifest['report']['sha256'] = digest
        self.assertEqual(self.verify()['retrieval_recipe'], previous_recipe)

    def test_version_three_previous_needs_its_retrieval_recipe(self):
        self.previous.pop('retrieval_recipe')
        self.previous.update(version=3, scorer='agent-eval/6')
        self.data = json.dumps(self.previous).encode()
        digest = hashlib.sha256(self.data).hexdigest()
        self.report['previous_diff']['previous']['report_sha256'] = digest
        self.manifest['report']['sha256'] = digest
        with self.assertRaisesRegex(ValueError, 'previous version 3 retrieval recipe missing'):
            self.verify()

    def test_previous_report_recipe_cannot_disagree_with_signed_manifest(self):
        self.manifest['recipes']['retrieval'] = copy.deepcopy(self.previous['retrieval_recipe'])
        self.manifest['recipes']['retrieval']['non_current_tail'] = 3
        with self.assertRaises(ValueError):
            self.verify()


if __name__ == '__main__':
    unittest.main()
