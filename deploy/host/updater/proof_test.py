import json
import unittest

from updater import activation_test as fixture
from updater import proof
from updater.deploy_io import CommandError


class ProofTest(fixture.ActivationFixture, unittest.TestCase):
    def test_proofs_reject_all_four_candidates_and_preserve_live_state(self):
        self.paths.lock.parent.mkdir(parents=True, exist_ok=True)
        self.paths.backup_lock.parent.mkdir(parents=True, exist_ok=True)
        self.paths.backup_lock.touch()
        self.paths.policy.parent.mkdir(parents=True, exist_ok=True)
        self.paths.policy.write_text(json.dumps(self.rules))
        self.paths.mode.write_text('verify\n')
        before = self.state.path.read_bytes()
        accepted = (self.old.directory / 'manifest.json').read_bytes()
        def verify(args, **kwargs):
            if 'verify-blob' in args:
                from pathlib import Path
                if Path(args[-1]).read_bytes() != accepted:
                    raise CommandError('signature rejected')
                return b''
            raise AssertionError('proof tried a host command')
        self.assertEqual(proof.run(self.paths, verify), 0)
        self.assertEqual(self.state.path.read_bytes(), before)
        rows = [json.loads(line) for line in self.paths.log.read_text().splitlines()]
        self.assertEqual([row['proof'] for row in rows],
                         ['dispatch-build', 'wrong-digest', 'replayed-rollback', 'older-sequence'])
        self.assertTrue(all(row['status'] == 'rejected' for row in rows))
        self.assertFalse(self.paths.result.exists())

    def test_proofs_refuse_activate_mode_before_commands_or_logs(self):
        self.paths.mode.parent.mkdir(parents=True, exist_ok=True)
        self.paths.mode.write_text('activate\n')
        with self.assertRaisesRegex(ValueError, 'verify mode'):
            proof.run(self.paths, lambda *args, **kwargs: self.fail('ran command'))
        self.assertFalse(self.paths.log.exists())


if __name__ == '__main__':
    unittest.main()
