import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest import mock

from updater.database_guard_test import LAW_NAME, LawFixture
from updater.deploy_corpus_lock import AdvisoryLock
from updater.frameworks_steps_test import StepsFixture


SCRAM = 'PGREQUIREAUTH=scram-sha-256'
SECRET = '/run/secrets/activator_db_password'


def container_env(args):
    stop = args.index('trachuan-postgres')
    return [args[index + 1] for index in range(stop) if args[index] == '--env']


def wrapper(args):
    return args[args.index('-c') + 1]


class AdminScramTest(LawFixture, unittest.TestCase):
    def assert_scram(self, match):
        found = [args for args in self.calls if match(args)]
        self.assertTrue(found)
        for args in found:
            self.assertIn(SCRAM, container_env(args))

    def test_admin_psql(self):
        self.host.psql('auth', 'SELECT 1;')
        self.assert_scram(lambda args: 'psql' in args)

    def test_postgres_restart_wait_prewarm_and_backup_check(self):
        self.host.postgres()
        self.assert_scram(lambda args: '/usr/local/bin/wait-postgres.sh' in args)
        self.assert_scram(lambda args: 'pgbackrest' in args)
        self.assert_scram(lambda args: 'psql' in args and '/usr/local/bin/postgres-admin.sh' in args)

    def test_activator_provisioning(self):
        self.assertTrue(self.host.provision_activator())
        self.assert_scram(lambda args: args[-2:] == ['bash', '-s'])

    def test_target_preparation_session(self):
        self.db.target_admin(self.row, 'SELECT 1;')
        self.assert_scram(lambda args: '/usr/local/bin/postgres-admin.sh' in args)


class ActivatorScramTest(LawFixture, unittest.TestCase):
    def test_wrapper_exports_the_requirement_with_the_password(self):
        script = wrapper(self.db.arguments('env'))
        self.assertIn(SECRET, script)
        with tempfile.TemporaryDirectory() as work:
            secret = Path(work) / 'password'
            secret.write_text('synthetic')
            result = subprocess.run(['sh', '-c', script.replace(SECRET, str(secret)), 'sh', 'env'],
                                    env={'PATH': os.environ['PATH']}, capture_output=True, text=True, check=True)
        lines = result.stdout.splitlines()
        self.assertIn(SCRAM, lines)
        self.assertIn('PGPASSWORD=synthetic', lines)

    def test_every_activator_client_runs_through_the_wrapper(self):
        expected = wrapper(self.db.arguments('env'))
        self.db.query('SELECT 1;')
        self.host.activator_psql(LAW_NAME, 'SELECT 1;')
        self.db.make(self.row)
        self.db.drop(LAW_NAME)
        self.db.load(self.row, '/synthetic/dump', '/run/pgbackrest/corpus-x.list')
        for program in ('psql', 'createdb', 'dropdb', 'pg_restore'):
            with self.subTest(program=program):
                found = [args for args in self.calls if program in args and '-U' in args
                         and args[args.index('-U') + 1] == 'activator']
                self.assertTrue(found)
                for args in found:
                    self.assertEqual(wrapper(args), expected)

    def test_lock_session_runs_through_the_wrapper(self):
        process = mock.Mock()
        selector = mock.MagicMock()
        selector.__enter__.return_value.select.return_value = [(None, None)]
        with mock.patch('updater.deploy_corpus_lock.subprocess.Popen', return_value=process) as start, \
                mock.patch('updater.deploy_corpus_lock.selectors.DefaultSelector', return_value=selector), \
                mock.patch('updater.deploy_corpus_lock.os.read', return_value=b't|4242\n'):
            with AdvisoryLock(self.db):
                pass
        args = start.call_args.args[0]
        self.assertEqual(wrapper(args), wrapper(self.db.arguments('env')))
        self.assertIn(SCRAM, wrapper(args))
        self.assertEqual(args[args.index('-d') + 1], 'auth')


class FrameworksScramTest(StepsFixture, unittest.TestCase):
    def test_frameworks_admin_session(self):
        self.db.target_admin(self.row, 'SELECT 1;')
        self.assertIn(SCRAM, container_env(self.call('postgres')[0]))


if __name__ == '__main__':
    unittest.main()
