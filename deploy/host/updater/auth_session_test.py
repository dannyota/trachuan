from pathlib import Path
import re
import sys
import tempfile
import unittest
from unittest import mock

from updater import deploy_corpus_sql as law
from updater import deploy_frameworks_sql as frameworks
from updater.deploy_corpus_database import Database
from updater.deploy_corpus_kind import FRAMEWORKS, LAW
from updater.deploy_corpus_lock import AdvisoryLock
from updater.deploy_host import Host, RELEASE_CLIENT_SECONDS, RELEASE_OUTPUT_LIMIT, release_arguments
from updater.deploy_io import CommandError, command
from updater.deploy_release_tables import PointerRefused, TableFailure, Tables
from updater.deploy_release_target import Waiting
from updater.deploy_store import Paths
from updater.frameworks_sql_test import frameworks_row, law_row


SCRIPT = ('set +x; set -eu; export PGPASSWORD PGREQUIREAUTH; PGREQUIREAUTH=scram-sha-256; '
          'PGPASSWORD=$(cat /run/secrets/auth_db_password); [ -n "$PGPASSWORD" ]; exec "$@"')
KINDS = (('law', LAW, law, law_row), ('frameworks', FRAMEWORKS, frameworks, frameworks_row))
UNQUALIFIED_TABLE = re.compile(r'\b(?:FROM|INTO|UPDATE|JOIN)\s+(?!public\.|pg_catalog\.|\()([a-z_]+)')


def options(mode):
    return ('PGOPTIONS=-c search_path=pg_catalog,public -c statement_timeout=30s -c lock_timeout=5s '
            '-c idle_in_transaction_session_timeout=30s -c default_transaction_read_only=' + mode +
            ' -c standard_conforming_strings=on -c client_encoding=UTF8 -c client_connection_check_interval=2s')


class ArgumentsTest(unittest.TestCase):
    def test_session_reads_sql_from_stdin_with_every_startup_option(self):
        for write, mode in ((False, 'on'), (True, 'off')):
            with self.subTest(write=write):
                self.assertEqual(release_arguments(write), [
                    'podman', 'exec', '-i', 'trachuan-postgres', 'sh', '-c', SCRIPT, 'sh',
                    'timeout', '--signal=TERM', '--kill-after=2s', '45s', 'env', 'PGCONNECT_TIMEOUT=5', options(mode),
                    'psql', '--no-password', '-XqAt', '-h', '127.0.0.1', '-U', 'auth_owner',
                    '-v', 'ON_ERROR_STOP=1', '-d', 'auth'])

    def test_no_password_reaches_the_arguments(self):
        self.assertFalse(any(arg.startswith('PGPASSWORD=') for arg in release_arguments(True)))

    def test_client_limit_outlasts_the_container_and_statement_limits(self):
        self.assertEqual(RELEASE_CLIENT_SECONDS, 60)
        self.assertGreaterEqual(RELEASE_OUTPUT_LIMIT, 1024 * 1024)


class BoundedCommandTest(unittest.TestCase):
    def test_input_and_output_bound_work_together(self):
        echo = [sys.executable, '-I', '-c', 'import sys; sys.stdout.buffer.write(sys.stdin.buffer.read())']
        self.assertEqual(command(echo, input=b'SELECT 1;', stdout_limit=64), b'SELECT 1;')

    def test_input_larger_than_a_pipe_does_not_deadlock(self):
        echo = [sys.executable, '-I', '-c',
                'import sys; data=sys.stdin.buffer.read(); sys.stdout.buffer.write(data[:10])']
        self.assertEqual(command(echo, input=b'x' * (1024 * 1024), stdout_limit=64, timeout=10), b'x' * 10)

    def test_output_over_the_bound_fails(self):
        noisy = [sys.executable, '-I', '-c', 'print("x" * 1000)']
        with self.assertRaisesRegex(CommandError, 'exceeds byte limit'):
            command(noisy, input=b'', stdout_limit=10)

    def test_a_client_that_exits_without_reading_input_is_not_an_input_error(self):
        quiet = [sys.executable, '-I', '-c', 'print("done")']
        self.assertEqual(command(quiet, input=b'x' * (1024 * 1024), stdout_limit=64), b'done\n')


