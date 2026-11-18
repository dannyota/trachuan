import importlib
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from corpus_test_support import write_fixture
from corpus_manifest import create


class CorpusPublishTest(unittest.TestCase):
    def setUp(self):
        self.module = importlib.import_module('corpus_publish')
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name)
        write_fixture(self.path)
        self.current = json.loads((self.path / 'previous-manifest.json').read_bytes()) | {'sequence': 3}

    def manifest(self, sequence):
        (self.path / 'manifest.json').write_bytes(create('a' * 40, sequence, 'vn', '20990102_1',
                                                       self.path, self.path / 'report.json'))

    def publish(self, current=None, existing=None, sign_error=None):
        with patch('corpus_publish.sign', side_effect=sign_error) as sign, \
                patch('corpus_production.production', return_value=current or self.current), \
                patch('corpus_production.existing_release', return_value=existing), \
                patch('publish.subprocess.run') as run:
            self.module.publish(self.path, 'refs/heads/main', 'a' * 40)
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
                self.module.publish(self.path, 'refs/heads/main', 'a' * 40)
        run.assert_not_called()

    def assert_refused_before_signing(self, **patches):
        with patch('corpus_publish.sign') as sign, patch('publish.subprocess.run') as run, \
                patch('corpus_production.production', **patches.get('production', dict(return_value=self.current))), \
                patch('corpus_production.existing_release', **patches.get('existing', dict(return_value=None))):
            with self.assertRaises(ValueError):
                self.module.publish(self.path, 'refs/heads/main', 'a' * 40)
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

    def test_existing_release_at_or_below_production_signs_without_comparison(self):
        self.manifest(7)
        moved = self.current | {'rel': '20990102_1', 'sequence': 6}
        sign, run = self.publish(current=moved, existing=dict(sequence=6))
        sign.assert_called_once()
        self.assertEqual(run.call_args_list[-1].args[0][:2], ['oras', 'tag'])

    def test_wrong_ref_or_commit_prevents_signing(self):
        self.manifest(7)
        with patch('corpus_publish.sign') as sign:
            for ref, commit in [('refs/heads/other', 'a' * 40), ('refs/heads/main', 'b' * 40)]:
                with self.assertRaises(ValueError):
                    self.module.publish(self.path, ref, commit)
            sign.assert_not_called()


if __name__ == '__main__':
    unittest.main()
