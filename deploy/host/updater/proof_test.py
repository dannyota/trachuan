import base64
import json
from pathlib import Path
import subprocess
import sys
import unittest

from updater import activation_test as fixture
from updater import proof
from updater.deploy_io import canonical, CommandError
from updater.verification_test import FIXTURES, statement


class ProofTest(fixture.ActivationFixture, unittest.TestCase):
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
        self.dispatch = self.rules['images']['server'] + '@sha256:' + '0' * 64

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
            manifest = json.loads((FIXTURES / 'manifest.json').read_bytes())
            self.assertEqual(args[-1], manifest['images']['server'])
            return (FIXTURES / 'server-attestation.json').read_bytes()
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

    def test_proofs_refuse_activate_mode_before_commands_or_logs(self):
        self.paths.mode.parent.mkdir(parents=True, exist_ok=True)
        self.paths.mode.write_text('activate\n')
        with self.assertRaisesRegex(ValueError, 'verify mode'):
            proof.run(self.paths, 'unused', lambda *args, **kwargs: self.fail('ran command'))
        self.assertFalse(self.paths.log.exists())


if __name__ == '__main__':
    unittest.main()
