import json
from pathlib import Path
import tempfile
import unittest


def target():
    from updater import deploy_release_target
    return deploy_release_target


class TargetRecordTest(unittest.TestCase):
    def setUp(self):
        self.work = tempfile.TemporaryDirectory()
        self.addCleanup(self.work.cleanup)
        self.directory = Path(self.work.name) / 'corpus-targets'
        self.operation = 'a' * 32

    def test_record_is_private_and_read_back_for_its_database_only(self):
        store = target().Targets(self.directory)
        self.assertIsNone(store.read(self.operation, 'corpus_vn_20990101'))
        store.write(self.operation, 'corpus_vn_20990101', '16400')
        self.assertEqual(store.read(self.operation, 'corpus_vn_20990101'), '16400')
        self.assertEqual(self.directory.stat().st_mode & 0o777, 0o700)
        self.assertEqual(store.path(self.operation).stat().st_mode & 0o777, 0o600)
        with self.assertRaisesRegex(ValueError, 'identity changed'):
            store.read(self.operation, 'corpus_vn_20990102')

    def test_malformed_records_and_values_are_refused(self):
        store = target().Targets(self.directory)
        store.write(self.operation, 'corpus_vn_20990101', '16400')
        store.path(self.operation).write_text(json.dumps({'database': 'corpus_vn_20990101', 'oid': 'x'}))
        with self.assertRaisesRegex(ValueError, 'identity changed'):
            store.read(self.operation, 'corpus_vn_20990101')
        with self.assertRaisesRegex(ValueError, 'identity changed'):
            store.write(self.operation, 'corpus_vn_20990101', '1; DROP')
        with self.assertRaisesRegex(ValueError, 'invalid activation operation id'):
            store.path('../../secret')

    def test_keep_removes_records_of_closed_operations(self):
        store = target().Targets(self.directory)
        store.keep({self.operation})
        other = 'b' * 32
        store.write(self.operation, 'corpus_vn_20990101', '1')
        store.write(other, 'corpus_vn_20990102', '2')
        store.keep({self.operation})
        self.assertTrue(store.path(self.operation).exists())
        self.assertFalse(store.path(other).exists())


class TargetSqlTest(unittest.TestCase):
    def test_guard_compares_the_connected_database_before_the_statements(self):
        sql = target().guard('16400', 'SELECT 1;')
        self.assertIn("d.oid::pg_catalog.text='16400' AS release_target", sql)
        self.assertIn('d.datname=pg_catalog.current_database()', sql)
        self.assertLess(sql.index('\\gset'), sql.index('\\if :release_target'))
        self.assertLess(sql.index('\\if :release_target'), sql.index('SELECT 1;'))
        self.assertLess(sql.index('SELECT 1;'), sql.index("SELECT '" + target().MISMATCH + "';"))
        self.assertTrue(sql.endswith('\\endif\n'))
        with self.assertRaises(ValueError):
            target().guard("1' OR true", 'SELECT 1;')

    def test_backend_pid_must_be_a_number(self):
        self.assertIn('pg_catalog.pg_backend_pid(),77)', target().end_backends(77))
        for value in ('77); DROP', None, True):
            with self.subTest(value=value), self.assertRaises(ValueError):
                target().end_backends(value)


if __name__ == '__main__':
    unittest.main()
