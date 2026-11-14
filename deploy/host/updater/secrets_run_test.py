import fcntl
import json
import unittest
from unittest import mock

from updater import deploy_run
from updater import secrets_test as fixture
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
        self.state.data['secrets']['pending'] = {'google_client_secret': 2}
        self.state.save()
        self.paths.hold.touch()
        self.fail_create = 'google_client_secret'
        with mock.patch('updater.deploy_run.Activator.recover') as recover:
            self.assertEqual(self.app.run(), 1)
        recover.assert_not_called()
        self.assertTrue(self.paths.hold.exists())

    def test_pending_failure_never_runs_image_recovery(self):
        self.state.data['secrets']['pending'] = {'google_client_secret': 2}
        self.state.save()
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
