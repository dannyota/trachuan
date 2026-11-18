import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock

from updater import deploy_corpus_sql as law
from updater import deploy_frameworks_sql as frameworks
from updater.corpus_verify_test import manifest as law_manifest
from updater.deploy_corpus_database import Database
from updater.deploy_corpus_kind import FRAMEWORKS, LAW
from updater.deploy_host import Host
from updater.deploy_store import Paths
from updater.frameworks_verify_test import manifest


def frameworks_row(mode='new', expected='frameworks_20990101'):
    m = manifest()
    return dict(operation_id='e' * 32, country='vn', rel=m['rel'], mode=mode, database_name=m['database'],
                manifest_hash='a' * 64, dump_sha256=m['dump']['sha256'], expected_active_db=expected,
                manifest=m, step='checked')


def law_row(mode='new'):
    m = law_manifest()
    return dict(operation_id='e' * 32, country='vn', rel=m['rel'], mode=mode, database_name=m['database'],
                manifest_hash='a' * 64, dump_sha256=m['dump']['sha256'], expected_active_db='corpus_vn_20990101_1',
                manifest=m, step='checked')


class FrameworksSqlTest(unittest.TestCase):
    def test_frameworks_journal_rows_carry_kind_and_law_inserts_stay_unchanged(self):
        sql = frameworks.journal(frameworks_row(), 'started')
        self.assertIn(',corpus_kind)', sql)
        self.assertIn("'frameworks started','frameworks')", sql)
        self.assertNotIn('corpus_kind', law.journal(law_row(), 'started'))
        self.assertIn("'corpus started')", law.journal(law_row(), 'started'))

    def test_pointer_swap_or_bootstrap_insert_in_one_transaction(self):
        for mode, expected in (('new', 'frameworks_20990101'), ('new', None), ('retained', 'frameworks_20990101')):
            sql = frameworks.activate(frameworks_row(mode, expected))
            with self.subTest(mode=mode, expected=expected):
                self.assertEqual(sql.count('BEGIN;'), 1)
                self.assertTrue(sql.rstrip().endswith('COMMIT;'))
                self.assertNotIn('corpus_release', sql)
                order = ['FROM frameworks_release WHERE country=\'vn\' FOR UPDATE', 'IS DISTINCT FROM',
                         'INSERT INTO frameworks_release', 'UPDATE frameworks_release', 'GET DIAGNOSTICS',
                         'IF changed <> 1 THEN', 'INSERT INTO retired_release', 'INSERT INTO release_journal']
                positions = [sql.index(text) for text in order]
                self.assertEqual(positions, sorted(positions))
                self.assertIn("ON CONFLICT (country) DO NOTHING", sql)
                self.assertIn("'frameworks')\n      ON CONFLICT (country,db_name)", sql)
                self.assertIn("corpus_kind='frameworks' AND country='vn' AND database_name=previous", sql)
                self.assertIn("'frameworks activated','frameworks')", sql)
                self.assertEqual("DELETE FROM retired_release WHERE corpus_kind='frameworks'" in sql, mode == 'retained')
        self.assertIn('IS DISTINCT FROM NULL', frameworks.activate(frameworks_row('new', None)))

    def test_frameworks_reads_filter_kind_and_never_touch_corpus_release(self):
        name = 'frameworks_20990101'
        statements = [frameworks.current('vn'), frameworks.retirement(name), frameworks.operations(),
                      frameworks.expired(), frameworks.forget(name), frameworks.protected(name)]
        for sql in statements:
            self.assertNotIn('corpus_release', sql)
        for sql in statements[1:]:
            self.assertIn("corpus_kind='frameworks'", sql)
        self.assertIn('FROM frameworks_release', frameworks.current('vn'))
        self.assertIn('frameworks_release WHERE active_db=', frameworks.protected(name))

    def test_law_reads_and_writes_filter_law_kind_and_never_touch_frameworks_release(self):
        name = 'corpus_vn_20990101_1'
        for sql in (law.retirement(name), law.operations(), law.expired(), law.forget(name)):
            with self.subTest(sql=sql):
                self.assertIn("corpus_kind='law'", sql)
        protection = law.protected(name)
        self.assertEqual(protection.count("corpus_kind='law'"), 3)
        for mode in ('new', 'retained'):
            sql = law.activate(law_row(mode))
            self.assertIn("corpus_kind='law' AND country='vn' AND database_name=previous", sql)
            self.assertEqual("DELETE FROM retired_release WHERE corpus_kind='law'" in sql, mode == 'retained')
            self.assertNotIn('frameworks_release', sql)
            self.assertNotIn('frameworks', sql.replace('corpus_kind', ''))
        self.assertNotIn('frameworks_release', protection)

    def test_names_are_validated_by_kind_prefix(self):
        self.assertEqual(frameworks.database('frameworks_20990101_1'), '"frameworks_20990101_1"')
        for bad in ('corpus_vn_20990101', 'frameworks_2099010', 'frameworks_20990101_123', 'frameworks_20990101;'):
            with self.subTest(bad=bad), self.assertRaises(ValueError):
                frameworks.database(bad)
        with self.assertRaises(ValueError):
            law.database('frameworks_20990101')


class FrameworksDatabaseTest(unittest.TestCase):
    def setUp(self):
        self.work = tempfile.TemporaryDirectory()
        self.addCleanup(self.work.cleanup)
        self.calls, self.response = [], b''
        def runner(args, **kwargs):
            self.calls.append((args, kwargs))
            return self.response
        self.host = Host(Paths(Path(self.work.name)), runner, user_prefix=[])

    def database(self, kind):
        db = Database(self.host, {'extensions': ['vector', 'ltree']}, kind)
        db.holder = mock.Mock()
        db.holder.poll.return_value = None
        return db

    def sql(self):
        return self.calls[-1][1]['input'].decode()

    def test_frameworks_database_queries_its_pointer_and_kind_rows(self):
        db = self.database(FRAMEWORKS)
        self.response = b'frameworks_20990101\n'
        self.assertEqual(db.current('vn'), 'frameworks_20990101')
        self.assertIn('FROM frameworks_release', self.sql())
        self.response = b'corpus_vn_20990101\n'
        with self.assertRaises(ValueError):
            db.current('vn')
        self.response = b'\n'
        self.assertIsNone(db.current('vn'))
        self.response = b'[]'
        db.operations()
        self.assertIn("corpus_kind='frameworks'", self.sql())
        self.response = json.dumps(dict(frameworks_row(), step='started', corpus_kind='frameworks')).encode()
        db.append(frameworks_row(), 'started')
        self.assertIn("'frameworks')", self.sql())
        self.response = json.dumps(dict(frameworks_row(), step='activated')).encode()
        db.targets().write(frameworks_row()['operation_id'], frameworks_row()['database_name'], '16400')
        db.activate(frameworks_row())
        self.assertIn('frameworks_release', self.sql())
        self.assertNotIn('corpus_release', self.sql())
        for method in (db.exists, db.connections, db.protected, db.retirement):
            with self.subTest(method=method.__name__), self.assertRaises(ValueError):
                method('corpus_vn_20990101')

    def test_law_database_refuses_frameworks_names_and_filters_law(self):
        db = self.database(LAW)
        for method in (db.exists, db.connections, db.protected, db.retirement, db.drop):
            with self.subTest(method=method.__name__), self.assertRaises(ValueError):
                method('frameworks_20990101')
        self.response = b'[]'
        db.expired()
        self.assertIn("WHERE corpus_kind='law' AND retain_until <= now()", self.sql())


if __name__ == '__main__':
    unittest.main()
