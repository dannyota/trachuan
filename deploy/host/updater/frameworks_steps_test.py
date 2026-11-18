import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock

from updater.deploy_corpus_database import Database
from updater import deploy_frameworks_sql as sql
from updater.deploy_corpus_kind import FRAMEWORKS
from updater.deploy_host import Host
from updater.deploy_io import CommandError
from updater.deploy_store import Paths
from updater.frameworks_toc_test import DATA, EXECUTABLE, HEADER, SPECIAL, executable_sql
from updater.frameworks_verify_test import manifest, rules


ADMIN = ('PGOPTIONS=-c log_min_messages=panic -c log_min_error_statement=panic '
         '-c log_min_duration_statement=-1 -c log_statement=none -c search_path=pg_catalog')
SETTINGS = ("log_min_messages = 'panic'", "log_min_error_statement = 'panic'",
            "log_min_duration_statement = '-1'", "log_statement = 'none'")
GUARD = tuple("pg_catalog.current_setting('" + name + "') OPERATOR(pg_catalog.<>) '" + value + "'"
              for name, value in (('log_min_messages', 'panic'), ('log_min_error_statement', 'panic'),
                                  ('log_min_duration_statement', '-1'), ('log_statement', 'none')))


def classify(args, text):
    joined = ' '.join(args)
    if 'frameworks-schema' in args:
        return 'schema'
    if 'release-check' in args:
        return 'release-check'
    if 'grant-corpus.sh' in joined:
        return 'grant-corpus'
    if 'rm -f /run/pgbackrest/corpus-*.list' in args:
        return 'discard-lists'
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
        return 'prewarm' if 'pg_prewarm' in text else 'configure' if 'ALTER DATABASE' in text else 'postgres'
    for word, name in (('pg_available_extensions', 'extensions'), ('datdba', 'owner'),
                       ('EXISTS(SELECT FROM pg_catalog.pg_database', 'exists'), ('frameworks_release WHERE active_db', 'protected'),
                       ("to_regclass('public.release')", 'restored'),
                       ('frameworks_before', 'check'), ('CREATE OR REPLACE', 'apply'),
                       ('FROM public.release WHERE singleton', 'validate'), ('GRANT SELECT ON ALL TABLES', 'grants'),
                       ('current_setting', 'effective')):
        if word in text:
            return name
    raise AssertionError('unexpected command')


class StepsFixture:
    def setUp(self):
        self.work = tempfile.TemporaryDirectory()
        self.addCleanup(self.work.cleanup)
        self.paths = Paths(Path(self.work.name))
        self.m = manifest()
        self.calls, self.failures = [], {}
        self.responses = dict(
            schema=executable_sql().encode(), extensions=json.dumps(self.m['extensions']).encode(),
            exists=b'f\n', owner=b'activator\n', configure=b'configured\n', effective=b'panic,panic,-1,none\n',
            restored=b'f\n', list=(HEADER + DATA + EXECUTABLE).encode(),
            validate=json.dumps(dict({key: self.m[key] for key in ('rel', 'schema_version', 'recipes', 'counts')},
                                     kind='frameworks')).encode(),
            check=b'ok\n',
            **{'release-check': b'{"status":"ok","search_hits":1,"read_items":1}\n'})
        def runner(args, **kwargs):
            name = classify(args, (kwargs.get('input') or b'').decode())
            self.calls.append((name, args, kwargs))
            if name in self.failures:
                raise self.failures[name]
            return self.responses.get(name, b'')
        self.db = Database(Host(self.paths, runner, user_prefix=[]), rules(), FRAMEWORKS)
        self.db.holder = mock.Mock()
        self.db.holder.poll.return_value = None
        self.steps = FRAMEWORKS.steps(self.db)
        self.name = self.m['database']
        self.row = dict(operation_id='e' * 32, country='vn', rel=self.m['rel'], mode='new', database_name=self.name,
                        manifest_hash='a' * 64, dump_sha256=self.m['dump']['sha256'], expected_active_db=None,
                        manifest=self.m, step='restoring')

    def names(self):
        return [name for name, _, _ in self.calls]

    def call(self, name):
        return next((args, kwargs) for found, args, kwargs in self.calls if found == name)

    def sql(self, name):
        return self.call(name)[1]['input'].decode()

    def assert_activator(self, args, database, path='pg_catalog'):
        self.assertIn('PGPASSWORD=$(cat /run/secrets/activator_db_password)', ' '.join(args))
        self.assertEqual(args[args.index('-U') + 1], 'activator')
        self.assertEqual(args[args.index('-d') + 1] if '-d' in args else args[args.index('--dbname') + 1], database)
        self.assertNotIn('postgres-admin.sh', ' '.join(args))
        self.assertFalse(any(arg.startswith('PGOPTIONS=') and 'log_' in arg for arg in args))
        options = next(arg for arg in args if arg.startswith('PGOPTIONS='))
        self.assertTrue(options.endswith(' -c search_path=' + path) or options == 'PGOPTIONS=-c search_path=' + path)

    def assert_postgres(self, args, database):
        self.assertEqual(args[:6], ['podman', 'exec', '-i', '--env', ADMIN, 'trachuan-postgres'])
        self.assertEqual(args[6:], ['/usr/local/bin/postgres-admin.sh', 'psql', '-XqAt', '-h', '127.0.0.1', '-U',
                                    'postgres', '-d', database, '-v', 'ON_ERROR_STOP=1'])

    def assert_guard_first(self, sql, statement):
        self.assertIn('DO $guard$', sql)
        for condition in GUARD:
            self.assertIn(condition, sql)
        self.assertLess(sql.index('DO $guard$'), sql.index(statement))


