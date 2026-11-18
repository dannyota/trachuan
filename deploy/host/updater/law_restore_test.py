import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock

from updater import deploy_corpus_sql as sql
from updater.corpus_verify_test import manifest
from updater.deploy_corpus_database import Database
from updater.deploy_host import Host
from updater.deploy_io import CommandError
from updater.deploy_release_catalog import KINDS
from updater.deploy_store import Paths
from updater.frameworks_check_test import UNQUALIFIED
from updater.frameworks_toc_test import DATA, HEADER


ADMIN = 'PGOPTIONS=-c search_path=pg_catalog'
OPERATION = 'e' * 32


def classify(args, text):
    joined = ' '.join(args)
    if 'release-check' in args:
        return 'release-check'
    if 'grant-corpus.sh' in joined:
        return 'grant-corpus'
    for program in ('createdb', 'dropdb'):
        if program in args:
            return program
    if 'pg_restore' in args:
        return 'list' if '--list' in args else 'pg_restore'
    if 'umask 077; cat > "$1"' in args:
        return 'write-list'
    if 'rm' in args:
        return 'remove-list'
    if '/usr/local/bin/postgres-admin.sh' in args:
        return 'prepare' if 'CREATE EXTENSION' in text else 'postgres'
    for word, name in (('pg_available_extensions', 'extensions'), ('datdba', 'owner'),
                       ('EXISTS(SELECT FROM pg_catalog.pg_database', 'exists'),
                       ('corpus_release WHERE active_db', 'protected'), ('pg_catalog.pg_event_trigger', 'check'),
                       ("to_regclass('public.release')", 'restored'), ('GRANT SELECT ON ALL TABLES', 'grants'),
                       ('FROM public.release WHERE singleton', 'validate'), ('public.pg_prewarm', 'prewarm'),
                       ('document_summary', 'projection')):
        if word in text:
            return name
    raise AssertionError('unexpected command')


class LawFixture:
    def setUp(self):
        self.work = tempfile.TemporaryDirectory()
        self.addCleanup(self.work.cleanup)
        self.m = manifest()
        self.name = self.m['database']
        self.calls, self.failures = [], {}
        release = {key: self.m[key] for key in ('country', 'rel', 'schema_version', 'recipes')}
        self.responses = dict(extensions=json.dumps(self.m['extensions']).encode(), exists=b'f\n', protected=b'f\n',
                              owner=b'activator\n', prepare=b'configured\n', restored=b'f\n',
                              list=(HEADER + DATA).encode(), check=b'ok\n', prewarm=b'12\n',
                              validate=json.dumps(release).encode(), **{'release-check': b'{}\n'})
        def runner(args, **kwargs):
            name = classify(args, (kwargs.get('input') or b'').decode())
            self.calls.append((name, args, kwargs))
            if name in self.failures:
                raise self.failures[name]
            return self.responses.get(name, b'')
        self.db = Database(Host(Paths(Path(self.work.name)), runner, user_prefix=[]), {'extensions': ['vector', 'ltree']})
        self.db.holder = mock.Mock()
        self.db.holder.poll.return_value = None
        self.row = dict(operation_id=OPERATION, country='vn', rel=self.m['rel'], mode='new', database_name=self.name,
                        manifest_hash='a' * 64, dump_sha256=self.m['dump']['sha256'],
                        expected_active_db='corpus_vn_20990101_1', manifest=self.m, step='restoring')

    def names(self):
        return [name for name, _, _ in self.calls]

    def call(self, name):
        return next((args, kwargs) for found, args, kwargs in self.calls if found == name)

    def sql(self, name):
        return self.call(name)[1]['input'].decode()

    def options(self, args):
        return next(arg for arg in args if arg.startswith('PGOPTIONS='))

    def assert_activator(self, args, database, path='pg_catalog'):
        self.assertIn('PGPASSWORD=$(cat /run/secrets/activator_db_password)', ' '.join(args))
        self.assertEqual(args[args.index('-U') + 1], 'activator')
        self.assertEqual(args[args.index('-d') + 1] if '-d' in args else args[args.index('--dbname') + 1], database)
        self.assertNotIn('postgres-admin.sh', ' '.join(args))
        self.assertTrue(self.options(args).endswith('-c search_path=' + path))


