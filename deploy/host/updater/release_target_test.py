import unittest


def target():
    from updater import deploy_release_target
    return deploy_release_target


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

    def test_sweep_keeps_every_protected_name_as_a_literal(self):
        sql = target().strays({"corpus_vn_20990101_1", "it's"})
        self.assertIn("ARRAY['corpus_vn_20990101_1','it''s']::pg_catalog.text[]", sql)
        self.assertIn('pg_catalog.pg_get_userbyid(d.datdba)=current_user', sql)
        self.assertTrue(sql.rstrip().endswith(');'))
        self.assertEqual(sql.count("pg_catalog.format('DROP DATABASE %I',s.datname)"), 1)

    def test_sweep_with_nothing_protected_is_valid_sql(self):
        self.assertIn('ALL(ARRAY[]::pg_catalog.text[])', target().strays(set()))

    def test_a_drop_is_bound_to_the_name_owner_and_recorded_oid(self):
        sql = target().drop('corpus_vn_20990101_1', 41001)
        self.assertIn("d.datname='corpus_vn_20990101_1'", sql)
        self.assertIn('pg_catalog.pg_get_userbyid(d.datdba)=current_user', sql)
        self.assertIn('d.oid=41001::pg_catalog.oid', sql)
        self.assertNotIn('d.oid', target().drop('corpus_vn_20990101_1'))
        with self.assertRaises(ValueError):
            target().drop('corpus_vn_20990101_1', '1; DROP')

    def test_backend_pid_must_be_a_number(self):
        self.assertIn('pg_catalog.pg_backend_pid(),77)', target().end_backends(77))
        for value in ('77); DROP', None, True):
            with self.subTest(value=value), self.assertRaises(ValueError):
                target().end_backends(value)


if __name__ == '__main__':
    unittest.main()
