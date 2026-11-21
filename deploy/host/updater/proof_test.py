import base64
import hashlib
import json
from pathlib import Path
import re
import subprocess
import sys
import unittest

from updater import activation_test as fixture
from updater import proof
from updater.deploy_io import canonical, CommandError
from updater.verification_test import FIXTURES, statement


class ProofTest(fixture.ActivationFixture, unittest.TestCase):
    def test_release_b_proof_fixture_passes_shipped_policy(self):
        from updater.deploy_verify import policy, release
        rules = policy(Path(__file__).parents[1] / 'deploy-policy.json')
        raw = (FIXTURES / 'proof-manifest.json').read_bytes()
        self.assertEqual(hashlib.sha256(raw).hexdigest(),
                         '0480a211c61d9f7da7cabd4f6eed3dca9d67b25051fdc9ed83004826132c33da')
        recorded = release(raw, rules)
        self.assertEqual(recorded['repository'], 'dntmakers/trachuan')
        self.assertEqual(recorded['sequence'], 66)
        self.assertNotEqual(recorded['images']['server'].split('@')[1], recorded['images']['web'].split('@')[1])

    def test_state_snapshot_detects_deployment_hold_changes(self):
        before = proof.snapshot(self.paths)
        self.paths.hold.write_bytes(b'held\n')
        self.assertNotEqual(proof.snapshot(self.paths), before)

    def test_proof_entrypoint_accepts_dispatch_image_argument(self):
        result = subprocess.run([sys.executable, '-I', str(Path(proof.__file__)), '--help'],
                                capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('dispatch_image', result.stdout)

    def setup_proof(self):
        self.paths.lock.parent.mkdir(parents=True, exist_ok=True)
        self.paths.backup_lock.parent.mkdir(parents=True, exist_ok=True)
        self.paths.backup_lock.touch()
        self.paths.policy.parent.mkdir(parents=True, exist_ok=True)
        self.paths.policy.write_text(json.dumps(self.rules))
        self.paths.mode.write_text('verify\n')
        self.dispatch = self.rules['images']['dannyota/trachuan']['server'] + '@sha256:' + '0' * 64

    def attestation(self, args, **kwargs):
        if 'verify-blob' in args:
            return b''
        if 'verify-attestation' not in args:
            raise AssertionError('proof tried a host command')
        if args[-1] == self.dispatch:
            identity = args[args.index('--certificate-identity-regexp') + 1]
            if '@refs/heads/main$' not in identity:
                raise CommandError('identity rejected', reason='identity')
            manifest = {'ref': 'refs/heads/main', 'commit': 'a' * 40}
        else:
            manifest = json.loads((FIXTURES / 'proof-manifest.json').read_bytes())
            self.assertEqual(args[-1], manifest['images']['server'])
            payload = statement(manifest, args[-1])
            definition = payload['predicate']['buildDefinition']
            definition['externalParameters']['workflow']['repository'] = 'https://github.com/' + manifest['repository']
            definition['resolvedDependencies'][0]['uri'] = 'git+https://github.com/' + manifest['repository'] + '@' + manifest['ref']
            return canonical({'payload': base64.b64encode(canonical(payload)).decode()})
        return canonical({'payload': base64.b64encode(canonical(statement(manifest, args[-1]))).decode()})

    def test_proofs_reject_all_four_candidates_and_preserve_live_state(self):
        self.setup_proof()
        before = self.state.path.read_bytes()
        self.assertEqual(proof.run(self.paths, self.dispatch, self.attestation), 0)
        self.assertEqual(self.state.path.read_bytes(), before)
        rows = [json.loads(line) for line in self.paths.log.read_text().splitlines()]
        self.assertEqual([row['proof'] for row in rows],
                         ['dispatch-build', 'wrong-digest', 'replayed-rollback', 'older-sequence'])
        self.assertTrue(all(row['status'] == 'rejected' for row in rows))
        self.assertEqual([row['stage'] for row in rows],
                         ['identity', 'subject', 'rollback-consumption', 'sequence-floor'])
        self.assertFalse(self.paths.result.exists())

    def test_proof_refuses_network_failure_in_place_of_identity_rejection(self):
        self.setup_proof()
        def unavailable(args, **kwargs):
            if 'verify-attestation' in args and args[-1] == self.dispatch:
                raise CommandError('registry unavailable')
            return self.attestation(args, **kwargs)
        with self.assertRaises(CommandError):
            proof.run(self.paths, self.dispatch, unavailable)
        self.assertFalse(self.paths.log.exists())

    def test_dispatch_identity_follows_the_repository_whose_images_hold_the_dispatch(self):
        for repository in self.rules['repositories']:
            calls = []

            def record(args, **kwargs):
                calls.append(args)
                raise CommandError('stop after flags')

            image = self.rules['images'][repository]['web'] + '@sha256:' + '0' * 64
            with self.subTest(repository=repository), self.assertRaises(CommandError):
                proof.dispatch_manifest(image, self.rules, record)
            args = calls[0]
            self.assertEqual(args[args.index('--certificate-github-workflow-repository') + 1], repository)
            self.assertEqual(args[args.index('--certificate-identity-regexp') + 1],
                             '^' + re.escape('https://github.com/' + repository + '/.github/workflows/images.yml')
                             + '@refs/heads/main$')
        for image in ('ghcr.io/attacker/trachuan-web@sha256:' + '0' * 64, self.rules['images']['dannyota/trachuan']['web']):
            with self.subTest(image=image), self.assertRaisesRegex(ValueError, 'invalid dispatch image'):
                proof.dispatch_manifest(image, self.rules, lambda *args, **kwargs: self.fail('ran command'))

    def test_proofs_refuse_activate_mode_before_commands_or_logs(self):
        self.paths.mode.parent.mkdir(parents=True, exist_ok=True)
        self.paths.mode.write_text('activate\n')
        with self.assertRaisesRegex(ValueError, 'verify mode'):
            proof.run(self.paths, 'unused', lambda *args, **kwargs: self.fail('ran command'))
        self.assertFalse(self.paths.log.exists())


if __name__ == '__main__':
    unittest.main()
