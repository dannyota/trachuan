import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock

from updater.corpus_verify_test import manifest
from updater.deploy_corpus_database import Database
from updater.deploy_corpus_lock import AdvisoryLock
from updater.deploy_host import Host
from updater.deploy_io import CommandError
from updater.deploy_release_target import LockBusy, LockLost, LockUnavailable, Unreachable
from updater.deploy_store import Paths


def database_of(args):
    return args[args.index('-d') + 1] if '-d' in args else None


class SessionFaultTest(unittest.TestCase):
    def setUp(self):
        self.work = tempfile.TemporaryDirectory()
        self.addCleanup(self.work.cleanup)
        self.calls, self.failure = [], None
        self.db = Database(Host(Paths(Path(self.work.name)), self.runner, user_prefix=[]), {})
        self.db.holder = mock.Mock()
        self.db.holder.poll.return_value = None
        self.db.backend = 77
        self.m = manifest()
        self.row = dict(operation_id='e' * 32, country='vn', rel=self.m['rel'], mode='new',
                        database_name=self.m['database'], manifest_hash='a' * 64, dump_sha256=self.m['dump']['sha256'],
                        expected_active_db='corpus_vn_20990101_1', manifest=self.m, step='creating', oid=None)

    def runner(self, args, **kwargs):
        self.calls.append((args, (kwargs.get('input') or b'').decode()))
        if self.failure is not None and self.failure[0] == database_of(args):
            raise CommandError('command failed', returncode=self.failure[1], detail='psql: exit ' + str(self.failure[1]))
        text = (kwargs.get('input') or b'').decode()
        if 'pg_terminate_backend' in text:
            return b'0\nf\n'
        if 'DROP DATABASE' in text:
            return b'f\n'
        if 'json_object_agg(d.datname' in text:
            return b'{}'
        if 'AS target_oid' in text:
            return b'16400\n'
        if 'datdba' in text:
            return b'activator\n'
        if '-U' in args and args[args.index('-U') + 1] == 'auth_owner':
            return b'[]'
        return b'f\n'

    def activator_sessions(self):
        return [(args, text) for args, text in self.calls if '-U' in args and args[args.index('-U') + 1] == 'activator']

    def test_every_activator_catalog_statement_runs_from_postgres(self):
        self.db.catalog()
        self.db.clear_leftovers()
        self.db.other_superuser()
        self.db.exists(self.m['database'])
        self.db.connections(self.m['database'])
        self.db.owned(self.m['database'])
        self.db.drop_database(self.m['database'], 16400)
        self.db.make(self.row)
        sessions = self.activator_sessions()
        self.assertGreater(len(sessions), 8)
        for args, text in sessions:
            with self.subTest(text=text[:60]):
                self.assertEqual(database_of(args), 'postgres')
                self.assertIn('PGOPTIONS=-c search_path=pg_catalog', args)
        self.assertTrue(any('CREATE DATABASE "' + self.m['database'] + '" TEMPLATE template0;' in text
                            for _, text in sessions))
        self.assertFalse(any('createdb' in args for args, _ in self.calls))

    def test_no_activator_statement_may_name_auth(self):
        with self.assertRaises(ValueError):
            self.db.query('SELECT 1;', 'auth')
        self.assertEqual(self.calls, [])

    def test_a_catalog_session_that_cannot_connect_is_a_wait(self):
        self.failure = ('postgres', 2)
        for action in (self.db.catalog, self.db.other_superuser, self.db.clear_leftovers,
                       lambda: self.db.exists(self.m['database']), lambda: self.db.make(self.row)):
            with self.subTest(action=action), self.assertRaises(Unreachable):
                action()

    def test_other_catalog_failures_stay_command_errors(self):
        self.failure = ('postgres', 3)
        with self.assertRaises(CommandError):
            self.db.catalog()

    def test_a_target_session_that_cannot_connect_is_not_turned_into_a_wait(self):
        self.failure = (self.m['database'], 2)
        with self.assertRaises(CommandError):
            self.db.verify_target(dict(self.row, oid=16400))

    def test_the_catalog_for_reconciling_is_read_from_postgres_without_the_lock(self):
        self.db.holder, self.db.backend = None, None
        self.assertEqual(self.db.catalog(locked=False), {})
        args, text = self.activator_sessions()[0]
        self.assertEqual(database_of(args), 'postgres')
        self.assertIn('PGOPTIONS=-c search_path=pg_catalog', args)
        self.assertIn('FROM pg_catalog.pg_database d', text)
        for word in ('public.', 'corpus_release', 'release_journal', 'retired_release', 'frameworks_release'):
            self.assertNotIn(word, text)
        with self.assertRaises(LockLost):
            self.db.catalog()
        self.failure = ('postgres', 2)
        with self.assertRaises(Unreachable):
            self.db.catalog(locked=False)

    def test_a_lost_lock_is_a_wait_before_any_session(self):
        self.db.holder.poll.return_value = 1
        for action in (self.db.catalog, lambda: self.db.verify_target(dict(self.row, oid=16400))):
            with self.subTest(action=action), self.assertRaises(LockLost):
                action()
        self.db.holder = None
        with self.assertRaises(LockLost):
            self.db.catalog()
        self.db.holder, self.db.backend = mock.Mock(), None
        self.db.holder.poll.return_value = None
        with self.assertRaises(LockLost):
            self.db.clear_leftovers()
        self.assertEqual(self.calls, [])


