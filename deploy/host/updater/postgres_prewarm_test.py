import json
import unittest

from updater import activation_test as fixture
from updater.deploy_io import CommandError


ACTIVE = 'corpus_vn_20990101_1'


class PostgresPrewarmTest(fixture.ActivationFixture, unittest.TestCase):
    def setUp(self):
        super().setUp()
        self.owner, self.warmed, self.mounted = b'activator\n', b'12\n', True
        self.host.runner = self.prewarm_runner

    def prewarm_runner(self, args, input=None, **kwargs):
        if args[-3:-1] == ['test', '-r'] and not self.mounted:
            self.calls.append((args, input))
            raise CommandError('unreadable mount', returncode=1)
        if input and b'datdba' in input:
            self.calls.append((args, input))
            return self.owner
        if input and b'public.pg_prewarm' in input:
            self.calls.append((args, input))
            return self.warmed
        return self.runner(args, input=input, **kwargs)

    def index(self, match):
        return next(i for i, call in enumerate(self.calls) if match(*call))

    def prewarms(self):
        return [args for args, data in self.calls if data and b'pg_prewarm' in data]

    def logged(self):
        if not self.paths.log.exists():
            return []
        rows = [json.loads(line) for line in self.paths.log.read_text().splitlines()]
        return [row['message'] for row in rows if row.get('step') == 'prewarm']

    def test_prewarm_runs_as_activator_after_provisioning(self):
        self.host.postgres()
        check = self.index(lambda args, _: 'pgbackrest' in args)
        provision = self.index(lambda args, _: '--no-auth-tables' in args)
        prewarm = self.index(lambda _, data: bool(data and b'public.pg_prewarm' in data))
        self.assertLess(check, provision)
        self.assertLess(provision, prewarm)
        args = self.calls[prewarm][0]
        self.assertEqual(args[args.index('-U') + 1], 'activator')
        self.assertEqual(args[args.index('-d') + 1], ACTIVE)
        self.assertIn('PGOPTIONS=-c search_path=pg_catalog', args)
        self.assertIn('PGPASSWORD=$(cat /run/secrets/activator_db_password)', ' '.join(args))
        self.assertNotIn('postgres-admin.sh', ' '.join(args))
        self.assertEqual(self.logged(), [])

    def test_superuser_sessions_stay_out_of_auth_and_the_active_database(self):
        self.host.postgres()
        sessions = [args for args, _ in self.calls if 'postgres-admin.sh' in ' '.join(args) and 'psql' in args]
        self.assertTrue(sessions)
        for args in sessions:
            self.assertEqual(args[args.index('-d') + 1], 'postgres')
            self.assertIn('PGOPTIONS=-c search_path=pg_catalog', args)

    def test_database_activator_does_not_own_is_skipped_with_a_log_line(self):
        self.owner = b'postgres\n'
        self.host.postgres()
        self.assertEqual(self.prewarms(), [])
        self.assertEqual(self.logged(), ['active corpus prewarm skipped: database not owned by activator'])

    def test_database_without_pg_prewarm_is_skipped_with_a_log_line(self):
        self.warmed = b'skipped\n'
        self.host.postgres()
        self.assertEqual(len(self.prewarms()), 1)
        self.assertEqual(self.logged(), ['active corpus prewarm skipped: database has no pg_prewarm'])

    def test_unit_without_the_activator_mounts_skips_without_failing(self):
        self.mounted = False
        self.host.postgres()
        self.assertEqual(self.prewarms(), [])
        self.assertEqual(self.logged(), ['active corpus prewarm skipped: activator not provisioned'])

    def test_unwritable_log_does_not_fail_the_deploy(self):
        self.owner = b'postgres\n'
        self.paths.log.mkdir(parents=True)
        self.host.postgres()
        self.assertEqual(self.prewarms(), [])

    def test_prewarm_failure_still_fails_the_deploy(self):
        def runner(args, input=None, **kwargs):
            if input and b'public.pg_prewarm' in input:
                raise CommandError('command failed')
            return self.prewarm_runner(args, input=input, **kwargs)
        self.host.runner = runner
        with self.assertRaises(CommandError):
            self.host.postgres()

    def test_invalid_active_database_name_refuses_before_any_activator_session(self):
        def runner(args, input=None, **kwargs):
            if b'active_db' in fixture.sql_text(args, input):
                return b'auth\n'
            return self.prewarm_runner(args, input=input, **kwargs)
        self.host.runner = runner
        with self.assertRaisesRegex(ValueError, 'invalid corpus database name'):
            self.host.postgres()
        self.assertEqual(self.prewarms(), [])

    def test_active_database_without_a_release_suffix_is_prewarmed(self):
        def runner(args, input=None, **kwargs):
            if b'active_db' in fixture.sql_text(args, input):
                return b'corpus_vn_20990101\n'
            return self.prewarm_runner(args, input=input, **kwargs)
        self.host.runner = runner
        self.host.postgres()
        args = self.prewarms()[0]
        self.assertEqual(args[args.index('-d') + 1], 'corpus_vn_20990101')


if __name__ == '__main__':
    unittest.main()
