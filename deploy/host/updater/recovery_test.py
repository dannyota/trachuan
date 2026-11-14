import json
import os
from pathlib import Path
import shlex
import subprocess
import sys
import unittest
from unittest import mock

from updater.activation_test import ActivationFixture, Crash
from updater.deploy_io import CommandError, canonical
from updater.deploy_state import State
from updater.deploy_verify import pull_image
from updater.deploy_run import Updater


class TransferTest(ActivationFixture, unittest.TestCase):
    def setUp(self):
        previous = os.umask(0o077)
        self.addCleanup(os.umask, previous)
        super().setUp()
        self.transfers = []
        self.host.user_prefix = ['as-trachuan']
        self.host.runner = self.separate_user

    def separate_user(self, args, input=None, **kwargs):
        if args[0] != 'as-trachuan':
            return self.runner(args, input=input, **kwargs)
        args = args[1:]
        for arg in args:
            if arg.startswith(str(self.paths.bundles)) or arg.startswith(str(self.paths.state)):
                raise CommandError('separate user cannot traverse root private paths')
        if args[:3] == ['python3', '-I', '-c']:
            destination = Path(args[-2])
            self.assertIn(destination.parent, (self.paths.quadlet, self.paths.user_units,
                                              self.paths.host / 'quadlet', self.paths.host / 'systemd'))
            result = subprocess.run([sys.executable, *args[1:]], input=input, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.transfers.append((destination, input))
            self.assertEqual(destination.read_bytes(), input)
            self.assertEqual(destination.stat().st_uid, os.getuid())
            self.assertEqual(destination.stat().st_mode & 0o777, int(args[-1], 8))
            return result.stdout
        return self.runner(args, input=input, **kwargs)

    def test_recovery_streams_private_bundle_content_to_user_destination(self):
        self.paths.postgres_unit.write_bytes(b'changed\n')
        self.crash_step = 'migrate'
        with self.assertRaises(Crash):
            self.engine.activate(self.new)
        directory = self.store.unpack(self.old)
        self.assertEqual(directory.stat().st_mode & 0o777, 0o700)
        self.assertEqual(self.engine.recover(), 'rolled_back')
        wanted = self.old.files['quadlet/trachuan-postgres.container'].replace(
            b'@POSTGRES_IMAGE@', self.old.manifest['images']['postgres'].encode())
        self.assertIn((self.paths.postgres_unit, wanted), self.transfers)

    def test_bundle_installer_streams_every_user_directory_template(self):
        installer = Path(__file__).parents[1] / 'install.sh'
        source = installer.read_text()
        directory = self.store.unpack(self.old)
        (directory / 'systemd').mkdir()
        (directory / 'systemd/trachuan-backup.service').write_bytes(b'[Service]\n')
        for name in ('systemd', 'quadlet'):
            (self.paths.host / name).mkdir()
        helper = installer.parent / 'updater/deploy_transfer.py'
        (directory / 'updater').mkdir()
        (directory / 'updater/deploy_transfer.py').write_bytes(helper.read_bytes())
        command = source[source.index('for template in "$bundle"/systemd/*; do'):]
        command = command[:command.index('if ! semanage')]
        command = command.replace('/srv/trachuan/host/', str(self.paths.host) + '/')
        user = self.paths.root / 'user-runner.py'
        user.write_text('import pathlib, subprocess, sys\n'
                        'args = sys.argv[1:]\n'
                        'private = sys.argv[1]\n'
                        'assert not any(arg.startswith(private) for arg in args[1:])\n'
                        'assert args[1:4] == ["python3", "-I", "-c"]\n'
                        'raise SystemExit(subprocess.run(args[1:]).returncode)\n')
        prelude = ('set -euo pipefail\n'
                   'bundle=' + shlex.quote(str(directory)) + '\n'
                   'user_file_program=$(<"$bundle/updater/deploy_transfer.py")\n'
                   'as_user() { ' + shlex.join([sys.executable, str(user), str(directory)]) + ' "$@"; }\n')
        result = subprocess.run(['bash', '-c', prelude + command], capture_output=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        for name in ('systemd', 'quadlet'):
            for template in (directory / name).iterdir():
                destination = self.paths.host / name / template.name
                self.assertEqual(destination.read_bytes(), template.read_bytes())
                self.assertEqual(destination.stat().st_mode & 0o777, 0o644)


class CachedImageTest(unittest.TestCase):
    def setUp(self):
        self.image = 'ghcr.io/dannyota/trachuan-postgres@sha256:' + 'a' * 64
        self.calls = []

    def test_cached_digest_is_inspected_without_registry_pull(self):
        def runner(args, **kwargs):
            self.calls.append(args)
            if 'pull' in args:
                raise CommandError('registry unavailable')
            return canonical([{'RepoDigests': [self.image]}])
        pull_image(self.image, runner, ['as-trachuan'])
        self.assertEqual(self.calls, [['as-trachuan', 'podman', 'image', 'inspect', self.image]])

    def test_absent_image_is_pulled_then_digest_checked(self):
        cached = []
        def runner(args, **kwargs):
            self.calls.append(args)
            if 'pull' in args:
                cached.append(True)
                return b''
            if not cached:
                raise CommandError('absent', returncode=1)
            return canonical([{'RepoDigests': [self.image]}])
        pull_image(self.image, runner, [])
        self.assertEqual([args[1:-1] for args in self.calls],
                         [['image', 'inspect'], ['image', 'exists'], ['pull'], ['image', 'inspect']])

    def test_local_storage_failure_does_not_pull(self):
        def runner(args, **kwargs):
            self.calls.append(args)
            raise CommandError('storage unavailable', returncode=125)
        with self.assertRaises(CommandError):
            pull_image(self.image, runner, [])
        self.assertFalse(any('pull' in args for args in self.calls))

    def test_cached_and_pulled_digest_mismatches_are_rejected(self):
        for cached in (False, True):
            with self.subTest(cached=cached):
                present = [cached]
                def runner(args, **kwargs):
                    if 'pull' in args:
                        present[0] = True
                        return b''
                    if not present[0]:
                        raise CommandError('absent', returncode=1)
                    return canonical([{'RepoDigests': []}])
                with self.assertRaisesRegex(ValueError, 'local digest mismatch'):
                    pull_image(self.image, runner, [])


class RecoveryCacheTest(ActivationFixture, unittest.TestCase):
    def test_postgres_recovery_with_cached_image_ignores_unavailable_registry(self):
        self.paths.postgres_unit.write_bytes(b'changed\n')
        self.crash_step = 'migrate'
        with self.assertRaises(Crash):
            self.engine.activate(self.new)
        def runner(args, **kwargs):
            if 'pull' in args:
                raise CommandError('registry unavailable')
            return self.runner(args, **kwargs)
        self.host.runner = runner
        self.assertEqual(self.engine.recover(), 'rolled_back')


class HousekeepingTest(ActivationFixture, unittest.TestCase):
    def setUp(self):
        super().setUp()
        self.paths.lock.parent.mkdir(parents=True, exist_ok=True)
        self.paths.backup_lock.parent.mkdir(parents=True, exist_ok=True)
        self.paths.backup_lock.touch()
        self.paths.mode.parent.mkdir(parents=True, exist_ok=True)
        self.paths.mode.write_text('activate\n')
        self.paths.policy.write_bytes(canonical(self.rules))
        self.app = Updater(self.paths, self.runner, user_prefix=[])
        self.app.registry = mock.Mock()
        self.app.registry.fetch.return_value = self.new.directory
        self.app.registry.latest_rollback.return_value = None
        self.images = mock.patch('updater.deploy_run.Verifier.images')
        self.images.start()
        self.addCleanup(self.images.stop)
        secrets = mock.patch('updater.deploy_run.Secrets.run', return_value=True)
        secrets.start()
        self.addCleanup(secrets.stop)

    def test_prune_failure_is_success_and_next_run_can_restart_caddy(self):
        self.paths.mode.write_text('activate\n')
        with mock.patch.object(self.app.store, 'prune', side_effect=OSError('synthetic prune failure')):
            self.assertEqual(self.app.run(), 0)
        state = State(self.state.path)
        self.assertEqual(state.data['journal']['status'], 'activated')
        self.assertEqual(state.data['active'], self.new.hash)
        self.assertFalse(self.paths.hold.exists())
        self.assertEqual(json.loads(self.paths.result.read_bytes())['status'], 'ok')
        rows = [json.loads(line) for line in self.paths.log.read_bytes().splitlines()]
        self.assertTrue(any(row.get('level') == 'warning' and row.get('step') == 'prune' for row in rows))
        before = self.state.path.read_bytes()
        self.app.host.hold()
        ready = []
        def runner(args, **kwargs):
            if args[-1] == 'http://127.0.0.1:8080/readyz':
                self.assertTrue(self.paths.hold.exists())
                ready.append(True)
            if 'start' in args and 'trachuan-caddy.service' in args:
                self.assertTrue(ready)
                self.assertFalse(self.paths.hold.exists())
            return self.runner(args, **kwargs)
        self.app.host.runner = runner
        self.calls.clear()
        self.assertEqual(self.app.run(), 0)
        self.assertTrue(ready)
        self.assertFalse(self.paths.hold.exists())
        self.assertEqual(self.state.path.read_bytes(), before)
        self.assertTrue(any('start' in args and 'trachuan-caddy.service' in args for args, _ in self.calls))

    def test_rollback_prune_failure_preserves_terminal_journal_and_can_resume_ingress(self):
        self.crash_step = 'migrate'
        with self.assertRaises(Crash):
            self.engine.activate(self.new)
        with mock.patch.object(self.store, 'prune', side_effect=OSError('synthetic prune failure')):
            self.assertEqual(self.engine.recover(), 'rolled_back')
        self.assertFalse(self.paths.hold.exists())
        self.assertEqual(State(self.state.path).data['journal']['status'], 'rolled_back')
        before = self.state.path.read_bytes()
        self.host.hold()
        self.calls.clear()
        self.engine.recover()
        self.assertEqual(self.state.path.read_bytes(), before)
        self.assertFalse(self.paths.hold.exists())
        self.assertTrue(any('start' in args and 'trachuan-caddy.service' in args for args, _ in self.calls))

    def test_terminal_ingress_restart_failure_reasserts_hold_without_changing_journal(self):
        self.engine.activate(self.new)
        self.host.hold()
        before = self.state.path.read_bytes()
        def runner(args, **kwargs):
            if 'start' in args and 'trachuan-caddy.service' in args:
                raise CommandError('synthetic caddy failure')
            return self.runner(args, **kwargs)
        self.host.runner = runner
        with self.assertRaises(CommandError):
            self.engine.recover()
        self.assertTrue(self.paths.hold.exists())
        self.assertEqual(self.state.path.read_bytes(), before)


if __name__ == '__main__':
    unittest.main()
