from pathlib import Path
import subprocess
import tempfile
import unittest

from updater import activation_test as fixture
from updater.deploy_io import CommandError


PROBES = ('/usr/local/bin/postgres-admin.sh', '/run/secrets/activator_db_password')
FEED = ['/usr/local/bin/postgres-admin.sh', 'bash', '-s']


class ProvisionTest(fixture.ActivationFixture, unittest.TestCase):
    def test_frameworks_pointer_grant_is_guarded_and_excludes_delete(self):
        script = (Path(__file__).parents[1] / 'provision-activator.sh').read_text()
        self.assertIn("SELECT 'GRANT SELECT, INSERT, UPDATE ON public.frameworks_release TO activator'\n"
                      "WHERE pg_catalog.to_regclass('public.frameworks_release') IS NOT NULL \\gexec", script)
        self.assertNotIn("'retired_release','frameworks_release'", script)

    def provision_input(self):
        script = (Path(__file__).parents[1] / 'provision-activator.sh').read_text()
        with tempfile.NamedTemporaryFile('w', suffix='.secret') as secret:
            secret.write('synthetic-password')
            secret.flush()
            script = script.replace('/run/secrets/activator_db_password', secret.name)
            script = script.replace('/run/secrets/auth_db_password', secret.name)
            script = script.replace('/usr/local/bin/postgres-admin.sh', 'postgres_admin')
            fake = ('postgres_admin() { printf "PGOPTIONS=%s\\n" "${PGOPTIONS-}"; cat; }\n'
                    'timeout() { printf "PGOPTIONS=%s\\n" "${PGOPTIONS-}"; cat; }\n')
            result = subprocess.run(['bash', '-s'], input=fake + script, capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        return result.stdout

    def test_activator_settings_are_reset_except_in_frameworks_databases(self):
        text = self.provision_input()
        self.assertIn('ALTER ROLE activator RESET ALL;\n', text)
        self.assertIn("pg_catalog.format('ALTER ROLE activator IN DATABASE %I RESET ALL', d.datname)", text)
        self.assertIn('FROM pg_catalog.pg_db_role_setting s', text)
        self.assertIn("d.datname OPERATOR(pg_catalog.!~) '^frameworks_[0-9]{8}(_[0-9]{1,2})?$' \\gexec", text)
        self.assertLess(text.index('ALTER ROLE activator LOGIN'), text.index('ALTER ROLE activator RESET ALL;'))

    def test_provisioner_session_searches_only_the_catalog(self):
        self.assertEqual(self.provision_input().splitlines()[0].split(' -c ')[-1], 'search_path=pg_catalog')

    def test_provision_feeds_the_installed_script_over_stdin(self):
        self.calls.clear()
        self.paths.host.joinpath('provision-activator.sh').write_bytes(b'activated bundle script\n')
        self.assertTrue(self.host.provision_activator())
        args = [call for call, _ in self.calls]
        self.assertEqual(args[:2], [['podman', 'exec', 'trachuan-postgres', 'test', '-r', path] for path in PROBES])
        self.assertEqual(args[2], ['podman', 'exec', '-i', '--env', 'PGREQUIREAUTH=scram-sha-256',
                                   'trachuan-postgres', *FEED])
        self.assertEqual(self.calls[2][1], b'activated bundle script\n')

    def test_missing_installed_script_is_not_provisioned_and_opens_no_container_session(self):
        self.paths.host.joinpath('provision-activator.sh').unlink()
        self.calls.clear()
        self.assertFalse(self.host.provision_activator())
        self.assertEqual(self.calls, [])

    def test_images_skip_provisioning_when_either_container_path_is_unreadable(self):
        for missing in PROBES:
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
                self.assertFalse(any(call[-3:] == FEED for call in args))
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
            if args[-3:] == FEED:
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
        provision = next(i for i, a in enumerate(args) if a[-3:] == FEED)
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
