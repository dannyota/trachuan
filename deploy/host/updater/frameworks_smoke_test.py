import json
import sys
import unittest

from updater.deploy_corpus_database import Database
from updater.deploy_corpus_kind import FRAMEWORKS, LAW
from updater.deploy_frameworks_steps import SAFE
from updater.deploy_io import command, CommandError
from updater.frameworks_steps_test import StepsFixture
from updater.frameworks_verify_test import rules


class FrameworksValidateTest(StepsFixture, unittest.TestCase):
    def test_validate_reads_release_row_in_a_guarded_session_after_the_settings_check(self):
        self.steps.validate(self.row)
        self.assertEqual(self.names(), ['effective', 'validate'])
        args, _ = self.call('validate')
        self.assert_activator(args, self.name)
        sql = self.sql('validate')
        self.assert_guard_first(sql, 'FROM release WHERE singleton')
        self.assertNotIn('country', sql)
        for column in ("'kind',kind", "'versions',version_count", "'controls',control_count"):
            self.assertIn(column, sql)

    def test_validate_refuses_any_difference_from_manifest(self):
        expected = dict({key: self.m[key] for key in ('rel', 'schema_version', 'recipes', 'counts')}, kind='frameworks')
        changes = [dict(expected, rel='20990101'), dict(expected, schema_version=2), dict(expected, kind='law'),
                   {key: value for key, value in expected.items() if key != 'kind'},
                   dict(expected, counts=dict(expected['counts'], versions=expected['counts']['versions'] + 1)),
                   dict(expected, counts=dict(expected['counts'], controls=expected['counts']['controls'] - 1)),
                   dict(expected, recipes=dict(expected['recipes'], chunk={'version': 2}))]
        for value in [json.dumps(change).encode() for change in changes] + [b'\n', b'{"rel":', b'[]']:
            self.responses['validate'] = value
            with self.subTest(value=value), self.assertRaisesRegex(ValueError, 'release row'):
                self.steps.validate(self.row)

    def test_validate_refuses_law_names_before_any_session(self):
        with self.assertRaisesRegex(ValueError, 'invalid frameworks database name'):
            self.steps.validate(dict(self.row, database_name='corpus_vn_20990102'))
        self.assertEqual(self.calls, [])

    def test_validate_refuses_settings_before_reading_the_row(self):
        self.responses['effective'] = b'panic,panic,-1,all\n'
        with self.assertRaisesRegex(ValueError, '^frameworks log settings refused$'):
            self.steps.validate(self.row)
        self.assertEqual(self.names(), ['effective'])


class FrameworksSmokeTest(StepsFixture, unittest.TestCase):
    def test_smoke_prewarms_as_postgres_with_log_options_then_runs_frameworks_release_check(self):
        self.steps.smoke(self.row, 'server-image')
        self.assertEqual(self.names(), ['prewarm', 'release-check'])
        args, _ = self.call('prewarm')
        self.assert_postgres(args, self.name)
        sql = self.sql('prewarm')
        self.assert_guard_first(sql, 'pg_prewarm')
        for text in ('SET statement_timeout=0;', 'SET lock_timeout=0;', 'SET idle_in_transaction_session_timeout=0;',
                     'SET transaction_timeout=0;', 'DROP EXTENSION pg_prewarm;'):
            self.assertIn(text, sql)
        args, kwargs = self.call('release-check')
        self.assertEqual(args, ['podman', 'run', '--rm', '--network=host', '--read-only', '--cap-drop=ALL',
                                '--security-opt=no-new-privileges', '--env-file', str(self.paths.host / 'server.env'),
                                '--secret', 'server_corpus_database_url,type=env,target=SERVER_CORPUS_DATABASE_URL',
                                'server-image', 'release-check', '--kind', 'frameworks', '--database', self.name])
        self.assertIs(kwargs['safe'], SAFE)
        self.assertEqual(kwargs['timeout'], 150)

    def test_smoke_requires_ok_status(self):
        for value in (b'{"status":"failed"}\n', b'[]\n'):
            self.responses['release-check'] = value
            with self.subTest(value=value), self.assertRaisesRegex(ValueError, '^frameworks release-check did not report ok$'):
                self.steps.smoke(self.row, 'server-image')

    def test_dead_advisory_lock_refuses_prewarm(self):
        self.db.holder.poll.return_value = 1
        with self.assertRaisesRegex(ValueError, 'activation advisory lock lost'):
            self.steps.smoke(self.row, 'server-image')
        self.assertEqual(self.calls, [])

    def test_release_check_database_names_are_checked_per_kind(self):
        with self.assertRaisesRegex(ValueError, 'invalid frameworks database name'):
            self.db.describe('server-image', 'corpus_vn_20990102')
        law = Database(self.db.host, rules(), LAW)
        with self.assertRaisesRegex(ValueError, 'invalid corpus database name'):
            law.describe('server-image', self.name)
        self.assertEqual(self.calls, [])


class FrameworksCauseTest(unittest.TestCase):
    def test_safe_release_check_line_grammar(self):
        for line, cause in (('server: frameworks release-check search: SQLSTATE 42P01',
                             'frameworks release-check search: SQLSTATE 42P01'),
                            ('server: frameworks release-check open engine: Go type serve.failure',
                             'frameworks release-check open engine: Go type serve.failure'),
                            ('server: frameworks release-check log settings: Go type *errors.errorString',
                             'frameworks release-check log settings: Go type *errors.errorString')):
            self.assertEqual(SAFE.fullmatch(line)[1], cause)
        for line in ('server: frameworks release-check search: SQLSTATE 42P01 invented',
                     'server: frameworks release-check search: invented licensed text',
                     'server: frameworks release-check search: Go type invented text',
                     'frameworks release-check search: SQLSTATE 42P01',
                     'server: invalid release-check arguments'):
            self.assertIsNone(SAFE.fullmatch(line))

    def test_command_keeps_only_the_last_safe_stderr_line(self):
        script = ('import sys; sys.stderr.write("Error: invented licensed marker\\n"'
                  '"server: frameworks release-check read: SQLSTATE 57014\\n"); sys.exit(1)')
        with self.assertRaises(CommandError) as raised:
            command([sys.executable, '-I', '-c', script], safe=SAFE)
        self.assertEqual(raised.exception.safe, 'frameworks release-check read: SQLSTATE 57014')
        with self.assertRaises(CommandError) as raised:
            command([sys.executable, '-I', '-c', 'import sys; sys.stderr.write("invented marker\\n"); sys.exit(1)'],
                    safe=SAFE)
        self.assertIsNone(raised.exception.safe)
        with self.assertRaises(CommandError) as raised:
            command([sys.executable, '-I', '-c', script])
        self.assertIsNone(raised.exception.safe)

    def test_frameworks_cause_is_program_exit_and_safe_line_only(self):
        error = CommandError('command failed', detail='podman run: exit 1',
                             safe='frameworks release-check search: SQLSTATE 42P01')
        self.assertEqual(FRAMEWORKS.cause(error), 'podman run: exit 1: frameworks release-check search: SQLSTATE 42P01')
        error = CommandError('command failed', detail='pg_restore: exit 1: pg_restore: error: invented licensed marker')
        self.assertEqual(FRAMEWORKS.cause(error), 'pg_restore: exit 1')
        self.assertIsNone(FRAMEWORKS.cause(CommandError('command failed')))
        self.assertEqual(LAW.cause(error), error.detail)
        self.assertIsNone(LAW.cause(CommandError('command failed')))


if __name__ == '__main__':
    unittest.main()