class LawCreateTest(LawFixture, unittest.TestCase):
    def test_create_as_activator_then_one_postgres_step_before_any_dump_byte(self):
        self.db.create(self.row)
        self.assertEqual(self.names(), ['extensions', 'exists', 'createdb', 'owner', 'prepare'])
        args, _ = self.call('createdb')
        self.assertIn('--template=template0', args)
        self.assertEqual(args[-1], self.name)
        self.assertEqual(args[args.index('-U') + 1], 'activator')
        self.assertEqual(self.options(args), ADMIN)
        args, _ = self.call('prepare')
        self.assertEqual(args[:6], ['podman', 'exec', '-i', '--env', ADMIN, 'trachuan-postgres'])
        self.assertEqual(args[6:], ['/usr/local/bin/postgres-admin.sh', 'psql', '-XqAt', '-h', '127.0.0.1', '-U',
                                    'postgres', '-d', self.name, '-v', 'ON_ERROR_STOP=1'])
        text = self.sql('prepare')
        for statement in ("CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public VERSION '0.8.1';",
                          "CREATE EXTENSION IF NOT EXISTS ltree WITH SCHEMA public VERSION '1.3';",
                          'CREATE EXTENSION IF NOT EXISTS pg_prewarm WITH SCHEMA public;',
                          'REVOKE ALL ON DATABASE "' + self.name + '" FROM PUBLIC;'):
            self.assertIn(statement + '\n', text)
        for word in ('pg_catalog.pg_roles', 'rolsuper', "pg_catalog.pg_has_role(r.oid,s.oid,'MEMBER')"):
            self.assertIn(word, text)
        self.assertLess(text.index('rolsuper'), text.index('\\if'))
        self.assertLess(text.index('\\if'), text.index('CREATE EXTENSION'))
        self.assertNotIn('GRANT', text)
        self.assertEqual(UNQUALIFIED.findall(sql.SUPERUSER_CHECK), [])

    def test_another_superuser_or_unexpected_outcome_refuses(self):
        self.responses['prepare'] = b'superuser\n'
        with self.assertRaisesRegex(ValueError, '^a role other than postgres is a superuser or a member of one$'):
            self.db.create(self.row)
        self.responses['prepare'] = b'\n'
        with self.assertRaisesRegex(ValueError, '^corpus database not prepared$'):
            self.db.create(self.row)

    def test_existing_target_is_dropped_and_created_again(self):
        self.responses['exists'] = b't\n'
        self.db.create(self.row)
        self.assertEqual(self.names(), ['extensions', 'exists', 'owner', 'protected', 'dropdb', 'createdb', 'owner',
                                        'prepare'])
        self.assertIn("operation_id <> '" + OPERATION + "'", self.sql('protected'))
        args, _ = self.call('dropdb')
        self.assertEqual(args[args.index('-U') + 1], 'activator')
        self.assertEqual(args[-2:], ['--if-exists', self.name])
        self.assertEqual(self.options(args), ADMIN)

    def test_existing_target_activator_does_not_own_is_refused_without_a_postgres_session(self):
        self.responses.update(exists=b't\n', owner=b'postgres\n')
        with self.assertRaisesRegex(ValueError, '^activation target not owned by activator$'):
            self.db.create(self.row)
        self.assertEqual(self.names(), ['extensions', 'exists', 'owner'])

    def test_dropping_an_owned_target_opens_no_postgres_session(self):
        self.db.drop_owned(self.name)
        self.assertEqual(self.names(), ['protected', 'dropdb'])

    def test_protected_existing_target_is_refused_and_kept(self):
        self.responses.update(exists=b't\n', protected=b't\n')
        with self.assertRaisesRegex(ValueError, 'refusing to drop protected database'):
            self.db.create(self.row)
        for name in ('dropdb', 'createdb', 'prepare'):
            self.assertNotIn(name, self.names())

    def test_target_owned_by_another_role_refuses_before_the_postgres_step(self):
        self.responses['owner'] = b'postgres\n'
        with self.assertRaisesRegex(ValueError, 'not owned by activator'):
            self.db.create(self.row)
        self.assertNotIn('prepare', self.names())

    def test_dead_advisory_lock_refuses_the_postgres_step(self):
        self.db.holder.poll.return_value = 1
        with self.assertRaisesRegex(ValueError, 'activation advisory lock lost'):
            self.db.admin(self.name, 'SELECT 1;')
        self.assertEqual(self.calls, [])


