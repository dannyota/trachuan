import hashlib
import importlib
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import manifest


class RegistryTest(unittest.TestCase):
    def setUp(self):
        self.assertTrue(Path(__file__).with_name('registry.py').exists(), 'registry helper missing')
        self.registry = importlib.import_module('registry')
        self.document = manifest.release('a' * 40, 'refs/tags/v1.2.3', 4,
                                         {n: 'sha256:' + 'b' * 64 for n in ('server', 'web', 'queryembed', 'postgres')},
                                         'docker.io/library/caddy@sha256:' + 'c' * 64, b'bundle')
        self.hash = hashlib.sha256(self.document).hexdigest()
        self.signature = b'synthetic signature bundle'
        self.layers = [dict(digest='sha256:' + self.hash, size=len(self.document),
                            annotations={'org.opencontainers.image.title': 'manifest.json'}),
                       dict(digest='sha256:' + hashlib.sha256(self.signature).hexdigest(), size=len(self.signature),
                            annotations={'org.opencontainers.image.title': 'manifest.sigstore.json'}),
                       dict(digest='sha256:' + 'e' * 64, size=6,
                            annotations={'org.opencontainers.image.title': 'host-bundle.tar.gz'})]
        self.artifact = dict(artifactType='application/vnd.trachuan.release.v1', layers=self.layers)
        self.commands = []

    def command(self, args):
        self.commands.append(args)
        if args[:3] == ['oras', 'repo', 'tags']:
            return b'{"tags":["v1.2.3","production"]}'
        if args[:2] == ['oras', 'resolve']:
            return ('sha256:' + 'd' * 64).encode()
        if args[:3] == ['oras', 'manifest', 'fetch']:
            self.assertTrue(args[-1].endswith('@sha256:' + 'd' * 64))
            return json.dumps(self.artifact).encode()
        if args[:3] == ['oras', 'blob', 'fetch']:
            self.assertEqual(args[3:5], ['--output', '-'])
            return self.document if args[-1].endswith(self.hash) else self.signature
        self.fail('unexpected command: ' + str(args))

    def test_discovers_by_manifest_hash_and_verifies_before_returning(self):
        with tempfile.TemporaryDirectory() as work, patch('registry.command', self.command), \
                patch('registry.verify') as verify:
            result = self.registry.find({self.hash}, Path(work))
            self.assertEqual(result[self.hash]['commit'], 'a' * 40)
            self.assertEqual((Path(work) / self.hash / 'manifest.json').read_bytes(), self.document)
            args = verify.call_args
            self.assertEqual(args.args[2:], ('images.yml', 'refs/tags/v1.2.3', 'a' * 40))
            self.assertEqual(args.kwargs, {'release': True})
        self.assertEqual(sum(args[:3] == ['oras', 'blob', 'fetch'] for args in self.commands), 2)

    def test_missing_manifest_fails_closed(self):
        with tempfile.TemporaryDirectory() as work, patch('registry.command', self.command):
            with self.assertRaisesRegex(ValueError, 'not found'):
                self.registry.find({'f' * 64}, Path(work))

    def test_wrong_blob_hash_or_signature_cannot_authorize_rollback(self):
        with tempfile.TemporaryDirectory() as work, patch('registry.command', self.command), \
                patch('registry.verify', side_effect=subprocess.CalledProcessError(1, ['cosign'])):
            with self.assertRaises(subprocess.CalledProcessError):
                self.registry.find({self.hash}, Path(work))
        self.layers[0]['size'] += 1
        with tempfile.TemporaryDirectory() as work, patch('registry.command', self.command):
            with self.assertRaises(ValueError):
                self.registry.find({self.hash}, Path(work))


if __name__ == '__main__':
    unittest.main()
