import copy
import json
from pathlib import Path
import tempfile
import unittest

from updater import deploy_state as s


class StateTest(unittest.TestCase):
    def setUp(self):
        self.work = tempfile.TemporaryDirectory()
        self.addCleanup(self.work.cleanup)
        self.path = Path(self.work.name) / 'state.json'
        self.state = s.State.bootstrap(self.path, 'a' * 64, 14, 7, 'b' * 64)

    def test_existing_bootstrap_state_keeps_main_validator_shape(self):
        original = dict(version=1, active='a' * 64,
                        accepted={'image': {'14': {'hash': 'a' * 64, 'status': 'active'}},
                                  'rollback': {}, 'corpus': {}},
                        retained=[], failed=[], consumed=[], schemas={'a' * 64: 7},
                        installed_bundle='b' * 64, journal=None)
        self.assertNotIn('secrets', original)
        raw = json.dumps(original).encode() + b'\n'
        self.path.write_bytes(raw)
        state = s.State(self.path)
        main_fields = {'version', 'active', 'accepted', 'retained', 'failed', 'consumed',
                       'schemas', 'installed_bundle', 'journal'}
        self.assertEqual(set(state.data), main_fields)
        self.assertEqual(state.data, original)
        self.assertEqual(self.path.read_bytes(), raw)
        state.save()
        self.assertEqual(s.State(self.path).data, state.data)
        self.assertEqual(set(json.loads(self.path.read_bytes())), main_fields)
        self.assertFalse(self.path.with_name('secrets-state.json').exists())
        self.assertEqual(self.path.stat().st_mode & 0o777, 0o600)

    def test_missing_or_corrupt_state_never_resets_floor(self):
        self.path.unlink()
        with self.assertRaisesRegex(ValueError, 'bootstrap'):
            s.State(self.path)
        for data in ({}, {'version': 1}, {'version': 2}):
            self.path.write_text(json.dumps(data))
            with self.assertRaisesRegex(ValueError, 'bootstrap'):
                s.State(self.path)

    def test_seen_and_failed_hashes_are_noops(self):
        self.assertEqual(self.state.decision('image', 'a' * 64, 14), 'noop')
        self.state.accept('image', 'c' * 64, 15)
        self.state.fail('c' * 64)
        self.assertEqual(s.State(self.path).decision('image', 'c' * 64, 15), 'noop')

    def test_older_sequence_and_reused_sequence_are_rejected(self):
        for sequence, reason in ((13, 'older'), (14, 'reuse')):
            with self.subTest(sequence=sequence), self.assertRaisesRegex(ValueError, reason):
                self.state.accept('image', 'c' * 64, sequence)
        self.assertEqual(s.State(self.path).data, self.state.data)

    def test_pending_continues_after_restart_but_not_after_superseded(self):
        self.state.accept('image', 'c' * 64, 15)
        self.state.accept('image', 'd' * 64, 16)
        self.assertEqual(s.State(self.path).decision('image', 'c' * 64, 15), 'pending')
        self.state.activate('d' * 64, 7)
        restarted = s.State(self.path)
        self.assertEqual(restarted.decision('image', 'c' * 64, 15), 'noop')
        self.assertIn('c' * 64, restarted.data['failed'])
        self.assertEqual(restarted.decision('image', 'a' * 64, 14), 'noop')

    def test_rollback_requires_current_retained_target_same_schema_and_consumes_first(self):
        self.state.accept('image', 'c' * 64, 15)
        self.state.activate('c' * 64, 7)
        auth = {'current': 'c' * 64, 'target': 'a' * 64, 'sequence': 1}
        baseline = copy.deepcopy(self.state.data)
        for changes, schema in (({'current': 'b' * 64}, 7), ({'target': 'b' * 64}, 7), ({}, 8)):
            with self.subTest(changes=changes, schema=schema), self.assertRaises(ValueError):
                self.state.authorize('e' * 64, dict(auth, **changes), schema)
            self.assertEqual(self.state.data, baseline)
        self.state.data['journal'] = dict(target='a' * 64, previous='c' * 64, baseline=7,
            restart_postgres=False, status='started', step='started', authorization='e' * 64)
        self.state.authorize('e' * 64, auth, 7)
        reloaded = s.State(self.path)
        self.assertIn('e' * 64, reloaded.data['consumed'])
        with self.assertRaisesRegex(ValueError, 'consumed'):
            reloaded.authorize('e' * 64, auth, 7)
        with self.assertRaisesRegex(ValueError, 'reuse'):
            reloaded.authorize('f' * 64, auth, 7)
        with self.assertRaisesRegex(ValueError, 'older'):
            reloaded.authorize('f' * 64, dict(auth, sequence=0), 7)

    def test_state_write_is_private_and_survives_reload(self):
        self.state.accept('image', 'c' * 64, 15)
        self.assertEqual(self.path.stat().st_mode & 0o777, 0o600)
        self.assertEqual(s.State(self.path).data, self.state.data)

    def test_applied_rollback_supersedes_older_pending_authorizations(self):
        self.state.accept('image', 'c' * 64, 15)
        self.state.activate('c' * 64, 7)
        self.state.accept('rollback', 'd' * 64, 1)
        auth = {'current': 'c' * 64, 'target': 'a' * 64, 'sequence': 2}
        self.state.data['journal'] = dict(target='a' * 64, previous='c' * 64, baseline=7,
            restart_postgres=False, status='started', step='started', authorization='e' * 64)
        self.state.authorize('e' * 64, auth, 7)
        self.state.activate('a' * 64, 7, authorization='e' * 64)
        self.assertEqual(s.State(self.path).data['accepted']['rollback']['1']['status'], 'failed')
        with self.assertRaises(ValueError):
            self.state.check_rollback('d' * 64, dict(auth, sequence=1, current='a' * 64, target='c' * 64), 7)


if __name__ == '__main__':
    unittest.main()
