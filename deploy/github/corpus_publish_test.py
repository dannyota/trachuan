import importlib
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

    def manifest(self, sequence):
        (self.path / 'manifest.json').write_bytes(create('a' * 40, sequence, 'vn', '20990102_1',
                                                       self.path, self.path / 'report.json'))

    def test_signs_and_publishes_two_layers_then_moves_country_pointer(self):
        self.manifest(7)
        with patch('corpus_publish.sign') as sign, patch('corpus_publish.production', return_value=None), \
                patch('publish.subprocess.run') as run:
            self.module.publish(self.path, 'refs/heads/main', 'a' * 40)
        self.assertEqual(sign.call_args.args[2:], ('corpus-release.yml', 'refs/heads/main', 'a' * 40))
        push = run.call_args_list[0].args[0]
        self.assertEqual(push[:3], ['oras', 'push', 'ghcr.io/dannyota/trachuan-corpus-release:vn-20990102_1'])
        self.assertIn('application/vnd.trachuan.corpus-release.v1', push)
        self.assertIn('manifest.json:application/json', push)
        self.assertFalse(any(arg.startswith('host-bundle') for arg in push))
        self.assertEqual(run.call_args_list[1].args[0],
                         ['oras', 'tag', 'ghcr.io/dannyota/trachuan-corpus-release:vn-20990102_1', 'vn-production'])

    def test_only_higher_sequence_moves_pointer(self):
        pointer = {'sequence': 3}
        for sequence, expected in ((5, 5), (4, 5), (5, 5)):
            self.manifest(sequence)
            with patch('corpus_publish.sign'), patch('corpus_publish.production', return_value=pointer.copy()), \
                    patch('publish.subprocess.run') as run:
                self.module.publish(self.path, 'refs/heads/main', 'a' * 40)
            if any(call.args[0][:2] == ['oras', 'tag'] for call in run.call_args_list):
                pointer['sequence'] = sequence
            self.assertEqual(pointer['sequence'], expected)

    def test_unverified_signature_or_current_pointer_never_moves_tag(self):
        self.manifest(7)
        with patch('corpus_publish.sign', side_effect=ValueError('signature')), \
                patch('publish.subprocess.run') as run:
            with self.assertRaises(ValueError):
                self.module.publish(self.path, 'refs/heads/main', 'a' * 40)
            run.assert_not_called()
        with patch('corpus_publish.sign'), patch('corpus_publish.production', side_effect=ValueError('signature')), \
                patch('publish.subprocess.run') as run:
            with self.assertRaises(ValueError):
                self.module.publish(self.path, 'refs/heads/main', 'a' * 40)
            self.assertFalse(any(call.args[0][:2] == ['oras', 'tag'] for call in run.call_args_list))

    def test_wrong_ref_or_commit_prevents_signing(self):
        self.manifest(7)
        with patch('corpus_publish.sign') as sign:
            for ref, commit in [('refs/heads/other', 'a' * 40), ('refs/heads/main', 'b' * 40)]:
                with self.assertRaises(ValueError):
                    self.module.publish(self.path, ref, commit)
            sign.assert_not_called()


if __name__ == '__main__':
    unittest.main()
