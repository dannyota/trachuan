import fcntl
import hashlib
import json
from pathlib import Path
import unittest
from unittest import mock

from updater import deploy_run as r
from updater import activation_test as fixture
from updater.activation_test import Crash
from updater.deploy_io import canonical
from updater.deploy_io import CommandError
from updater.deploy_state import State
from updater.deploy_host import pins


class RunTest(fixture.ActivationFixture, unittest.TestCase):
    def setUp(self):
        super().setUp()
        self.paths.lock.parent.mkdir(parents=True, exist_ok=True)
        self.paths.backup_lock.parent.mkdir(parents=True, exist_ok=True)
        self.paths.backup_lock.touch()
        self.paths.mode.parent.mkdir(parents=True, exist_ok=True)
        self.paths.mode.write_text('verify\n')
        self.paths.policy.write_bytes(canonical(self.rules))
        self.registry = mock.Mock()
        self.registry.fetch.return_value = self.new.directory
        self.registry.latest_rollback.return_value = None
        self.app = r.Updater(self.paths, self.runner, user_prefix=[])
        self.app.registry = self.registry
        self.verify_images = mock.patch('updater.deploy_run.Verifier.images')
        self.images = self.verify_images.start()
        self.addCleanup(self.verify_images.stop)

    def test_verify_keeps_pending_without_activation_or_install(self):
        self.assertEqual(self.app.run(), 0)
        state = State(self.state.path)
        self.assertEqual(state.data['active'], self.old.hash)
        self.assertEqual(state.decision('image', self.new.hash, 15), 'pending')
        self.assertIsNone(state.data['journal'])
        self.assertFalse(any('systemctl' in args or 'install.sh' in ' '.join(args) for args, _ in self.calls))
        self.assertEqual(json.loads(self.paths.result.read_bytes())['status'], 'verified')

    def test_activate_completes_pending_and_seen_manifest_is_noop(self):
        self.paths.mode.write_text('activate\n')
        self.assertEqual(self.app.run(), 0)
        self.assertEqual(State(self.state.path).data['active'], self.new.hash)
        before = self.state.path.read_bytes()
        self.calls.clear()
        self.assertEqual(self.app.run(), 0)
        self.assertEqual(self.state.path.read_bytes(), before)
        self.assertFalse(any('systemctl' in args or 'pull' in args for args, _ in self.calls))

    def test_failed_activation_recovers_and_noop_does_not_clear_alarm(self):
        self.paths.mode.write_text('activate\n')
        self.error_step = 'services'
        self.assertEqual(self.app.run(), 1)
        self.assertEqual(json.loads(self.paths.result.read_bytes())['status'], 'rolled_back')
        self.assertEqual(State(self.state.path).data['active'], self.old.hash)
        self.assertEqual(self.app.run(), 0)
        self.assertEqual(json.loads(self.paths.result.read_bytes())['status'], 'rolled_back')

    def test_both_locks_end_run_without_state_result_or_commands(self):
        for path in (self.paths.lock, self.paths.backup_lock):
            with self.subTest(path=path), path.open('a') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                before = self.state.path.read_bytes()
                self.calls.clear()
                self.assertEqual(self.app.run(), 0)
                self.assertEqual(self.state.path.read_bytes(), before)
                self.assertFalse(self.paths.result.exists())
                self.assertFalse(self.paths.log.exists())
                self.assertEqual(self.calls, [])

    def test_lost_state_records_failure_without_reset_or_host_command(self):
        self.state.path.unlink()
        self.calls.clear()
        self.assertEqual(self.app.run(), 1)
        self.assertFalse(self.state.path.exists())
        self.assertEqual(json.loads(self.paths.result.read_bytes())['status'], 'failed')
        self.assertEqual(self.calls, [])

    def test_open_journal_recovers_before_any_registry_fetch(self):
        self.crash_step = 'migrate'
        with self.assertRaises(Crash):
            self.engine.activate(self.new)
        self.registry.reset_mock()
        self.assertEqual(self.app.run(), 1)
        self.registry.fetch.assert_not_called()
        self.assertEqual(State(self.state.path).data['journal']['status'], 'rolled_back')

    def test_bootstrap_requires_verified_matching_installed_bundle_and_sets_verify_mode(self):
        self.state.path.unlink()
        self.paths.mode.unlink()
        self.registry.find.return_value = self.old.directory
        self.paths.host.joinpath('images.env').write_bytes(pins(self.old.manifest))
        for name in ('server.env', 'web.env'):
            self.paths.host.joinpath(name).write_bytes(self.old.files[name])
        self.assertEqual(self.app.bootstrap(self.old.hash), 0)
        self.assertEqual(State(self.state.path).data['active'], self.old.hash)
        self.assertEqual(self.paths.mode.read_text(), 'verify\n')
        self.assertEqual(self.paths.mode.stat().st_mode & 0o777, 0o644)

    def rollback_artifact(self, current, target, sequence=1):
        directory = self.paths.state / 'rollback-fixture'
        directory.mkdir(exist_ok=True)
        data = canonical(dict(kind='trachuan.rollback', version=1, current=current,
                              target=target, sequence=sequence))
        (directory / 'manifest.json').write_bytes(data)
        (directory / 'manifest.sigstore.json').write_bytes(b'{}')
        self.registry.latest_rollback.return_value = str(sequence)
        self.registry.fetch.side_effect = lambda repository, tag, path, kind: directory if kind == 'rollback' else self.new.directory
        return hashlib.sha256(data).hexdigest()

    def test_authorized_rollback_is_consumed_before_activation_and_not_replayed_by_timer(self):
        self.paths.mode.write_text('activate\n')
        self.assertEqual(self.app.run(), 0)
        authorization = self.rollback_artifact(self.new.hash, self.old.hash)
        self.assertEqual(self.app.run(), 0)
        state = State(self.state.path)
        self.assertEqual(state.data['active'], self.old.hash)
        self.assertIn(authorization, state.data['consumed'])
        self.assertEqual(state.data['journal']['authorization'], authorization)
        before = state.path.read_bytes()
        self.calls.clear()
        self.assertEqual(self.app.run(), 0)
        self.assertEqual(state.path.read_bytes(), before)
        self.assertFalse(any('systemctl' in args or 'psql' in args for args, _ in self.calls))

    def test_schema_change_refuses_signed_rollback_without_consuming_authorization(self):
        self.paths.mode.write_text('activate\n')
        self.assertEqual(self.app.run(), 0)
        authorization = self.rollback_artifact(self.new.hash, self.old.hash)
        self.schema = 8
        before = self.state.path.read_bytes()
        self.calls.clear()
        self.assertEqual(self.app.run(), 1)
        self.assertEqual(self.state.path.read_bytes(), before)
        self.assertNotIn(authorization, State(self.state.path).data['consumed'])
        self.assertFalse(any('systemctl' in args or 'install.sh' in ' '.join(args) for args, _ in self.calls))

    def test_failed_image_verification_keeps_pending_binding_and_does_not_start_journal(self):
        self.images.side_effect = ValueError('wrong image digest')
        self.assertEqual(self.app.run(), 1)
        state = State(self.state.path)
        self.assertEqual(state.data['active'], self.old.hash)
        self.assertIsNone(state.data['journal'])
        self.assertEqual(state.decision('image', self.new.hash, 15), 'pending')
        self.assertFalse(any('systemctl' in args for args, _ in self.calls))

    def test_caddy_stop_failure_records_failure_and_leaves_open_journal(self):
        self.crash_step = 'edge'
        with self.assertRaises(Crash):
            self.engine.activate(self.new)
        self.error_step = 'edge'
        self.assertEqual(self.app.run(), 1)
        self.assertEqual(json.loads(self.paths.result.read_bytes())['status'], 'failed')
        self.assertEqual(State(self.state.path).data['journal']['status'], 'started')

    def test_bootstrap_lock_contention_returns_failure_without_changing_state(self):
        before = self.state.path.read_bytes()
        with self.paths.backup_lock.open('a') as lock:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            self.assertEqual(self.app.bootstrap(self.old.hash), 1)
        self.assertEqual(self.state.path.read_bytes(), before)
        self.assertFalse(self.paths.result.exists())

    def test_transient_fetch_alarm_clears_after_healthy_seen_manifest(self):
        self.registry.fetch.side_effect = CommandError('synthetic unavailable registry')
        self.assertEqual(self.app.run(), 1)
        self.registry.fetch.side_effect = None
        self.registry.fetch.return_value = self.old.directory
        self.assertEqual(self.app.run(), 0)
        self.assertEqual(json.loads(self.paths.result.read_bytes())['status'], 'skipped')

    def test_bootstrap_refuses_matching_pins_with_stale_running_image(self):
        self.state.path.unlink()
        self.registry.find.return_value = self.old.directory
        self.paths.host.joinpath('images.env').write_bytes(pins(self.old.manifest))
        for name in ('server.env', 'web.env'):
            self.paths.host.joinpath(name).write_bytes(self.old.files[name])
        self.live_override['server'] = 'ghcr.io/dannyota/trachuan-server@sha256:' + '0' * 64
        self.assertEqual(self.app.bootstrap(self.old.hash), 1)
        self.assertFalse(self.state.path.exists())

    def test_authorization_for_current_active_manifest_runs_before_new_production_release(self):
        self.paths.mode.write_text('activate\n')
        previous = self.candidate(13, b'previous')
        self.store.retain(previous)
        state = State.bootstrap(self.state.path, previous.hash, 13, 7,
                                self.old.manifest['host_bundle']['sha256'])
        state.accept('image', self.old.hash, 14)
        state.activate(self.old.hash, 7)
        self.rollback_artifact(self.old.hash, previous.hash)
        self.assertEqual(self.app.run(), 0)
        self.assertEqual(State(self.state.path).data['active'], previous.hash)
        self.assertEqual(self.app.run(), 0)
        self.assertEqual(State(self.state.path).data['active'], self.new.hash)

    def test_unconfirmed_backup_marker_skips_run_and_bootstrap_without_host_changes(self):
        marker = self.paths.backup_lock.with_name('active-job')
        marker.write_bytes(b'synthetic unconfirmed backup')
        before = self.state.path.read_bytes()
        for action, expected in ((self.app.run, 0), (lambda: self.app.bootstrap(self.old.hash), 1)):
            self.calls.clear()
            self.registry.reset_mock()
            self.assertEqual(action(), expected)
            self.assertEqual(self.state.path.read_bytes(), before)
            self.assertEqual(json.loads(self.paths.result.read_bytes())['status'], 'skipped')
            self.registry.fetch.assert_not_called()
            self.registry.find.assert_not_called()
            self.assertEqual(self.calls, [])

    def test_crash_when_rollback_consumption_is_saved_also_has_activation_journal(self):
        self.paths.mode.write_text('activate\n')
        self.assertEqual(self.app.run(), 0)
        authorization = self.rollback_artifact(self.new.hash, self.old.hash)
        save = State.save
        def crash(state):
            save(state)
            if authorization in state.data['consumed']:
                raise Crash()
        with mock.patch.object(State, 'save', crash), self.assertRaises(Crash):
            self.app.run()
        state = State(self.state.path)
        self.assertIn(authorization, state.data['consumed'])
        self.assertEqual(state.data['journal']['authorization'], authorization)
        self.assertEqual(state.data['journal']['target'], self.old.hash)
        self.assertEqual(state.data['journal']['status'], 'started')
        self.assertEqual(self.app.run(), 1)
        self.assertEqual(State(self.state.path).data['journal']['status'], 'rolled_back')

    def test_stopped_run_reasserts_hold_and_stops_caddy_even_when_backup_is_pending(self):
        self.crash_step = 'migrate'
        with self.assertRaises(Crash):
            self.engine.activate(self.new)
        self.schema = 8
        self.assertEqual(self.engine.recover(), 'stopped')
        self.paths.hold.unlink()
        self.paths.backup_lock.with_name('active-job').touch()
        self.calls.clear()
        self.assertEqual(self.app.run(), 0)
        self.assertTrue(self.paths.hold.exists())
        self.assertEqual([args for args, _ in self.calls],
                         [['systemctl', '--user', 'stop', 'trachuan-caddy.service']])
        self.assertEqual(json.loads(self.paths.result.read_bytes())['status'], 'skipped')

    def test_higher_release_can_activate_after_stopped_journal(self):
        self.crash_step = 'migrate'
        with self.assertRaises(Crash):
            self.engine.activate(self.new)
        self.schema = 8
        self.assertEqual(self.engine.recover(), 'stopped')
        fixed = self.candidate(16, b'fixed')
        self.registry.fetch.return_value = fixed.directory
        self.paths.mode.write_text('activate\n')
        self.assertEqual(self.app.run(), 0)
        self.assertEqual(State(self.state.path).data['active'], fixed.hash)
        self.assertFalse(self.paths.hold.exists())


if __name__ == '__main__':
    unittest.main()
