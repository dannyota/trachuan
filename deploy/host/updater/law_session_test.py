import unittest

from updater import deploy_corpus_sql as sql
from updater.frameworks_check_test import UNQUALIFIED
from updater.law_restore_test import LawFixture


class LawSmokeTest(LawFixture, unittest.TestCase):
    def test_prewarm_runs_as_activator_with_every_name_qualified(self):
        self.db.smoke(self.row, 'server-image')
        self.assertEqual(self.names(), ['check', 'prewarm', 'verify', 'release-check', 'verify'])
        self.assertIn("'relation corpus_reader SELECT'", self.sql('check'))
        self.assert_activator(self.call('prewarm')[0], self.name)
        text = self.sql('prewarm')
        self.assertIn('public.pg_prewarm(c.oid::pg_catalog.regclass)', text)
        self.assertIn("pg_catalog.has_table_privilege(c.oid,'SELECT')", text)
        for setting in ('statement_timeout', 'lock_timeout', 'idle_in_transaction_session_timeout', 'transaction_timeout'):
            self.assertIn('SET ' + setting + '=0;', text)
        for word in ('CREATE EXTENSION', 'DROP EXTENSION'):
            self.assertNotIn(word, text)
        self.assertEqual(UNQUALIFIED.findall(text), [])

    def test_database_without_pg_prewarm_is_skipped_and_still_checked(self):
        self.responses['prewarm'] = b'skipped\n'
        self.db.smoke(self.row, 'server-image')
        self.assertEqual(self.names(), ['check', 'prewarm', 'verify', 'release-check', 'verify'])

    def test_a_new_operation_with_a_failing_check_stops_before_the_release_check(self):
        self.responses['check'] = b'trigger\n'
        with self.assertRaisesRegex(ValueError, '^corpus database trigger differs'):
            self.db.smoke(self.row, 'server-image')
        self.assertEqual(self.names(), ['check'])

    def test_retained_database_is_checked_before_it_becomes_active_again(self):
        self.db.smoke(dict(self.row, mode='retained'), 'server-image')
        self.assertEqual(self.names(), ['check', 'prewarm', 'release-check'])
        self.assert_activator(self.call('check')[0], self.name)
        self.assertNotIn('release_target', self.sql('check'))
        self.assertIn("('extension','vector',NULL)", self.sql('check'))
        self.calls.clear()
        self.responses['check'] = b'function\n'
        with self.assertRaisesRegex(ValueError, '^corpus database function differs'):
            self.db.smoke(dict(self.row, mode='retained'), 'server-image')
        self.assertEqual(self.names(), ['check'])


class LawSessionTest(LawFixture, unittest.TestCase):
    def test_no_postgres_session_once_the_restore_starts_and_every_session_pins_search_path(self):
        self.db.inspect(self.row, '/synthetic/dump')
        self.db.create(self.row)
        self.db.restore(self.row, '/synthetic/dump')
        self.db.validate(self.row)
        self.db.smoke(self.row, 'server-image')
        names = self.names()
        start = names.index('pg_restore')
        self.assertEqual(names[start:], ['pg_restore', 'remove-list', 'verify', 'check', 'grants', 'validate', 'check',
                                         'prewarm', 'verify', 'release-check', 'verify'])
        self.assertLess(names.index('prepare'), start)
        for _, args, _ in self.calls[start:]:
            self.assertNotIn('postgres-admin.sh', ' '.join(args))
            self.assertNotIn('grant-corpus.sh', ' '.join(args))
        for name, args, _ in self.calls:
            if name in ('list', 'write-list', 'remove-list', 'release-check'):
                continue
            with self.subTest(name=name):
                self.assertIn('search_path=pg_catalog', self.options(args) if name != 'prepare' else args[4])

    def test_superuser_check_from_auth_excludes_only_postgres(self):
        self.responses['exists'] = b''
        self.responses['superuser'] = b't\n'
        self.assertTrue(self.db.other_superuser())
        args, kwargs = next((args, kwargs) for _, args, kwargs in self.calls)
        self.assert_activator(args, 'auth', 'pg_catalog,public')
        text = kwargs['input'].decode()
        self.assertIn("r.rolname <> 'postgres'", text)
        self.assertNotIn('current_user', text)

    def test_auth_sessions_pin_catalog_then_public(self):
        self.db.exists(self.name)
        self.assert_activator(self.call('exists')[0], 'auth', 'pg_catalog,public')

    def test_database_names_are_refused_before_any_session(self):
        for action in (lambda row: self.db.create(row), lambda row: self.db.restore(row, '/synthetic/dump')):
            with self.assertRaisesRegex(ValueError, 'invalid corpus database name'):
                action(dict(self.row, database_name='frameworks_20990102'))
        self.assertEqual(self.calls, [])

    def test_grants_refuse_a_reader_that_is_not_a_plain_name(self):
        with self.assertRaises(ValueError):
            sql.grants(self.name, 'reader"; DROP')


if __name__ == '__main__':
    unittest.main()
