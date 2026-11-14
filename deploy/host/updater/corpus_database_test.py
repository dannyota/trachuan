import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock

from updater.corpus_verify_test import manifest
from updater import deploy_corpus_database as d
from updater.deploy_host import Host
from updater.deploy_store import Paths
from updater.deploy_io import command


class CorpusDatabaseTest(unittest.TestCase):
    def setUp(self):
        self.work = tempfile.TemporaryDirectory()
        self.addCleanup(self.work.cleanup)
        self.paths = Paths(Path(self.work.name))
        self.calls = []
        self.response = b'[]'
        def runner(args, **kwargs):
            self.calls.append((args, kwargs))
            return self.response
        self.db = d.Database(Host(self.paths, runner, user_prefix=[]), {'extensions':['vector','ltree']})
        self.db.holder = mock.Mock()
        self.db.holder.poll.return_value = None
        self.m = manifest()
        self.row = dict(operation_id='e'*32, country='vn', rel=self.m['rel'], mode='new',
                        database_name=self.m['database'], manifest_hash='a'*64,
                        dump_sha256=self.m['dump']['sha256'], expected_active_db='corpus_vn_20990101_1',
                        manifest=self.m, step='checked')

    def test_unprivileged_sql_loads_password_only_inside_container(self):
        self.response = b'corpus_vn_20990101_1'
        self.db.current('vn')
        args, kwargs = self.calls[0]
        self.assertIn('activator', args)
        self.assertIn('PGPASSWORD=$(cat /run/secrets/activator_db_password)', ' '.join(args))
        self.assertNotIn('postgres-admin.sh', ' '.join(args))
        self.assertIn('SELECT active_db', kwargs['input'].decode())

    def test_cas_is_one_transaction_with_journal_and_nullable_old_digest(self):
        self.response = json.dumps(dict(self.row, step='activated')).encode()
        self.db.activate(self.row)
        sql = self.calls[-1][1]['input'].decode()
        for word in ('BEGIN;', 'FOR UPDATE', 'IS DISTINCT FROM', 'retired_release',
                     "interval '7 days'", 'dump_sha256', 'release_journal', "'activated'", 'COMMIT;'):
            self.assertIn(word, sql)
        self.assertEqual(sql.count('BEGIN;'), 1)

    def test_cas_checks_pointer_and_update_count_before_retirement(self):
        for mode in ('new', 'retained'):
            self.response = json.dumps(dict(self.row, step='activated')).encode()
            self.db.activate(dict(self.row, mode=mode))
            sql = self.calls[-1][1]['input'].decode()
            with self.subTest(mode=mode):
                self.assertTrue('IF NOT FOUND THEN' in sql, 'missing pointer must raise')
                self.assertRegex(sql, r'GET DIAGNOSTICS \w+ = ROW_COUNT;')
                self.assertRegex(sql, r'IF \w+ <> 1 THEN')
                self.assertLess(sql.index('IF NOT FOUND THEN'), sql.index('UPDATE corpus_release'))
                self.assertLess(sql.index('UPDATE corpus_release'), sql.index('GET DIAGNOSTICS'))
                self.assertLess(sql.index('GET DIAGNOSTICS'), sql.index('INSERT INTO retired_release'))
                if mode == 'retained':
                    self.assertLess(sql.index('GET DIAGNOSTICS'), sql.index('DELETE FROM retired_release'))

    def test_protection_query_covers_active_open_journals_and_retention(self):
        self.response = b't\n'
        self.assertTrue(self.db.protected(self.m['database']))
        sql = self.calls[-1][1]['input'].decode()
        for text in ('active_db', 'operation_id', 'retain_until > now()', "'activated'", "'reverted'", "'aborted'"):
            self.assertIn(text, sql)
        self.calls.clear()
        with self.assertRaises(ValueError):
            self.db.drop(self.m['database'])
        self.assertFalse(any('dropdb' in args for args, _ in self.calls))

    def test_dead_advisory_lock_refuses_mutations(self):
        self.db.holder.poll.return_value = 1
        with self.assertRaises(ValueError):
            self.db.append(self.row, 'checked')
        self.assertEqual(self.calls, [])

    def test_drop_prepares_legacy_postgres_owned_database_as_activator(self):
        self.response = b'f\n'
        self.db.drop(self.m['database'])
        sql = [kwargs['input'].decode() for args, kwargs in self.calls if 'postgres-admin.sh' in ' '.join(args)]
        self.assertTrue(any('OWNER TO activator' in text for text in sql))
        self.assertIn('activator', self.calls[-1][0])
        self.assertIn('dropdb', self.calls[-1][0])

    def test_large_stdin_file_is_passed_without_reading_into_memory(self):
        path = Path(self.work.name)/'input'
        path.write_bytes(b'x'*10000)
        import sys
        raw = command([sys.executable, '-c', 'import sys; print(len(sys.stdin.buffer.read()))'], input_file=path)
        self.assertEqual(raw, b'10000\n')

    def test_restore_uses_transaction_toc_and_manifest_extension_versions(self):
        self.response = b'f\n'
        with mock.patch.object(self.db, 'toc', return_value='/run/pgbackrest/test.list'):
            self.db.restore(self.row, '/synthetic/dump')
        args, kwargs = next((args, kwargs) for args, kwargs in self.calls if 'pg_restore' in args)
        for value in ('--single-transaction', '--no-owner', '--no-privileges', '--exit-on-error', '--use-list'):
            self.assertIn(value, args)
        self.assertEqual(kwargs['input_file'], '/synthetic/dump')
        self.assertIn('maintenance_work_mem=2GB', ' '.join(args))

    def test_available_extensions_must_equal_manifest_versions(self):
        self.response = b'{"vector":"wrong","ltree":"1.3"}'
        with self.assertRaises(ValueError):
            self.db.check_extensions(self.m)
