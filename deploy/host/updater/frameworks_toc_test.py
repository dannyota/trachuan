from pathlib import Path
import unittest

from updater import deploy_frameworks_schema as s
from updater.deploy_corpus_verify import toc as law_toc


SCHEMA = Path(__file__).parents[3] / 'internal/corpusdb/frameworks'
EXTENSIONS = ['vector', 'ltree']
SPECIAL = '3500; 0 0 ENCODING - ENCODING \n3501; 0 0 STDSTRINGS - STDSTRINGS \n3502; 0 0 SEARCHPATH - SEARCHPATH \n'
HEADER = (';\n; Archive created at 2099-01-01\n;\n' + SPECIAL +
          '2; 3079 16385 EXTENSION - ltree \n3; 3079 16386 EXTENSION - vector \n')
DATA = ('215; 1259 16500 TABLE public release builder\n'
        '216; 1259 16510 TABLE public mapping_evidence builder\n'
        '3400; 0 16500 TABLE DATA public release builder\n'
        '3300; 2606 16800 CONSTRAINT public release release_pkey builder\n'
        '3301; 2606 16801 FK CONSTRAINT public mapping_evidence mapping_evidence_mapping_fkey builder\n'
        '3302; 1259 16802 INDEX public mapping_evidence_versions builder\n')
EXECUTABLE = ('225; 1259 16600 VIEW public body_lexical_stats builder\n'
              '230; 1255 16700 FUNCTION public check_evidence_dependencies() builder\n'
              '3310; 2620 16900 TRIGGER public mapping_evidence mapping_evidence_dependencies builder\n'
              '3311; 2620 16901 TRIGGER public release release_versions_sealed \n')


def executable_sql():
    return ''.join((SCHEMA / name).read_text() for name in ('views.sql', 'versions.sql', 'evidence.sql'))


