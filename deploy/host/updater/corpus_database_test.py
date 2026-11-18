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
        self.db.backend = 77
        self.m = manifest()
        self.row = dict(operation_id='e'*32, country='vn', rel=self.m['rel'], mode='new',
                        database_name=self.m['database'], manifest_hash='a'*64,
                        dump_sha256=self.m['dump']['sha256'], expected_active_db='corpus_vn_20990101_1',
                        manifest=self.m, step='checked', oid=16400)

    def test_catalog_read_loads_the_password_only_inside_the_container(self):
        self.response = b'{"corpus_vn_20990101_1":{"oid":41001,"owner":"activator"}}'
        self.assertEqual(self.db.catalog(), {'corpus_vn_20990101_1': dict(oid=41001, owner='activator')})
        args, kwargs = self.calls[0]
        self.assertIn('activator', args)
        self.assertIn('PGPASSWORD=$(cat /run/secrets/activator_db_password)', ' '.join(args))
        self.assertNotIn('postgres-admin.sh', ' '.join(args))
        self.assertIn('pg_catalog.pg_database', kwargs['input'].decode())

    def test_catalog_read_refuses_anything_but_an_object(self):
        self.response = b'[]'
        with self.assertRaises(ValueError):
            self.db.catalog()

    def test_cas_is_one_transaction_with_journal_and_retirement_of_the_recorded_database(self):
        self.response = json.dumps(dict(self.row, step='activated', state='complete', message='corpus activated',
                                        corpus_kind='law')).encode()
        statement = d.LAW.activation(self.db, self.row, ('corpus_vn_20990101_1', 'b' * 64))
        self.assertEqual(self.db.tables.swap(statement, self.row)['step'], 'activated')
        sql = self.calls[-1][1]['input'].decode()
        for word in ('BEGIN;', 'FOR UPDATE', 'IS DISTINCT FROM', 'public.retired_release',
                     "interval '7 days'", 'dump_sha256', 'public.release_journal', "'activated'", 'COMMIT;'):
            self.assertIn(word, sql)
        self.assertEqual(sql.count('BEGIN;'), 1)
        self.assertIn('auth_owner', self.calls[-1][0])
        self.assertIn('default_transaction_read_only=off', ' '.join(self.calls[-1][0]))

    def test_cas_checks_pointer_and_update_count_before_retirement(self):
        for mode in ('new', 'retained'):
            sql = d.LAW.activation(self.db, dict(self.row, mode=mode), ('corpus_vn_20990101_1', None))
            with self.subTest(mode=mode):
                self.assertTrue('IF NOT FOUND THEN' in sql, 'missing pointer must raise')
                self.assertRegex(sql, r'GET DIAGNOSTICS \w+ = ROW_COUNT;')
                self.assertRegex(sql, r'IF \w+ <> 1 THEN')
                self.assertLess(sql.index('IF NOT FOUND THEN'), sql.index('UPDATE public.corpus_release'))
                self.assertLess(sql.index('UPDATE public.corpus_release'), sql.index('GET DIAGNOSTICS'))
                self.assertLess(sql.index('GET DIAGNOSTICS'), sql.index('INSERT INTO public.retired_release'))
                if mode == 'retained':
                    self.assertLess(sql.index('GET DIAGNOSTICS'), sql.index('DELETE FROM public.retired_release'))

    def test_protected_names_combine_the_tables_and_the_host_records(self):
        self.response = b'["corpus_vn_20990101_1"]'
        self.db.catalog = mock.Mock(return_value={})
        self.db.guard = mock.Mock()
        self.db.guard.names.return_value = {'corpus_vn_20990103_1'}
        self.assertEqual(self.db.protected_names('e' * 32, {'x'}), {'corpus_vn_20990101_1', 'corpus_vn_20990103_1'})
        self.db.guard.names.assert_called_once_with({}, own='e' * 32, pruning={'x'})

    def test_dead_advisory_lock_refuses_catalog_reads(self):
        self.db.holder.poll.return_value = 1
        with self.assertRaises(ValueError):
            self.db.catalog()
        self.assertEqual(self.calls, [])

    def test_a_drop_runs_as_activator_in_auth_bound_to_the_recorded_oid(self):
        self.response = b'f\n'
        self.db.drop_database(self.m['database'], 16400)
        args, kwargs = self.calls[0]
        self.assertEqual(args[args.index('-U') + 1], 'activator')
        self.assertEqual(args[args.index('-d') + 1], 'auth')
        text = kwargs['input'].decode()
        self.assertIn('d.oid=16400::pg_catalog.oid', text)
        self.assertNotIn('OWNER TO', text)
        self.assertFalse(any('postgres-admin.sh' in ' '.join(call) for call, _ in self.calls))

    def test_large_stdin_file_is_passed_without_reading_into_memory(self):
        path = Path(self.work.name)/'input'
        path.write_bytes(b'x'*10000)
        import sys
        raw = command([sys.executable, '-c', 'import sys; print(len(sys.stdin.buffer.read()))'], input_file=path)
        self.assertEqual(raw, b'10000\n')

    def test_restore_uses_transaction_toc_and_manifest_extension_versions(self):
        self.response = b'f\n'
        with mock.patch.object(self.db, 'toc', return_value='/run/pgbackrest/test.list'), \
             mock.patch.object(self.db, 'check_catalog'):
            self.db.restore(self.row, '/synthetic/dump')
        args, kwargs = next((args, kwargs) for args, kwargs in self.calls if 'pg_restore' in args)
        for value in ('--single-transaction', '--no-owner', '--no-privileges', '--exit-on-error', '--use-list'):
            self.assertIn(value, args)
        self.assertEqual(kwargs['input_file'], '/synthetic/dump')
        self.assertIn('maintenance_work_mem=2GB', ' '.join(args))

    def test_toc_lists_are_removed_per_operation_and_by_prefix(self):
        self.db.remove_list('e' * 32)
        self.db.discard_lists()
        self.assertEqual([args for args, _ in self.calls[-2:]], [
            ['podman', 'exec', 'trachuan-postgres', 'rm', '-f', '/run/pgbackrest/corpus-' + 'e' * 32 + '.list'],
            ['podman', 'exec', 'trachuan-postgres', 'sh', '-c', 'rm -f /run/pgbackrest/corpus-*.list']])
        with self.assertRaises(ValueError):
            self.db.remove_list('../../secrets')

    def test_available_extensions_must_equal_manifest_versions(self):
        self.response = b'{"vector":"wrong","ltree":"1.3"}'
        with self.assertRaises(ValueError):
            self.db.check_extensions(self.m)


if __name__ == '__main__':
    unittest.main()
