import re
import unittest

from updater import deploy_frameworks_check as check
from updater.frameworks_steps_test import StepsFixture
from updater.frameworks_toc_test import executable_sql


LOGS = 'log_min_duration_statement=-1,log_min_error_statement=panic,log_min_messages=panic,log_statement=none'
UNQUALIFIED = re.compile(r"(?<![.\w'])(?:pg_(?!catalog\b|temp\b)[a-z_]+|(?:current_setting|format_type|left|unnest|"
                         r"array_to_string|concat_ws|to_regclass|json_build_object|current_database)\()")


class FrameworksCheckTest(StepsFixture, unittest.TestCase):
    def restore(self):
        self.steps.restore(self.row, '/synthetic/dump', 'server-image')

    def test_check_reapplies_image_schema_between_catalog_snapshots_and_rolls_back(self):
        self.restore()
        args, _ = self.call('check')
        self.assert_activator(args, self.name)
        self.assertIn('ON_ERROR_STOP=1', args)
        sql = self.sql('check')
        self.assertTrue(sql.startswith('SET standard_conforming_strings=on;\nBEGIN;\nDO $guard$'))
        self.assertTrue(sql.endswith('ROLLBACK;\n'))
        order = ['CREATE TEMP TABLE frameworks_before', 'SET LOCAL search_path = public;\n' + executable_sql(),
                 'SET LOCAL search_path = pg_catalog;\n', 'CREATE TEMP TABLE frameworks_after']
        self.assertEqual([sql.index(text) for text in order], sorted(sql.index(text) for text in order))
        self.assertEqual(sql.count("('view','public."), 4)
        self.assertEqual(sql.count("('function','public."), 6)
        self.assertEqual(sql.count("('trigger','public."), 8)
        for row in ("('view','public.body_lexical_stats',NULL)",
                    "('function','public.check_evidence_dependencies()',NULL)",
                    "('trigger','public.release release_versions_sealed',NULL)",
                    "('extension','ltree','1.3 public superuser')", "('extension','vector','0.8.1 public superuser')",
                    "('setting','database','" + LOGS + "')", "('setting','role activator','" + LOGS + "')",
                    "('setting','role corpus_reader','" + LOGS + "')"):
            self.assertIn(row, sql)

    def test_snapshot_covers_every_kind_with_qualified_catalog_names(self):
        sql = check.executable(executable_sql(), self.m['extensions'])
        for kind in check.KINDS:
            self.assertIn("'" + kind + "'", check.SNAPSHOT)
        self.assertEqual(set(check.KINDS), {'view', 'materialized view', 'trigger', 'function', 'procedure',
                                            'aggregate', 'rule', 'policy', 'event trigger', 'operator', 'cast',
                                            'schema', 'extension', 'type', 'domain', 'setting'})
        for source in ('pg_catalog.pg_class', 'pg_catalog.pg_trigger', 'pg_catalog.pg_proc', 'pg_catalog.pg_rewrite',
                       'pg_catalog.pg_policy', 'pg_catalog.pg_event_trigger', 'pg_catalog.pg_operator',
                       'pg_catalog.pg_cast', 'pg_catalog.pg_namespace', 'pg_catalog.pg_extension',
                       'pg_catalog.pg_type', 'pg_catalog.pg_db_role_setting', 'pg_catalog.pg_get_viewdef(',
                       'pg_catalog.pg_get_functiondef(', 'pg_catalog.pg_get_triggerdef(', "deptype='e'"):
            self.assertIn(source, sql)
        self.assertEqual(UNQUALIFIED.findall(sql.replace(executable_sql(), '')), [])
        for unsafe in ('prokind||', 'relkind||', 'typtype||', 'prokind ||', 'relkind ||'):
            self.assertNotIn(unsafe, sql)
        self.assertIn('pg_temp.frameworks_before', sql)
        self.assertIn('pg_temp.frameworks_after', sql)

    def test_roles_and_extensions_must_be_plain_names(self):
        for extensions, roles in (({'vector; DROP': '1'}, ('activator', 'corpus_reader')),
                                  ({'vector': '1'}, ('activator', 'Reader"'))):
            with self.subTest(extensions=extensions, roles=roles), self.assertRaises(ValueError):
                check.executable(executable_sql(), extensions, *roles)

    def test_difference_fails_with_the_kind_only(self):
        for outcome, kind in [(kind.encode() + b'\n', kind) for kind in check.KINDS] + [
                (b'invented secret body\n', 'object'), (b'\n', 'object')]:
            self.calls.clear()
            self.responses['check'] = outcome
            with self.subTest(outcome=outcome), self.assertRaises(ValueError) as caught:
                self.restore()
            self.assertEqual(str(caught.exception), 'frameworks database ' + kind + ' differs from image schema version')
            self.assertNotIn('grants', self.names())

    def test_no_postgres_session_once_the_restore_starts_and_every_session_pins_search_path(self):
        dump = '/synthetic/dump'
        self.steps.inspect(self.row, dump, 'server-image')
        self.steps.create(self.row)
        self.steps.restore(self.row, dump, 'server-image')
        self.steps.validate(self.row)
        self.steps.smoke(self.row, 'server-image')
        names = self.names()
        start = names.index('pg_restore')
        self.assertEqual(names[start:], ['pg_restore', 'remove-list', 'apply', 'check', 'grants', 'effective',
                                         'validate', 'release-check'])
        for name, args, _ in self.calls[start:]:
            joined = ' '.join(args)
            self.assertNotIn('postgres-admin.sh', joined)
            self.assertNotIn('grant-corpus.sh', joined)
            self.assertNotIn('pg_prewarm', joined)
        for name, args, _ in self.calls:
            if self.name in args and name not in ('createdb', 'release-check'):
                with self.subTest(name=name):
                    self.assertTrue(any(arg.startswith('PGOPTIONS=') and ' -c search_path=' in ' ' + arg[10:]
                                        for arg in args))


if __name__ == '__main__':
    unittest.main()
