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
MISMATCH = b'target-identity-differs\n'
REFUSED = '^activation target database identity changed$'


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
    for word, name in (('pg_terminate_backend', 'end-backends'), ('DROP DATABASE %I', 'strays'),
                       ('AS target_oid', 'target-oid'), ('pg_available_extensions', 'extensions'), ('datdba', 'owner'),
                       ('EXISTS(SELECT FROM pg_catalog.pg_database', 'exists'),
                       ('corpus_release WHERE active_db', 'protected'), ('pg_catalog.pg_event_trigger', 'check'),
                       ("to_regclass('public.release')", 'restored'), ('GRANT SELECT ON ALL TABLES', 'grants'),
                       ('FROM public.release WHERE singleton', 'validate'), ('public.pg_prewarm', 'prewarm'),
                       ('document_summary', 'projection'), ('release_target', 'verify'),
                       ('pg_has_role', 'superuser')):
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
                              **{'end-backends': b'0\nf\n', 'target-oid': b'16400\n'},
                              validate=json.dumps(release).encode())
        self.responses['release-check'] = b'{}\n'
        def runner(args, **kwargs):
            name = classify(args, (kwargs.get('input') or b'').decode())
            self.calls.append((name, args, kwargs))
            if name in self.failures:
                raise self.failures[name]
            value = self.responses.get(name, b'')
            return value.pop(0) if isinstance(value, list) else value
        self.db = Database(Host(Paths(Path(self.work.name)), runner, user_prefix=[]), {'extensions': ['vector', 'ltree']})
        self.db.holder = mock.Mock()
        self.db.holder.poll.return_value = None
        self.db.backend = 77
        self.row = dict(operation_id=OPERATION, country='vn', rel=self.m['rel'], mode='new', database_name=self.name,
                        manifest_hash='a' * 64, dump_sha256=self.m['dump']['sha256'],
                        expected_active_db='corpus_vn_20990101_1', manifest=self.m, step='restoring')
        self.db.targets().write(OPERATION, self.name, '16400')

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
        self.assertEqual(self.names(), ['extensions', 'exists', 'end-backends', 'strays', 'createdb', 'owner',
                                        'target-oid', 'prepare'])
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
        self.assertLess(text.index('release_target'), text.index('rolsuper'))
        self.assertLess(text.index('rolsuper'), text.index('\\if :release_superuser'))
        self.assertLess(text.index('\\if :release_superuser'), text.index('CREATE EXTENSION'))
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
        self.assertEqual(self.names(), ['extensions', 'exists', 'owner', 'end-backends', 'strays', 'protected', 'dropdb',
                                        'createdb', 'owner', 'target-oid', 'prepare'])
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

    def test_other_backends_are_ended_except_this_session_and_the_lock_holder(self):
        self.db.create(self.row)
        text = self.sql('end-backends')
        self.assertIn('pg_catalog.pg_terminate_backend(a.pid,5000)', text)
        self.assertIn('a.usename=current_user', text)
        self.assertEqual(text.count('a.pid NOT IN (pg_catalog.pg_backend_pid(),77)'), 2)
        self.assert_activator(self.call('end-backends')[0], 'auth', 'pg_catalog,public')

    def test_a_backend_that_survives_refuses_the_create(self):
        self.responses['end-backends'] = b'1\nt\n'
        with self.assertRaisesRegex(ValueError, '^another activator session could not be ended$'):
            self.db.create(self.row)
        for name in ('strays', 'createdb', 'prepare'):
            self.assertNotIn(name, self.names())

    def test_without_the_lock_backend_nothing_is_ended(self):
        self.db.backend = None
        with self.assertRaisesRegex(ValueError, 'activation advisory lock lost'):
            self.db.create(self.row)
        self.assertNotIn('end-backends', self.names())

    def test_strays_are_every_owned_database_no_kind_protects(self):
        self.db.create(self.row)
        text = self.sql('strays')
        for word in ('public.corpus_release', 'public.frameworks_release', 'public.retired_release',
                     'public.release_journal', 'retain_until > pg_catalog.now()', "mode <> 'prune'",
                     'pg_catalog.pg_get_userbyid(d.datdba)=current_user', "format('DROP DATABASE %I',d.datname)",
                     "format('ALTER DATABASE %I IS_TEMPLATE false',d.datname)", '\\gexec'):
            self.assertIn(word, text)
        self.assertNotIn('corpus_kind', text)
        self.assertLess(self.names().index('strays'), self.names().index('createdb'))

    def test_target_identity_is_recorded_after_createdb_and_checked_first_in_the_postgres_step(self):
        self.responses['target-oid'] = b'17001\n'
        self.db.create(self.row)
        self.assertEqual(self.db.targets().read(OPERATION, self.name), '17001')
        self.assertIn("='17001' AS release_target", self.sql('prepare'))
        self.responses['prepare'] = MISMATCH
        with self.assertRaisesRegex(ValueError, REFUSED):
            self.db.create(self.row)

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
        self.assertEqual(self.names(), ['restored', 'list', 'write-list', 'verify', 'pg_restore', 'remove-list', 'verify',
                                        'check', 'grants'])
        self.assertNotIn('relation corpus_reader SELECT', self.sql('check'))
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
        self.assertIn("'relation corpus_reader SELECT'", self.sql('check'))

    def test_target_identity_is_verified_before_and_after_pg_restore(self):
        self.responses['verify'] = MISMATCH
        with self.assertRaisesRegex(ValueError, REFUSED):
            self.restore()
        self.assertNotIn('pg_restore', self.names())
        self.calls.clear()
        self.responses['verify'] = [b'', MISMATCH]
        with self.assertRaisesRegex(ValueError, REFUSED):
            self.restore()
        self.assertIn('pg_restore', self.names())
        self.assertNotIn('check', self.names())

    def test_every_session_on_the_target_refuses_another_identity(self):
        original = dict(self.responses)
        for name, action in (('restored', self.restore), ('validate', lambda: self.db.validate(self.row)),
                             ('prewarm', lambda: self.db.smoke(self.row, 'server-image'))):
            self.responses = dict(original, **{name: MISMATCH})
            with self.subTest(name=name), self.assertRaisesRegex(ValueError, REFUSED):
                action()

    def test_the_swap_verifies_the_target_first(self):
        self.responses['verify'] = MISMATCH
        with self.assertRaisesRegex(ValueError, REFUSED):
            self.db.activate(self.row)
        self.assertEqual(self.names(), ['verify'])

    def test_a_resume_without_a_record_adopts_the_current_identity(self):
        self.db.targets().path(OPERATION).unlink()
        self.responses['target-oid'] = b'17002\n'
        self.db.validate(self.row)
        self.assertEqual(self.names(), ['target-oid', 'validate'])
        self.assertIn("='17002' AS release_target", self.sql('validate'))
        self.assertEqual(self.db.targets().read(OPERATION, self.name), '17002')

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


if __name__ == '__main__':
    unittest.main()