class TableFixture:
    def setUp(self):
        self.work = tempfile.TemporaryDirectory()
        self.addCleanup(self.work.cleanup)
        self.calls = []
        self.reply = b''
        self.failure = None
        self.host = Host(Paths(Path(self.work.name)), self.runner, user_prefix=[])

    def runner(self, args, **kwargs):
        self.calls.append((args, kwargs))
        if self.failure:
            raise self.failure
        return self.reply

    def sql(self):
        return self.calls[-1][1]['input'].decode()

    def mode(self):
        return next(arg for arg in self.calls[-1][0] if arg.startswith('PGOPTIONS='))


class TablesTest(TableFixture, unittest.TestCase):
    def test_reads_are_read_only_and_writes_are_not(self):
        for _, kind, _, row in KINDS:
            tables = Tables(self.host, kind)
            self.reply = b''
            tables.pointer()
            self.assertIn('default_transaction_read_only=on', self.mode())
            tables.forget(row()['database_name'])
            self.assertIn('default_transaction_read_only=off', self.mode())

    def test_every_call_is_bounded_and_goes_over_stdin(self):
        Tables(self.host, LAW).pointer()
        args, kwargs = self.calls[-1]
        self.assertEqual(kwargs['stdout_limit'], RELEASE_OUTPUT_LIMIT)
        self.assertEqual(kwargs['timeout'], RELEASE_CLIENT_SECONDS)
        self.assertNotIn('-c', args[args.index('psql'):])
        self.assertIn(b'public.corpus_release', kwargs['input'])

    def test_a_failed_client_is_a_table_failure_that_makes_the_phase_wait(self):
        self.failure = CommandError('command failed', detail='psql: exit 2')
        with self.assertRaises(TableFailure) as raised:
            Tables(self.host, LAW).pointer()
        self.assertIsInstance(raised.exception, Waiting)

    def test_pointer_is_checked_for_the_kind(self):
        for kind, good, bad in ((LAW, 'corpus_vn_20990101_1', 'frameworks_20990101'),
                                (FRAMEWORKS, 'frameworks_20990101', 'corpus_vn_20990101_1')):
            tables = Tables(self.host, kind)
            self.reply = good.encode() + b'\n'
            self.assertEqual(tables.pointer(), good)
            self.reply = b'\n'
            self.assertIsNone(tables.pointer())
            for value in (bad, 'auth', 'host=127.0.0.1 dbname=x', good + '\n' + good):
                self.reply = value.encode() + b'\n'
                with self.subTest(value=value), self.assertRaises(PointerRefused) as raised:
                    tables.pointer()
                self.assertIsInstance(raised.exception, Waiting)
                self.assertNotIn(value, str(raised.exception))

    def test_protected_names_are_strings_only(self):
        tables = Tables(self.host, LAW)
        self.reply = b'["corpus_vn_20990101_1","it\'s odd"]'
        self.assertEqual(tables.protected(), {'corpus_vn_20990101_1', "it's odd"})
        for reply in (b'{', b'{}', b'[1]', b'[null]'):
            self.reply = reply
            with self.subTest(reply=reply), self.assertRaises(TableFailure):
                tables.protected()

    def test_protected_query_covers_both_kinds_and_every_source(self):
        self.reply = b'[]'
        Tables(self.host, FRAMEWORKS).protected('e' * 32)
        text = self.sql()
        for part in ('public.corpus_release', 'public.frameworks_release', 'public.retired_release',
                     'retain_until > pg_catalog.now()', 'public.release_journal', "operation_id <> '" + 'e' * 32 + "'"):
            self.assertIn(part, text)

    def test_operations_read_only_open_rows_and_the_own_row(self):
        tables = Tables(self.host, LAW)
        self.reply = b'[]'
        self.assertEqual(tables.operations('e' * 32), [])
        text = self.sql()
        self.assertIn("corpus_kind='law'", text)
        self.assertIn("step NOT IN ('activated','reverted','aborted','pruned')", text)
        self.assertIn("operation_id='" + 'e' * 32 + "'", text)

    def test_every_table_statement_names_its_schema(self):
        statements = []
        for _, _, sql, row in KINDS:
            statements += [sql.current('vn'), sql.operations(), sql.close_open(), sql.forget(row()['database_name']),
                           sql.journal(row(), 'started'), sql.activate(row())]
        statements.append(law.protected_names())
        for text in statements:
            with self.subTest(text=text[:40]):
                self.assertEqual(UNQUALIFIED_TABLE.findall(text.replace('INTO current_db', '')), [])


