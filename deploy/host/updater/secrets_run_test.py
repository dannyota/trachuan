import fcntl
import json
import unittest
from unittest import mock

from updater import activation_test as activation_fixture
from updater import deploy_run
from updater import secrets_test as fixture
from updater.deploy_activate import Activator
from updater.deploy_io import canonical
from updater.deploy_state import State


class SecretsRunTest(fixture.SecretsFixture, unittest.TestCase):
    def setUp(self):
        super().setUp()
        self.paths.lock.parent.mkdir(parents=True, exist_ok=True)
        self.paths.backup_lock.parent.mkdir(parents=True, exist_ok=True)
        self.paths.backup_lock.touch()
        self.paths.mode.parent.mkdir(parents=True, exist_ok=True)
        self.paths.mode.write_text('activate\n')
        self.paths.policy.write_bytes(canonical(self.rules))
        self.app = deploy_run.Updater(self.paths, self.secret_runner, user_prefix=[])
        self.app.registry = mock.Mock()
        self.app.registry.fetch.return_value = self.new.directory
        self.app.registry.latest_rollback.return_value = None
        patch = mock.patch('updater.deploy_run.Verifier.images')
        patch.start()
        self.addCleanup(patch.stop)

    def test_secret_metadata_is_private_and_separate_from_image_state(self):
        main_fields = {'version', 'active', 'accepted', 'retained', 'failed', 'consumed',
                       'schemas', 'installed_bundle', 'journal'}
        before = self.state.path.read_bytes()
        self.app.phases = ('secrets',)
        self.assertEqual(self.app.run(), 0)
        self.assertEqual(self.state.path.read_bytes(), before)
        self.assertEqual(set(json.loads(self.state.path.read_bytes())), main_fields)
        path = self.paths.state / 'secrets-state.json'
        self.assertEqual(path.stat().st_mode & 0o777, 0o600)
        metadata = json.loads(path.read_bytes())
        self.assertEqual(set(metadata), {'versions', 'pending', 'last_success_at', 'held'})
        self.assertEqual(metadata['versions'], self.versions)
        self.assertEqual(metadata['pending'], {})
        self.assertIsNotNone(metadata['last_success_at'])
        self.assertFalse(metadata['held'])

    def test_pre_secrets_bootstrap_state_and_absent_metadata_adopt(self):
        original = dict(version=1, active=self.old.hash,
                        accepted={'image': {'14': {'hash': self.old.hash, 'status': 'active'}},
                                  'rollback': {}, 'corpus': {}},
                        retained=[], failed=[], consumed=[], schemas={self.old.hash: 7},
                        installed_bundle=self.old.manifest['host_bundle']['sha256'], journal=None)
        self.state.path.write_bytes(canonical(original) + b'\n')
        path = self.paths.state / 'secrets-state.json'
        self.assertFalse(path.exists())
        self.app.phases = ('secrets',)
        self.assertEqual(self.app.run(), 0)
        self.assertEqual(State(self.state.path).data, original)
        self.assertEqual(json.loads(path.read_bytes())['versions'], self.versions)
        self.assertEqual(self.restarts(), [])

    def test_corrupt_secret_metadata_fails_secrets_but_images_still_activate(self):
        metadata = dict(versions={}, pending={}, last_success_at=None, held=False)
        cases = [b'{synthetic corrupt metadata', b'[]', b'null', b'{}',
                 canonical(dict(metadata, versions={'unlisted': 1})),
                 canonical(dict(metadata, pending={'smtp_password': 'invalid'})),
                 canonical(dict(metadata, held='invalid')),
                 canonical(dict(metadata, last_success_at='invalid'))]
        for raw in cases:
            with self.subTest(raw=raw):
                self.setUp()
                path = self.paths.state / 'secrets-state.json'
                path.write_bytes(raw)
                installed = dict(self.installed)
                self.assertEqual(self.app.run(), 1)
                self.assertEqual(State(self.state.path).data['active'], self.new.hash)
                self.assertEqual(self.installed, installed)
                self.assertEqual(path.read_bytes(), raw)
                result = json.loads(self.paths.result.with_name('secrets.json').read_bytes())
                self.assertEqual((result['status'], result['step']), ('failed', 'secrets'))
                self.assertEqual(json.loads(self.paths.result.read_bytes())['status'], 'ok')
                self.assertFalse(any('deploy_vault.py' in ' '.join(args) for args, _, _ in self.secret_calls))
                self.assert_safe()

    def test_rollback_reinstall_and_crash_preserve_main_state_fields(self):
        main_fields = {'version', 'active', 'accepted', 'retained', 'failed', 'consumed',
                       'schemas', 'installed_bundle', 'journal'}
        self.app.phases = ('secrets',)
        self.assertEqual(self.app.run(), 0)
        path = self.paths.state / 'secrets-state.json'
        metadata = path.read_bytes()
        self.crash_step = 'migrate'
        with self.assertRaises(activation_fixture.Crash):
            self.engine.activate(self.new)
        self.assertEqual(set(json.loads(self.state.path.read_bytes())), main_fields)
        install = self.host.install
        def interrupted_reinstall(directory):
            install(directory)
            self.assertEqual(directory, self.paths.bundles / self.old.manifest['host_bundle']['sha256'])
            self.assertEqual(set(json.loads(self.state.path.read_bytes())), main_fields)
            raise activation_fixture.Crash()
        with mock.patch.object(self.host, 'install', side_effect=interrupted_reinstall):
            with self.assertRaises(activation_fixture.Crash):
                self.engine.recover()
        self.assertEqual(set(json.loads(self.state.path.read_bytes())), main_fields)
        state = State(self.state.path)
        self.assertEqual(state.data['journal']['status'], 'recovering')
        self.assertEqual(Activator(state, self.store, self.host).recover(), 'rolled_back')
        self.assertEqual(State(self.state.path).data['active'], self.old.hash)
        self.assertEqual(set(json.loads(self.state.path.read_bytes())), main_fields)
        self.assertEqual(path.read_bytes(), metadata)

    def test_hourly_phase_and_force_before_image_activation(self):
        self.assertEqual(self.app.phases, ('secrets', 'images'))
        with mock.patch('updater.deploy_run.Secrets.run', return_value=True) as run:
            self.assertEqual(self.app.run(), 0)
        self.assertEqual(run.call_args_list, [mock.call(), mock.call(force=True)])

    def test_sdk_failure_allows_images_when_no_replacement_started(self):
        self.vault_error = 'google_client_secret'
        self.assertEqual(self.app.run(), 1)
        self.assertEqual(State(self.state.path).data['active'], self.new.hash)
        self.assertEqual(json.loads(self.paths.result.with_name('secrets.json').read_bytes())['status'], 'failed')
        self.assert_safe()

    def test_pending_replacement_blocks_registry_and_activation(self):
        self.changed()
        self.state.save()
        self.fail_create = 'google_client_secret'
        self.assertEqual(self.app.run(), 1)
        self.app.registry.fetch.assert_not_called()
        self.assertEqual(json.loads(self.paths.result.read_bytes())['step'], 'secrets')
        self.assertEqual(State(self.state.path).data['active'], self.old.hash)
        self.assert_safe()

    def test_postgres_replacement_holds_backup_lock_and_waits(self):
        self.changed('backup_s3_key')
        self.state.save()
        original = self.app.host.postgres
        observed = []
        def postgres():
            with self.paths.backup_lock.open('r+') as file:
                with self.assertRaises(BlockingIOError):
                    fcntl.flock(file, fcntl.LOCK_EX | fcntl.LOCK_NB)
            observed.append(True)
            return original()
        self.app.host.postgres = postgres
        self.app.phases = ('secrets',)
        self.assertEqual(self.app.run(), 0)
        self.assertEqual(observed, [True])

    def test_pending_secrets_recover_before_activation_can_clear_hold(self):
        self.changed()
        self.reconcile.data['pending'] = {'google_client_secret': 2}
        self.reconcile.storage.save()
        self.paths.hold.touch()
        self.fail_create = 'google_client_secret'
        with mock.patch('updater.deploy_run.Activator.recover') as recover:
            self.assertEqual(self.app.run(), 1)
        recover.assert_not_called()
        self.assertTrue(self.paths.hold.exists())

    def test_pending_failure_never_runs_image_recovery(self):
        self.reconcile.data['pending'] = {'google_client_secret': 2}
        self.reconcile.storage.save()
        with mock.patch.object(self.app.host, 'hold', side_effect=OSError('synthetic hold failure')), \
             mock.patch('updater.deploy_run.Activator.recover') as recover:
            self.vault_error = 'smtp_password'
            self.assertEqual(self.app.run(), 1)
        recover.assert_not_called()
        self.app.registry.fetch.assert_not_called()

    def test_placeholder_does_not_disable_image_activation(self):
        self.app.rules['vault_id'] = 'ocid1.vault.REPLACE'
        self.assertEqual(self.app.run(), 1)
        self.assertEqual(State(self.state.path).data['active'], self.new.hash)
        self.assertFalse(any('deploy_vault.py' in ' '.join(args) for args, _, _ in self.secret_calls))


if __name__ == '__main__':
    unittest.main()
