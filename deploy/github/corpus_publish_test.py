import hashlib
import importlib
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from corpus_test_support import save_fixture, save_older_shape, write_fixture
from corpus_manifest import create


class CorpusPublishTest(unittest.TestCase):
    def setUp(self):
        self.module = importlib.import_module('corpus_publish')
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name)
        self.row, _ = write_fixture(self.path)
        self.current = json.loads((self.path / 'previous-manifest.json').read_bytes()) | {'sequence': 3}

    def report_sha256(self):
        return hashlib.sha256((self.path / 'report.json').read_bytes()).hexdigest()

    def release_sha256(self):
        return hashlib.sha256((self.path / 'release.json').read_bytes()).hexdigest()

    def manifest(self, sequence):
        (self.path / 'manifest.json').write_bytes(create('a' * 40, sequence, 'vn', '20990102_1',
                                                       self.path, self.path / 'report.json'))

    def publish(self, current=None, existing=None, sign_error=None, report_sha256=None, release_sha256=None):
        with patch('corpus_publish.sign', side_effect=sign_error) as sign, \
                patch('corpus_production.production', return_value=current or self.current), \
                patch('corpus_production.existing_release', return_value=existing), \
                patch('publish.subprocess.run') as run:
            self.module.publish(self.report_sha256() if report_sha256 is None else report_sha256,
                                self.release_sha256() if release_sha256 is None else release_sha256,
                                self.path, 'refs/heads/main', 'a' * 40)
        return sign, run

    def test_signs_and_publishes_two_layers_then_moves_country_pointer(self):
        self.manifest(7)
        sign, run = self.publish()
        self.assertEqual(sign.call_args.args[2:], ('corpus-release.yml', 'refs/heads/main', 'a' * 40))
        push = run.call_args_list[0].args[0]
        self.assertEqual(push[:3], ['oras', 'push', 'ghcr.io/dannyota/trachuan-corpus-release:vn-20990102_1'])
        self.assertIn('application/vnd.trachuan.corpus-release.v1', push)
        self.assertIn('manifest.json:application/json', push)
        self.assertFalse(any(arg.startswith('host-bundle') for arg in push))
        self.assertEqual(run.call_args_list[1].args[0],
                         ['oras', 'tag', 'ghcr.io/dannyota/trachuan-corpus-release:vn-20990102_1', 'vn-production'])

    def test_only_higher_sequence_moves_pointer(self):
        pointer = self.current
        signed = None
        for sequence, expected in ((5, 5), (4, 5), (5, 5)):
            self.manifest(sequence)
            _, run = self.publish(current=pointer, existing=signed)
            if any(call.args[0][:2] == ['oras', 'tag'] for call in run.call_args_list):
                signed = json.loads((self.path / 'manifest.json').read_bytes())
                pointer = signed
            self.assertEqual(pointer['sequence'], expected)

    def test_failed_signature_never_pushes(self):
        self.manifest(7)
        with self.assertRaises(ValueError), patch('publish.subprocess.run') as run:
            with patch('corpus_publish.sign', side_effect=ValueError('signature')), \
                    patch('corpus_production.production', return_value=self.current), \
                    patch('corpus_production.existing_release', return_value=None):
                self.module.publish(self.report_sha256(), self.release_sha256(), self.path, 'refs/heads/main', 'a' * 40)
        run.assert_not_called()

    def assert_refused_before_signing(self, report_sha256=None, release_sha256=None, **patches):
        with patch('corpus_publish.sign') as sign, patch('publish.subprocess.run') as run, \
                patch('corpus_production.production', **patches.get('production', dict(return_value=self.current))), \
                patch('corpus_production.existing_release', **patches.get('existing', dict(return_value=None))):
            with self.assertRaises(ValueError):
                self.module.publish(self.report_sha256() if report_sha256 is None else report_sha256,
                                    self.release_sha256() if release_sha256 is None else release_sha256,
                                    self.path, 'refs/heads/main', 'a' * 40)
        sign.assert_not_called()
        run.assert_not_called()

    def test_refused_comparison_leaves_no_signature_or_tag(self):
        self.manifest(7)
        self.assert_refused_before_signing(production=dict(return_value=self.current | {'rel': '20990101_2'}))

    def test_unverified_or_missing_production_never_signs(self):
        self.manifest(7)
        self.assert_refused_before_signing(production=dict(side_effect=ValueError('signature')))
        self.assert_refused_before_signing(production=dict(return_value=None))

    def test_unverified_existing_release_never_signs(self):
        self.manifest(7)
        self.assert_refused_before_signing(existing=dict(side_effect=ValueError('signature')))

    def test_report_file_must_be_the_signed_manifest_report(self):
        self.manifest(7)
        path = self.path / 'report.json'
        path.write_bytes(path.read_bytes() + b' ')
        self.assert_refused_before_signing()

    def test_report_must_have_the_dispatched_hash(self):
        self.manifest(7)
        dispatched = self.report_sha256()
        for value in ('0' * 64, dispatched.upper(), dispatched[:-1], dispatched + '\n', ''):
            with self.subTest(value=value):
                self.assert_refused_before_signing(report_sha256=value)

    def test_release_must_have_the_dispatched_hash(self):
        self.manifest(7)
        dispatched = self.release_sha256()
        for value in ('0' * 64, dispatched.upper(), dispatched[:-1], dispatched + '\n', ' '):
            with self.subTest(value=value):
                self.assert_refused_before_signing(release_sha256=value)

    def test_empty_release_hash_signs_only_an_already_signed_release(self):
        self.manifest(7)
        self.assert_refused_before_signing(release_sha256='')
        signed = json.loads((self.path / 'manifest.json').read_bytes()) | {'sequence': 2}
        sign, _ = self.publish(existing=signed, release_sha256='')
        sign.assert_called_once()

    def test_resigned_release_cannot_gain_a_field_its_signed_manifest_lacks(self):
        self.manifest(7)
        signed = json.loads((self.path / 'manifest.json').read_bytes()) | {'sequence': 2}
        signed.pop('evidence')
        self.assert_refused_before_signing(release_sha256='', existing=dict(return_value=signed))

    def test_consistent_swapped_set_is_refused_by_the_dispatched_hash(self):
        dispatched = self.report_sha256()
        report = json.loads((self.path / 'report.json').read_bytes())
        save_fixture(self.path, self.row, dict(reversed(report.items())))
        self.assertNotEqual(self.report_sha256(), dispatched)
        self.manifest(7)
        self.publish()
        self.assert_refused_before_signing(report_sha256=dispatched)

    def test_existing_release_at_or_below_production_signs_without_comparison(self):
        self.manifest(7)
        moved = self.current | {'rel': '20990102_1', 'sequence': 6}
        signed = json.loads((self.path / 'manifest.json').read_bytes()) | {'sequence': 6}
        sign, run = self.publish(current=moved, existing=signed)
        sign.assert_called_once()
        self.assertEqual(run.call_args_list[-1].args[0][:2], ['oras', 'tag'])

    @patch.dict('corpus_rules.LEGACY_CUTOFF', {'vn': '20990102_2'})
    def test_back_dated_release_without_a_signed_manifest_never_signs(self):
        save_older_shape(self.path, self.row)
        self.manifest(7)
        self.assert_refused_before_signing()

    @patch.dict('corpus_rules.LEGACY_CUTOFF', {'vn': '20990102_2'})
    def test_older_release_with_a_signed_manifest_signs_again(self):
        save_older_shape(self.path, self.row)
        self.manifest(7)
        signed = json.loads((self.path / 'manifest.json').read_bytes()) | {'sequence': 2}
        sign, _ = self.publish(existing=signed)
        sign.assert_called_once()

    @patch.dict('corpus_rules.LEGACY_CUTOFF', {'vn': '20990102_1'})
    def test_cutoff_release_signs_for_the_first_time_without_a_decision(self):
        save_older_shape(self.path, self.row)
        self.manifest(7)
        sign, _ = self.publish()
        sign.assert_called_once()

    def test_changed_signed_field_never_signs(self):
        self.manifest(7)
        signed = json.loads((self.path / 'manifest.json').read_bytes()) | {'sequence': 2, 'previous': '20990101_2'}
        self.assert_refused_before_signing(existing=dict(return_value=signed))

    def test_wrong_ref_or_commit_prevents_signing(self):
        self.manifest(7)
        with patch('corpus_publish.sign') as sign:
            for ref, commit in [('refs/heads/other', 'a' * 40), ('refs/heads/main', 'b' * 40)]:
                with self.assertRaises(ValueError):
                    self.module.publish(self.report_sha256(), self.release_sha256(), self.path, ref, commit)
            sign.assert_not_called()


if __name__ == '__main__':
    unittest.main()