class CloseOpenTest(TableFixture, unittest.TestCase):
    def test_close_is_one_table_only_statement_for_this_kind(self):
        for name, kind, sql, _ in KINDS:
            Tables(self.host, kind).close_open('e' * 32)
            text = self.sql()
            with self.subTest(kind=name):
                self.assertEqual(text.count('INSERT INTO public.release_journal'), 1)
                self.assertIn("'aborted','complete','" + kind.label + " aborted'", text)
                self.assertIn("corpus_kind='" + kind.name + "'", text)
                self.assertIn("mode <> 'prune'", text)
                self.assertIn("operation_id <> '" + 'e' * 32 + "'", text)
                self.assertNotIn('DROP', text)
                self.assertIn('default_transaction_read_only=off', self.mode())

    def test_without_an_operation_every_open_row_closes(self):
        self.assertNotIn('operation_id <>', law.close_open())


class SwapStatementTest(unittest.TestCase):
    RETIRE = ('corpus_vn_20990101_1', 'b' * 64)

    def test_pointer_at_the_target_is_accepted_and_everything_else_is_still_written(self):
        for name, sql, row, retire in (('law', law, law_row, self.RETIRE),
                                       ('frameworks', frameworks, frameworks_row, ('frameworks_20990101', 'b' * 64))):
            text = sql.activate(row(), retire=retire)
            target, expected = row()['database_name'], row()['expected_active_db']
            with self.subTest(kind=name):
                self.assertIn(f"current_db IS DISTINCT FROM '{target}' AND current_db IS DISTINCT FROM '{expected}'", text)
                self.assertIn("active_db='" + target + "'", text)
                self.assertIn('INSERT INTO public.retired_release', text)
                self.assertIn("'" + retire[0] + "',now(),now()+interval '7 days',NULL,'" + 'b' * 64 + "'", text)
                self.assertIn('INSERT INTO public.release_journal', text)
                self.assertEqual(text.count('BEGIN;'), 1)

    def test_the_summary_fields_are_written_on_every_swap(self):
        text = law.activate(law_row(), 1, self.RETIRE)
        self.assertIn('summary_projection=1', text)
        self.assertIn('summary_database=', text)

    def test_nothing_is_retired_without_a_previous_database(self):
        for sql, row in ((law, law_row), (frameworks, frameworks_row)):
            self.assertNotIn('INSERT INTO public.retired_release', sql.activate(row()))

    def test_a_previous_database_restored_by_hand_keeps_a_null_digest(self):
        text = law.activate(law_row(), retire=('corpus_vn_20990101_1', None))
        self.assertIn("'corpus_vn_20990101_1',now(),now()+interval '7 days',NULL,NULL", text)

    def test_retired_values_are_checked(self):
        for retire in (('auth', None), ('corpus_vn_20990101_1$', None), ('corpus_vn_20990101_1', 'x'),
                       ('corpus_vn_20990101_1', 'b' * 64 + '$')):
            with self.subTest(retire=retire), self.assertRaises(ValueError):
                law.activate(law_row(), retire=retire)

    def test_the_retained_target_row_is_deleted_only_in_retained_mode(self):
        self.assertIn('DELETE FROM public.retired_release', law.activate(law_row('retained')))
        self.assertNotIn('DELETE FROM public.retired_release', law.activate(law_row('new')))
        self.assertIn('DELETE FROM public.retired_release', frameworks.activate(frameworks_row('retained')))


