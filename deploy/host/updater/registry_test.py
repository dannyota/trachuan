import hashlib
import json
from pathlib import Path
import tempfile
import unittest

from updater import deploy_registry as r
from updater.deploy_io import canonical, CommandError
from updater.deploy_verify import policy


class RegistryTest(unittest.TestCase):
    def setUp(self):
        self.work = tempfile.TemporaryDirectory()
        self.addCleanup(self.work.cleanup)
        self.directory = Path(self.work.name)
        self.policy = policy(Path(__file__).parents[1] / 'deploy-policy.json')
        self.calls = []
        self.blobs = {'manifest.json': b'{}', 'manifest.sigstore.json': b'signature', 'host-bundle.tar.gz': b'bundle'}
        self.artifact = {'artifactType': 'application/vnd.trachuan.release.v1', 'layers': [
            {'annotations': {'org.opencontainers.image.title': name}, 'digest': 'sha256:' + hashlib.sha256(data).hexdigest(),
             'size': len(data)} for name, data in self.blobs.items()]}

    def command(self, args, **kwargs):
        self.calls.append(args)
        if 'tags' in args:
            return b'{"tags":["2","9","10"]}'
        if 'resolve' in args:
            return b'sha256:' + b'a' * 64 + b'\n'
        if 'manifest' in args:
            return canonical(self.artifact)
        if 'blob' in args:
            digest = args[-1].split('@sha256:')[1]
            data = next(data for data in self.blobs.values() if hashlib.sha256(data).hexdigest() == digest)
            if args[args.index('--output') + 1] == '-':
                self.assertEqual(kwargs['stdout_limit'], len(data))
                return data
            Path(args[args.index('--output') + 1]).write_bytes(data)
            return b''
        raise AssertionError(args)

    def test_fetch_pins_resolved_digest_and_uses_only_title_allowlist(self):
        registry = r.Registry(self.policy, self.command)
        path = registry.fetch(self.policy['release_repository'], 'production', self.directory, 'release')
        self.assertEqual((path / 'host-bundle.tar.gz').read_bytes(), b'bundle')
        self.assertTrue(all('@sha256:' in args[-1] for args in self.calls if 'manifest' in args or 'blob' in args))
        self.assertFalse(any('pull' in args for args in self.calls))

    def test_newest_rollback_tag_is_numeric_not_lexical(self):
        self.assertEqual(r.Registry(self.policy, self.command).latest_rollback(), '10')

    def test_missing_rollback_repository_is_empty_but_other_registry_errors_fail(self):
        def missing(*args, **kwargs):
            raise CommandError('repository absent', reason='not_found')
        self.assertIsNone(r.Registry(self.policy, missing).latest_rollback())
        def denied(*args, **kwargs):
            raise CommandError('repository not readable anonymously', reason='denied')
        registry = r.Registry(self.policy, denied)
        self.assertIsNone(registry.latest_rollback())
        self.assertTrue(registry.rollback_unreadable)
        with self.assertRaises(CommandError):
            registry.latest_rollback(readable_before=True)
        def failed(*args, **kwargs):
            raise CommandError('registry failed')
        with self.assertRaises(CommandError):
            r.Registry(self.policy, failed).latest_rollback()

    def test_blob_fetch_uses_bounded_stdout_before_publishing_file(self):
        r.Registry(self.policy, self.command).fetch(self.policy['release_repository'], 'production', self.directory, 'release')
        blobs = [args for args in self.calls if 'blob' in args]
        self.assertTrue(all(args[args.index('--output') + 1] == '-' for args in blobs))

    def test_wrong_artifact_layers_and_unbounded_size_rejected_before_blob_fetch(self):
        for mutation in ('title', 'size', 'type', 'digest', 'duplicate'):
            artifact = json.loads(canonical(self.artifact))
            with self.subTest(mutation=mutation):
                if mutation == 'title':
                    artifact['layers'][0]['annotations']['org.opencontainers.image.title'] = '../../escape'
                elif mutation == 'size':
                    artifact['layers'][0]['size'] = 1024 ** 3
                elif mutation == 'type':
                    artifact['artifactType'] = 'other'
                elif mutation == 'duplicate':
                    artifact['layers'].append(artifact['layers'][0])
                else:
                    artifact['layers'][0]['digest'] = 'sha256:no'
                with self.assertRaises(ValueError):
                    r.layers(artifact, 'release')


if __name__ == '__main__':
    unittest.main()