class FrameworksTocTest(unittest.TestCase):
    def test_schema_objects_are_the_image_views_functions_and_triggers(self):
        objects = s.objects(executable_sql())
        views = {('VIEW', name) for name in ('body_lexical_stats', 'body_lexical_df', 'public_lexical_stats',
                                             'public_lexical_df')}
        functions = {('FUNCTION', name) for name in ('check_evidence_dependencies', 'reject_version_mutation',
                                                     'reject_lifecycle_reset', 'check_version_insert',
                                                     'check_evidence_insert', 'check_versions_seal_transition')}
        triggers = {('TRIGGER', 'mapping_evidence', 'mapping_evidence_dependencies'),
                    ('TRIGGER', 'framework_version', 'framework_version_immutable'),
                    ('TRIGGER', 'release', 'release_delete'), ('TRIGGER', 'release', 'release_truncate'),
                    ('TRIGGER', 'mapping_evidence', 'mapping_evidence_truncate'),
                    ('TRIGGER', 'framework_version', 'framework_version_insert'),
                    ('TRIGGER', 'mapping_evidence', 'mapping_evidence_insert'),
                    ('TRIGGER', 'release', 'release_versions_sealed')}
        self.assertEqual(objects, views | functions | triggers)

    def test_unrecognized_schema_statement_is_refused(self):
        for statement in ('CREATE TABLE invented (id bigint);',
                          'CREATE OR REPLACE FUNCTION invented(a integer) RETURNS trigger LANGUAGE plpgsql AS $$',
                          'CREATE OR REPLACE FUNCTION invented() RETURNS integer LANGUAGE sql AS $$',
                          'CREATE RULE invented AS ON INSERT TO release DO INSTEAD NOTHING;',
                          'CREATE OR REPLACE VIEW "Invented" AS'):
            with self.subTest(statement=statement), self.assertRaisesRegex(ValueError, 'unrecognized frameworks schema'):
                s.objects(executable_sql() + statement + '\n')

    def test_toc_omits_image_executable_objects_and_keeps_tables_data_constraints_indexes(self):
        self.assertEqual(s.toc(HEADER + DATA + EXECUTABLE, EXTENSIONS, executable_sql()), SPECIAL + DATA)

    def test_toc_refuses_view_function_or_trigger_outside_schema_version_with_type_only(self):
        for extra in ('5; 1259 17000 VIEW public invented_secret_view builder',
                      '5; 1255 17001 FUNCTION public invented_secret_function() builder',
                      '5; 1255 17002 FUNCTION public check_evidence_dependencies(integer) builder',
                      '5; 1255 17003 FUNCTION invented_secret_schema check_evidence_dependencies() builder',
                      '5; 2620 17004 TRIGGER public release invented_secret_trigger builder',
                      '5; 2620 17005 TRIGGER public framework mapping_evidence_dependencies builder',
                      '5; 1259 17006 VIEW public body_lexical_stats invented secret owner'):
            with self.subTest(extra=extra), self.assertRaisesRegex(ValueError, 'outside schema version') as caught:
                s.toc(HEADER + DATA + extra + '\n', EXTENSIONS, executable_sql())
            self.assertNotIn('invented', str(caught.exception))
            self.assertNotIn('secret', str(caught.exception))

    def test_toc_refuses_every_other_executable_object_type(self):
        for extra in ('5; 2618 17000 RULE public framework invented_rule builder',
                      '5; 3256 17001 POLICY public framework invented_policy builder',
                      '5; 1255 17002 AGGREGATE public invented_aggregate(integer) builder',
                      '5; 2617 17003 OPERATOR public === (integer, integer) builder',
                      '5; 2605 17004 CAST - CAST (text AS integer) ',
                      '5; 3466 17005 EVENT TRIGGER - invented_event builder',
                      '5; 1255 17006 PROCEDURE public invented_procedure() builder',
                      '5; 1259 17007 MATERIALIZED VIEW public invented_view builder',
                      '5; 0 0 MATERIALIZED VIEW DATA public invented_view builder',
                      '5; 1247 17008 TYPE public invented_type builder',
                      '5; 1247 17009 DOMAIN public invented_domain builder',
                      '5; 0 17010 ROW SECURITY public framework builder',
                      '5; 2612 17011 PROCEDURAL LANGUAGE - plpython3u builder',
                      '5; 3576 17012 TRANSFORM - TRANSFORM FOR integer LANGUAGE plpython3u ',
                      '5; 2616 17013 OPERATOR CLASS public invented_class USING btree builder',
                      '5; 2601 17014 ACCESS METHOD - invented_method ',
                      '5; 3079 17015 EXTENSION - plpython3u ',
                      '5; 6104 17016 PUBLICATION - invented_publication builder'):
            with self.subTest(extra=extra), self.assertRaises(ValueError):
                s.toc(HEADER + DATA + EXECUTABLE + extra + '\n', EXTENSIONS, executable_sql())

    def test_toc_keeps_law_extension_rules(self):
        with self.assertRaisesRegex(ValueError, 'missing dump extensions'):
            s.toc(HEADER.replace('2; 3079 16385 EXTENSION - ltree \n', '') + DATA, EXTENSIONS, executable_sql())
        with self.assertRaisesRegex(ValueError, 'duplicate'):
            s.toc(HEADER + '9; 3079 16387 EXTENSION - vector \n' + DATA, EXTENSIONS, executable_sql())
        with self.assertRaisesRegex(ValueError, 'invalid dump TOC'):
            s.toc(HEADER + 'malformed\n', EXTENSIONS, executable_sql())

    def test_law_toc_still_refuses_views_functions_and_triggers(self):
        for line in EXECUTABLE.splitlines():
            with self.subTest(line=line), self.assertRaisesRegex(ValueError, 'unsafe dump TOC object'):
                law_toc(HEADER + DATA + line + '\n', EXTENSIONS)
        self.assertEqual(law_toc(HEADER + DATA, EXTENSIONS), s.toc(HEADER + DATA, EXTENSIONS, executable_sql()))


if __name__ == '__main__':
    unittest.main()