class FrameworksCreateTest(StepsFixture, unittest.TestCase):
    def test_create_as_activator_then_one_postgres_log_step_then_effective_check(self):
        self.steps.create(self.row)
        self.assertEqual(self.names(), ['extensions', 'exists', 'createdb', 'owner', 'configure', 'effective'])
        args, _ = self.call('createdb')
        self.assertIn('--template=template0', args)
        self.assertEqual(args[-1], self.name)
        self.assertEqual(args[args.index('-U') + 1], 'activator')
        args, _ = self.call('configure')
        self.assert_postgres(args, self.name)
        sql = self.sql('configure')
        target = '"' + self.name + '"'
        for scope in ('DATABASE ' + target, 'ROLE "activator" IN DATABASE ' + target,
                      'ROLE "corpus_reader" IN DATABASE ' + target):
            for setting in SETTINGS:
                self.assertIn('ALTER ' + scope + ' SET ' + setting + ';\n', sql)
        for text in ('FROM pg_catalog.pg_parameter_acl a', 'pg_catalog.aclexplode(a.paracl)', 'JOIN pg_catalog.pg_roles r',
                     "privilege_type='SET'", 'rolsuper', 'pg_catalog.unnest(',
                     "pg_catalog.has_parameter_privilege(r.oid,p,'SET')",
                     'REVOKE ALL ON DATABASE ' + target + ' FROM PUBLIC;\n',
                     "CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public VERSION '0.8.1';",
                     "CREATE EXTENSION IF NOT EXISTS ltree WITH SCHEMA public VERSION '1.3';",
                     "pg_catalog.pg_has_role(r.oid,s.oid,'MEMBER')"):
            self.assertIn(text, sql)
        self.assertNotIn('GRANT', sql)
        self.assertLess(sql.index('pg_has_role'), sql.index('\\if'))
        self.assertLess(sql.index('pg_parameter_acl'), sql.index('\\if'))
        self.assertLess(sql.index('\\if'), sql.index('ALTER DATABASE'))
        args, _ = self.call('effective')
        self.assert_activator(args, self.name)
        for setting in ('log_min_messages', 'log_min_error_statement', 'log_min_duration_statement', 'log_statement'):
            self.assertIn("pg_catalog.current_setting('" + setting + "')", self.sql('effective'))
        self.assertIn('pg_catalog.concat_ws(', self.sql('effective'))

    def test_existing_target_is_dropped_and_created_again(self):
        self.responses['exists'] = b't\n'
        self.steps.create(self.row)
        self.assertEqual(self.names(), ['extensions', 'exists', 'owner', 'protected', 'dropdb', 'createdb', 'owner',
                                        'configure', 'effective'])
        self.assertIn("operation_id <> '" + 'e' * 32 + "'", self.sql('protected'))

    def test_existing_target_activator_does_not_own_is_refused_without_a_postgres_session(self):
        self.responses.update(exists=b't\n', owner=b'postgres\n')
        with self.assertRaisesRegex(ValueError, '^activation target not owned by activator$'):
            self.steps.create(self.row)
        self.assertEqual(self.names(), ['extensions', 'exists', 'owner'])

    def test_protected_existing_target_is_refused_and_kept(self):
        self.responses.update(exists=b't\n', protected=b't\n')
        with self.assertRaisesRegex(ValueError, 'refusing to drop protected database'):
            self.steps.create(self.row)
        for name in ('dropdb', 'createdb', 'configure'):
            self.assertNotIn(name, self.names())

    def test_another_superuser_refuses_before_the_effective_check(self):
        self.responses['configure'] = b'superuser\n'
        with self.assertRaisesRegex(ValueError, '^a role other than postgres is a superuser or a member of one$'):
            self.steps.create(self.row)
        self.assertNotIn('effective', self.names())

    def test_parameter_grant_refuses_before_effective_check(self):
        self.responses['configure'] = b'granted\n'
        with self.assertRaisesRegex(ValueError, '^frameworks log parameter SET granted to a role other than a superuser$'):
            self.steps.create(self.row)
        self.assertNotIn('effective', self.names())
        self.responses['configure'] = b'\n'
        with self.assertRaisesRegex(ValueError, '^frameworks log settings not configured$'):
            self.steps.create(self.row)

    def test_effective_values_other_than_the_four_refuse(self):
        for value in (b'warning,panic,-1,none\n', b'panic,error,-1,none\n', b'panic,panic,0,none\n',
                      b'panic,panic,-1,all\n', b'\n'):
            self.responses['effective'] = value
            with self.subTest(value=value), self.assertRaisesRegex(ValueError, '^frameworks log settings refused$'):
                self.steps.create(self.row)

    def test_owner_other_than_activator_refuses_before_postgres_step(self):
        self.responses['owner'] = b'postgres\n'
        with self.assertRaisesRegex(ValueError, 'not owned by activator'):
            self.steps.create(self.row)
        self.assertNotIn('configure', self.names())

    def test_law_database_name_is_refused_before_any_command(self):
        with self.assertRaisesRegex(ValueError, 'invalid frameworks database name'):
            self.steps.create(dict(self.row, database_name='corpus_vn_20990102'))
        self.assertEqual(self.calls, [])

    def test_configure_refuses_names_that_are_not_plain_identifiers(self):
        for name, extensions, roles in (('corpus_vn_20990102', {'vector': '0.8.1'}, ('activator', 'corpus_reader')),
                                        (self.name, {'vector; DROP': '0.8.1'}, ('activator', 'corpus_reader')),
                                        (self.name, {'vector': '0.8.1'}, ('Activator"', 'corpus_reader')),
                                        (self.name, {'vector': '0.8.1'}, ('activator', 'reader; DROP'))):
            with self.subTest(name=name, extensions=extensions, roles=roles), self.assertRaises(ValueError):
                sql.configure(name, extensions, *roles)

    def test_dead_advisory_lock_refuses_postgres_step(self):
        self.db.holder.poll.return_value = 1
        with self.assertRaisesRegex(ValueError, 'activation advisory lock lost'):
            self.steps.create(self.row)
        self.assertEqual(self.calls, [])


