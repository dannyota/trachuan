from pathlib import Path
import unittest

from updater import activation_test as fixture
from updater.deploy_io import CommandError


class ProvisionTest(fixture.ActivationFixture, unittest.TestCase):
    def test_provision_checks_both_running_container_mounts(self):
        self.calls.clear()
        self.assertTrue(self.host.provision_activator())
        args = [call for call, _ in self.calls]
        probes = [['podman', 'exec', 'trachuan-postgres', 'test', '-r', path] for path in
                  ('/usr/local/bin/provision-activator.sh', '/run/secrets/activator_db_password')]
        self.assertEqual(args[:2], probes)
        self.assertEqual(args[2][-2:], ['bash', '/usr/local/bin/provision-activator.sh'])

    def test_images_skip_provisioning_when_either_mount_is_unreadable(self):
        for missing in ('/usr/local/bin/provision-activator.sh', '/run/secrets/activator_db_password'):
            self.calls.clear()
            def runner(args, **kwargs):
                self.runner(args, **kwargs)
                if args[-3:] == ['test', '-r', missing]:
                    raise CommandError('unreadable mount', returncode=1)
                return b'success\n' if 'show' in args else b''
            self.host.runner = runner
            with self.subTest(missing=missing):
                self.host.migrate()
                args = [call for call, _ in self.calls]
                self.assertFalse(any('bash' in call and call[-1] == '/usr/local/bin/provision-activator.sh' for call in args))
                self.assertTrue(any('start' in call and 'trachuan-migrate.service' in call for call in args))
                self.assertEqual(sum(call[-3:-1] == ['test', '-r'] for call in args), 2)

    def test_probe_errors_other_than_unreadable_fail_provisioning(self):
        def runner(args, **kwargs):
            if 'test' in args:
                raise CommandError('container unavailable', returncode=125)
            return self.runner(args, **kwargs)
        self.host.runner = runner
        with self.assertRaises(CommandError):
            self.host.provision_activator()

    def test_interrupted_first_upgrade_recovers_previous_unit_without_mounts(self):
        self.paths.postgres_unit.write_bytes(b'changed\n')
        self.crash_step = 'migrate'
        with self.assertRaises(fixture.Crash):
            self.engine.activate(self.new)
        self.calls.clear()
        def runner(args, **kwargs):
            if args[-3:-1] == ['test', '-r']:
                self.calls.append((args, kwargs.get('input')))
                raise CommandError('old unit has no provisioning mount', returncode=1)
            if 'bash' in args and args[-1] == '/usr/local/bin/provision-activator.sh':
                raise CommandError('provision script absent', returncode=127)
            return self.runner(args, **kwargs)
        self.host.runner = runner
        self.assertEqual(self.engine.recover(), 'rolled_back')
        self.assertEqual(self.state.data['active'], self.old.hash)
        self.assertEqual(self.state.data['journal']['status'], 'rolled_back')
        self.assertFalse(self.paths.hold.exists())
        self.assertEqual(self.paths.postgres_unit.read_bytes(),
                         self.old.files['quadlet/trachuan-postgres.container'].replace(
                             b'@POSTGRES_IMAGE@', self.old.manifest['images']['postgres'].encode()))

    def test_images_provision_after_postgres_and_before_migrate(self):
        self.engine.activate(self.new)
        args = [call for call, _ in self.calls]
        provision = next(i for i, a in enumerate(args) if '/usr/local/bin/provision-activator.sh' in a)
        migration = next(i for i, a in enumerate(args) if 'trachuan-migrate.service' in a and 'start' in a)
        self.assertLess(provision, migration)

    def test_postgres_mounts_secret_and_provision_script(self):
        root = Path(__file__).parents[1]
        unit = (root/'quadlet/trachuan-postgres.container').read_text()
        self.assertIn('Secret=activator_db_password,type=mount,uid=999,gid=999,mode=0400', unit)
        self.assertIn('provision-activator.sh:/usr/local/bin/provision-activator.sh:ro,z', unit)
        script = (root/'provision-activator.sh').read_text()
        self.assertIn('log_min_error_statement=panic', script)
        self.assertIn('NOSUPERUSER NOCREATEROLE NOREPLICATION NOBYPASSRLS', script)