class LockSessionTest(unittest.TestCase):
    def enter(self, reads, code=None, broken=False):
        db = Database(Host(Paths(), user_prefix=[]), {})
        process = mock.Mock()
        process.poll.return_value = None
        process.wait.return_value = code
        if broken:
            process.stdin.write.side_effect = BrokenPipeError()
        selector = mock.MagicMock()
        selector.__enter__.return_value.select.return_value = [(None, None)]
        with mock.patch('updater.deploy_corpus_lock.subprocess.Popen', return_value=process), \
             mock.patch('updater.deploy_corpus_lock.selectors.DefaultSelector', return_value=selector), \
             mock.patch('updater.deploy_corpus_lock.os.read', side_effect=reads):
            try:
                with AdvisoryLock(db):
                    return 'held'
            finally:
                self.statements = [call.args[0] for call in process.stdin.write.call_args_list]

    def test_a_refused_connection_is_an_unavailable_lock(self):
        with self.assertRaisesRegex(LockUnavailable, 'activator cannot connect to auth'):
            self.enter([b''], code=2)
        with self.assertRaises(LockUnavailable):
            self.enter([b''], code=2, broken=True)

    def test_other_early_exits_stay_command_errors(self):
        for code in (1, 3, None):
            with self.subTest(code=code), self.assertRaises(CommandError):
                self.enter([b''], code=code)

    def test_a_lock_an_activator_session_holds_is_busy(self):
        for answer in (b'1|1\n', b'0|0\n'):
            with self.subTest(answer=answer), self.assertRaisesRegex(LockBusy, 'activation advisory lock busy'):
                self.enter([b'f|4242\n', answer])

    def test_a_lock_another_role_holds_is_unavailable(self):
        for answer in (b'1|0\n', b'2|1\n'):
            with self.subTest(answer=answer), self.assertRaisesRegex(LockUnavailable, 'held by another role'):
                self.enter([b'f|4242\n', answer])

    def test_a_malformed_holder_answer_is_a_command_error(self):
        for answer in (b'x\n', b'1|\n', b'1|1|1\n'):
            with self.subTest(answer=answer), self.assertRaises(CommandError):
                self.enter([b'f|4242\n', answer])

    def test_the_holder_query_reads_only_the_catalog(self):
        with self.assertRaises(LockBusy):
            self.enter([b'f|4242\n', b'1|1\n'])
        holder = self.statements[1].decode()
        for word in ('pg_catalog.pg_locks', 'pg_catalog.pg_stat_activity', 'session_user', "'advisory'",
                     "pg_catalog.hashtextextended('activation',0)", 'pg_catalog.current_database()', 'l.granted'):
            self.assertIn(word, holder)
        for word in ('public.', 'corpus_release', 'release_journal', 'auth_owner'):
            self.assertNotIn(word, holder)

    def test_a_free_lock_is_held(self):
        self.assertEqual(self.enter([b't|4242\n']), 'held')
        self.assertEqual(len(self.statements), 1)


if __name__ == '__main__':
    unittest.main()
