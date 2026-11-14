import importlib
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from corpus_manifest import create
from corpus_test_support import write_fixture


class CorpusRegistryTest(unittest.TestCase):
    def setUp(self):
        self.module = importlib.import_module('corpus_registry')
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name)
        write_fixture(self.path)
        self.data = create('a' * 40, 7, 'vn', '20990102_1', self.path, self.path / 'report.json')
        self.artifact = dict(artifactType='application/vnd.trachuan.corpus-release.v1',
                             layers=[dict(digest='sha256:' + 'b' * 64, size=20,
                                          annotations={'org.opencontainers.image.title': name})
                                     for name in ('manifest.json', 'manifest.sigstore.json')])

    def production(self, artifact=None, verify_error=None):
        responses = [b'{"tags":["vn-production"]}', ('sha256:' + 'c' * 64).encode(),
                     json.dumps(artifact or self.artifact).encode()]
        with patch('corpus_registry.command', side_effect=responses), \
                patch('corpus_registry.fetch_blob', side_effect=[self.data, b'signature']), \
                patch('corpus_registry.verify', side_effect=verify_error) as verify:
            result = self.module.production('vn', self.path / 'current')
        return result, verify

    def test_current_pointer_is_verified_against_corpus_workflow_and_commit(self):
        result, verify = self.production()
        self.assertEqual(result['sequence'], 7)
        self.assertEqual(verify.call_args.args[2:], ('corpus-release.yml', 'refs/heads/main', 'a' * 40))

    def test_no_pointer_is_allowed_but_bad_type_layers_country_or_signature_fails(self):
        with patch('corpus_registry.command', return_value=b'{"tags":["vn-20990102_1"]}'):
            self.assertIsNone(self.module.production('vn', self.path))
        for artifact in [self.artifact | {'artifactType': 'wrong'},
                         self.artifact | {'layers': self.artifact['layers'] * 2}]:
            with self.assertRaises(ValueError):
                self.production(artifact)
        with self.assertRaises(ValueError):
            self.production(verify_error=ValueError('signature'))
        self.data = self.data.replace(b'"country":"vn"', b'"country":"us"')
        with self.assertRaises(ValueError):
            self.production()


if __name__ == '__main__':
    unittest.main()
