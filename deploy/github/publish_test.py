import importlib
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import manifest


class PublishTest(unittest.TestCase):
    def setUp(self):
        self.assertTrue(Path(__file__).with_name('publish.py').exists(), 'publisher missing')
        self.publisher = importlib.import_module('publish')

    def test_release_publishes_three_layers_then_tags_production(self):
        data = manifest.release('a' * 40, 'refs/tags/v1.2.3', 4,
                                {n: 'sha256:' + 'b' * 64 for n in ('server', 'web', 'queryembed', 'postgres')},
                                'docker.io/library/caddy@sha256:' + 'c' * 64, b'bundle')
        with tempfile.TemporaryDirectory() as work, patch('publish.sign') as sign, \
                patch('publish.subprocess.run') as run:
            path = Path(work)
            (path / 'manifest.json').write_bytes(data)
            self.publisher.release(path, 'refs/tags/v1.2.3', 'a' * 40)
            self.assertEqual(sign.call_count, 1)
            push = run.call_args_list[0].args[0]
            self.assertEqual(push[:3], ['oras', 'push', 'ghcr.io/dannyota/trachuan-release:v1.2.3'])
            self.assertIn('application/vnd.trachuan.release.v1', push)
            self.assertIn('host-bundle.tar.gz:application/gzip', push)
            self.assertEqual(run.call_args_list[1].args[0],
                             ['oras', 'tag', 'ghcr.io/dannyota/trachuan-release:v1.2.3', 'production'])

    def test_failed_verification_prevents_publication(self):
        data = manifest.release('a' * 40, 'refs/tags/v1.2.3', 4,
                                {n: 'sha256:' + 'b' * 64 for n in ('server', 'web', 'queryembed', 'postgres')},
                                'docker.io/library/caddy@sha256:' + 'c' * 64, b'bundle')
        with tempfile.TemporaryDirectory() as work, patch('publish.sign', side_effect=ValueError('signature')), \
                patch('publish.subprocess.run') as run:
            path = Path(work)
            (path / 'manifest.json').write_bytes(data)
            with self.assertRaises(ValueError):
                self.publisher.release(path, 'refs/tags/v1.2.3', 'a' * 40)
            run.assert_not_called()

    def test_rollback_publishes_only_verified_authorization(self):
        current = json.loads(manifest.release('a' * 40, 'refs/tags/v1.2.3', 4,
                                             {n: 'sha256:' + 'b' * 64 for n in ('server', 'web', 'queryembed', 'postgres')},
                                             'docker.io/library/caddy@sha256:' + 'c' * 64, b'bundle'))
        target = current | {'sequence': 3}
        with tempfile.TemporaryDirectory() as work, \
                patch('publish.find', return_value={'a' * 64: current, 'b' * 64: target}), \
                patch('publish.sign') as sign, patch('publish.subprocess.run') as run:
            path = Path(work)
            self.publisher.rollback(path, 'a' * 64, 'b' * 64, 9, 'refs/heads/main', 'c' * 40)
            self.assertEqual((path / 'manifest.json').read_bytes(),
                             ('{"current":"' + 'a' * 64 + '","kind":"trachuan.rollback",'
                              '"sequence":9,"target":"' + 'b' * 64 + '","version":1}').encode())
            self.assertEqual(sign.call_args.args[2:], ('rollback.yml', 'refs/heads/main', 'c' * 40))
            command = run.call_args.args[0]
            self.assertEqual(command[:3], ['oras', 'push', 'ghcr.io/dannyota/trachuan-rollback:9'])
            self.assertIn('application/vnd.trachuan.rollback.v1', command)
            self.assertFalse(any(arg.startswith('host-bundle') for arg in command))

    def test_rollback_rejects_wrong_ref_or_hash_before_registry_access(self):
        with patch('publish.find') as find:
            for current, ref in [('a' * 64, 'refs/tags/v1.2.3'), ('x' * 64, 'refs/heads/main')]:
                with self.subTest(current=current, ref=ref), self.assertRaises(ValueError):
                    self.publisher.rollback(Path('/unused'), current, 'b' * 64, 9, ref, 'c' * 40)
            find.assert_not_called()


if __name__ == '__main__':
    unittest.main()