class LawRestoreTest(LawFixture, unittest.TestCase):
    def restore(self):
        self.db.restore(self.row, '/synthetic/dump')

    def test_restore_pins_search_path_then_checks_then_owner_grants(self):
        self.restore()
        self.assertEqual(self.names(), ['restored', 'list', 'write-list', 'pg_restore', 'remove-list', 'check', 'grants'])
        self.assert_activator(self.call('restored')[0], self.name)
        self.assertIn("pg_catalog.to_regclass('public.release')", self.sql('restored'))
        args, kwargs = self.call('pg_restore')
        self.assert_activator(args, self.name)
        self.assertEqual(kwargs['input_file'], '/synthetic/dump')
        for value in ('--no-owner', '--no-privileges', '--exit-on-error', '--single-transaction', '--use-list'):
            self.assertIn(value, args)
        for value in ('maintenance_work_mem=2GB', 'max_parallel_maintenance_workers=2', 'statement_timeout=0',
                      'lock_timeout=0', 'idle_in_transaction_session_timeout=0', 'transaction_timeout=0'):
            self.assertIn('-c ' + value, self.options(args))
        self.assert_activator(self.call('check')[0], self.name)
        args, _ = self.call('grants')
        self.assert_activator(args, self.name, 'pg_catalog,public')
        target = '"' + self.name + '"'
        text = self.sql('grants')
        for statement in ('REVOKE ALL ON DATABASE ' + target + ' FROM PUBLIC;',
                          'GRANT CONNECT ON DATABASE ' + target + ' TO "corpus_reader";',
                          'REVOKE CREATE ON SCHEMA public FROM PUBLIC;', 'GRANT USAGE ON SCHEMA public TO "corpus_reader";',
                          'GRANT SELECT ON ALL TABLES IN SCHEMA public TO "corpus_reader";',
                          'GRANT SELECT ON ALL SEQUENCES IN SCHEMA public TO "corpus_reader";'):
            self.assertIn(statement + '\n', text)
        self.assertNotIn('grant-corpus', self.names())

    def test_difference_fails_with_the_kind_only_and_before_the_reader_gets_access(self):
        for outcome, kind in [(kind.encode() + b'\n', kind) for kind in KINDS] + [(b'invented secret\n', 'object'), (b'\n', 'object')]:
            self.calls.clear()
            self.responses['check'] = outcome
            with self.subTest(outcome=outcome), self.assertRaises(ValueError) as caught:
                self.restore()
            self.assertEqual(str(caught.exception), 'corpus database ' + kind + ' differs from what a release may hold')
            self.assertNotIn('grants', self.names())

    def test_resumed_restore_repeats_the_check_and_grants_without_pg_restore(self):
        self.responses['restored'] = b't\n'
        self.restore()
        self.assertEqual(self.names(), ['restored', 'check', 'grants'])

    def test_pg_restore_failure_removes_the_list_and_skips_the_check(self):
        self.failures['pg_restore'] = CommandError('command failed', detail='pg_restore: exit 1')
        with self.assertRaises(CommandError):
            self.restore()
        self.assertEqual(self.names()[-2:], ['pg_restore', 'remove-list'])

    def test_validate_and_projection_read_qualified_names_under_the_catalog_path(self):
        self.db.validate(self.row)
        self.assert_activator(self.call('validate')[0], self.name)
        text = self.sql('validate')
        self.assertIn('pg_catalog.json_build_object(', text)
        self.assertIn('FROM public.release WHERE singleton', text)
        self.assertEqual(UNQUALIFIED.findall(text), [])
        self.responses['projection'] = b't\n'
        self.assertEqual(self.db.projection(dict(self.row, manifest=dict(self.m, schema_version=8))), 1)
        for found, args, kwargs in self.calls:
            if found == 'projection':
                self.assert_activator(args, self.name)
                self.assertIn('public.document_summary', kwargs['input'].decode())


class LawSmokeTest(LawFixture, unittest.TestCase):
    def test_prewarm_runs_as_activator_with_every_name_qualified(self):
        self.db.smoke(self.row, 'server-image')
        self.assertEqual(self.names(), ['prewarm', 'release-check'])
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
        self.assertEqual(self.names(), ['prewarm', 'release-check'])

    def test_retained_database_is_checked_before_it_becomes_active_again(self):
        self.db.smoke(dict(self.row, mode='retained'), 'server-image')
        self.assertEqual(self.names(), ['check', 'prewarm', 'release-check'])
        self.assert_activator(self.call('check')[0], self.name)
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
        self.assertEqual(names[start:], ['pg_restore', 'remove-list', 'check', 'grants', 'validate', 'prewarm',
                                         'release-check'])
        self.assertLess(names.index('prepare'), start)
        for _, args, _ in self.calls[start:]:
            self.assertNotIn('postgres-admin.sh', ' '.join(args))
            self.assertNotIn('grant-corpus.sh', ' '.join(args))
        for name, args, _ in self.calls:
            if name in ('list', 'write-list', 'remove-list', 'release-check'):
                continue
            with self.subTest(name=name):
                self.assertIn('search_path=pg_catalog', self.options(args) if name != 'prepare' else args[4])

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