class FrameworksRestoreTest(StepsFixture, unittest.TestCase):
    def restore(self):
        self.steps.restore(self.row, '/synthetic/dump', 'server-image')

    def test_restore_checks_settings_filters_toc_restores_applies_image_schema_then_grants(self):
        self.restore()
        self.assertEqual(self.names(), ['effective', 'restored', 'schema', 'list', 'write-list', 'pg_restore',
                                        'remove-list', 'apply', 'check', 'grants'])
        self.assert_activator(self.call('effective')[0], self.name)
        self.assert_activator(self.call('restored')[0], self.name)
        self.assert_guard_first(self.sql('restored'), "pg_catalog.to_regclass('public.release')")
        args, kwargs = self.call('schema')
        self.assertEqual(args, ['podman', 'run', '--rm', '--network=host', '--read-only', '--cap-drop=ALL',
                                '--security-opt=no-new-privileges', '--env-file', str(self.paths.host / 'server.env'),
                                'server-image', 'frameworks-schema', '--version', '1'])
        self.assertEqual(kwargs['timeout'], 150)
        self.assertEqual(self.call('list')[1]['input_file'], '/synthetic/dump')
        listing = '/run/pgbackrest/corpus-' + 'e' * 32 + '.list'
        args, kwargs = self.call('write-list')
        self.assertEqual(args[-1], listing)
        self.assertEqual(kwargs['input'].decode(), SPECIAL + DATA)
        args, kwargs = self.call('pg_restore')
        self.assert_activator(args, self.name)
        self.assertEqual(args[args.index('pg_restore') - 1].split(' -c ')[-1], 'search_path=pg_catalog')
        for value in ('--no-owner', '--no-privileges', '--exit-on-error', '--single-transaction'):
            self.assertIn(value, args)
        self.assertEqual(args[args.index('--use-list') + 1], listing)
        self.assertEqual(kwargs['input_file'], '/synthetic/dump')
        options = next(arg for arg in args if arg.startswith('PGOPTIONS='))
        for value in ('maintenance_work_mem=2GB', 'max_parallel_maintenance_workers=2', 'statement_timeout=0',
                      'lock_timeout=0', 'idle_in_transaction_session_timeout=0', 'transaction_timeout=0'):
            self.assertIn(value, options)
        self.assertEqual(self.call('remove-list')[0][-1], listing)
        args, _ = self.call('apply')
        self.assert_activator(args, self.name, 'pg_catalog,public')
        self.assertIn('ON_ERROR_STOP=1', args)
        sql = self.sql('apply')
        self.assertTrue(sql.startswith('SET standard_conforming_strings=on;\nBEGIN;\nDO $guard$'))
        self.assertIn('SET LOCAL search_path = public;\n' + executable_sql(), sql)
        self.assertTrue(sql.endswith('COMMIT;\n'))
        self.assert_activator(self.call('check')[0], self.name)
        args, _ = self.call('grants')
        self.assert_activator(args, self.name, 'pg_catalog,public')
        sql = self.sql('grants')
        self.assert_guard_first(sql, 'REVOKE')
        for statement in ('GRANT CONNECT ON DATABASE "' + self.name + '" TO "corpus_reader";',
                          'REVOKE CREATE ON SCHEMA public FROM PUBLIC;', 'GRANT USAGE ON SCHEMA public TO "corpus_reader";',
                          'GRANT SELECT ON ALL TABLES IN SCHEMA public TO "corpus_reader";',
                          'GRANT SELECT ON ALL SEQUENCES IN SCHEMA public TO "corpus_reader";'):
            self.assertIn(statement + '\n', sql)
        self.assertNotIn('grant-corpus', self.names())

    def test_settings_refusal_stops_before_reading_the_dump(self):
        self.responses['effective'] = b'warning,panic,-1,none\n'
        with self.assertRaisesRegex(ValueError, '^frameworks log settings refused$'):
            self.restore()
        self.assertEqual(self.names(), ['effective'])

    def test_executable_outside_schema_version_is_refused_before_restore(self):
        for extra in ('5; 1255 17001 FUNCTION public invented_function() builder',
                      '5; 1259 17000 VIEW public invented_view builder',
                      '5; 2620 17004 TRIGGER public release invented_trigger builder',
                      '5; 2618 17005 RULE public release invented_rule builder'):
            self.calls.clear()
            self.steps.schemas.clear()
            self.responses['list'] = (HEADER + DATA + EXECUTABLE + extra + '\n').encode()
            with self.subTest(extra=extra):
                with self.assertRaises(ValueError):
                    self.steps.inspect(self.row, '/synthetic/dump', 'server-image')
                self.assertEqual(self.names(), ['extensions', 'schema', 'list'])
                with self.assertRaises(ValueError):
                    self.restore()
                for name in ('write-list', 'pg_restore', 'apply', 'grants'):
                    self.assertNotIn(name, self.names())

    def test_inspect_checks_extensions_and_toc_without_writing_a_list(self):
        self.steps.inspect(self.row, '/synthetic/dump', 'server-image')
        self.assertEqual(self.names(), ['extensions', 'schema', 'list'])
        self.restore()
        self.assertEqual(self.names().count('schema'), 1)

    def test_invalid_schema_or_toc_bytes_fail_without_echoing_them(self):
        for name in ('schema', 'list'):
            self.steps.schemas.clear()
            self.responses = dict(self.responses, **{name: b'\xffinvented'})
            with self.subTest(name=name), self.assertRaises(ValueError) as caught:
                self.steps.inspect(self.row, '/synthetic/dump', 'server-image')
            self.assertNotIn('xff', str(caught.exception))
            self.assertNotIn('invented', str(caught.exception))
            self.responses[name] = executable_sql().encode() if name == 'schema' else (HEADER + DATA).encode()

    def test_resumed_restore_reapplies_schema_and_grants_without_pg_restore(self):
        self.responses['restored'] = b't\n'
        self.restore()
        self.assertEqual(self.names(), ['effective', 'restored', 'schema', 'apply', 'check', 'grants'])

    def test_schema_apply_failure_fails_the_restore_before_grants(self):
        self.failures['apply'] = CommandError('command failed', detail='psql: exit 3')
        with self.assertRaises(CommandError):
            self.restore()
        self.assertNotIn('grants', self.names())

    def test_pg_restore_failure_removes_the_list_and_skips_schema(self):
        self.failures['pg_restore'] = CommandError('command failed', detail='pg_restore: exit 1: invented')
        with self.assertRaises(CommandError):
            self.restore()
        self.assertEqual(self.names()[-2:], ['pg_restore', 'remove-list'])


if __name__ == '__main__':
    unittest.main()