class ActivatorStaysInTheCatalogTest(unittest.TestCase):
    def setUp(self):
        self.work = tempfile.TemporaryDirectory()
        self.addCleanup(self.work.cleanup)
        self.seen = []
        self.host = Host(Paths(Path(self.work.name)), self.runner, user_prefix=[])
        self.db = Database(self.host, {'extensions': ['vector', 'ltree']})
        self.db.holder = mock.Mock()
        self.db.holder.poll.return_value = None
        self.db.backend = 77

    def runner(self, args, **kwargs):
        text = (kwargs.get('input') or b'').decode()
        if '-U' in args and args[args.index('-U') + 1] == 'activator':
            self.seen.append((args, text))
        if 'pg_terminate_backend' in text:
            return b'0\nf\n'
        if 'json_object_agg' in text:
            return b'{}'
        if '-U' in args and args[args.index('-U') + 1] == 'auth_owner':
            return b'[]'
        return b'f\n'

    def test_activator_catalog_sessions_run_from_postgres_and_name_only_the_catalog(self):
        self.db.catalog()
        self.db.clear_leftovers()
        self.db.other_superuser()
        self.db.exists('corpus_vn_20990101_1')
        self.db.connections('corpus_vn_20990101_1')
        self.db.owned('corpus_vn_20990101_1')
        self.assertTrue(self.seen)
        for args, text in self.seen:
            self.assertEqual(args[args.index('-d') + 1], 'postgres')
            option = next(arg for arg in args if arg.startswith('PGOPTIONS='))
            self.assertEqual(option, 'PGOPTIONS=-c search_path=pg_catalog')
            for word in ('public.', 'corpus_release', 'frameworks_release', 'release_journal', 'retired_release'):
                self.assertNotIn(word, text)

    def test_the_protected_set_reaches_the_sweep_as_literals(self):
        replies = iter([b'["corpus_vn_20990101_1"]'])
        original = self.runner
        def runner(args, **kwargs):
            if '-U' in args and args[args.index('-U') + 1] == 'auth_owner':
                return next(replies)
            return original(args, **kwargs)
        self.host.runner = runner
        self.db.clear_leftovers()
        sweep = next(text for _, text in self.seen if 'DROP DATABASE' in text)
        self.assertIn("ARRAY['corpus_vn_20990101_1']::pg_catalog.text[]", sweep)

    def test_lock_session_turns_the_idle_timeout_off_and_keeps_the_statement(self):
        process = mock.Mock()
        process.poll.return_value = None
        selector = mock.MagicMock()
        selector.__enter__.return_value.select.return_value = [(None, None)]
        with mock.patch('updater.deploy_corpus_lock.subprocess.Popen', return_value=process) as start, \
             mock.patch('updater.deploy_corpus_lock.selectors.DefaultSelector', return_value=selector), \
             mock.patch('updater.deploy_corpus_lock.os.read', return_value=b't|42\n'):
            with AdvisoryLock(self.db):
                pass
        args = start.call_args.args[0]
        self.assertIn('PGOPTIONS=-c search_path=pg_catalog -c idle_session_timeout=0', args)
        self.assertEqual(args[args.index('-d') + 1], 'auth')
        self.assertEqual(process.stdin.write.call_args.args[0],
                         b"SELECT pg_catalog.pg_try_advisory_lock(pg_catalog.hashtextextended('activation',0)),"
                         b"pg_catalog.pg_backend_pid();\n")


if __name__ == '__main__':
    unittest.main()
