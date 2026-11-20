import json
from pathlib import Path
import tempfile
import unittest

from updater import deploy_corpus_sql as law
from updater import deploy_frameworks_sql as frameworks
from updater.deploy_corpus_kind import FRAMEWORKS, LAW
from updater.deploy_host import Host
from updater.deploy_release_tables import Tables
from updater.deploy_store import Paths
from updater.frameworks_sql_test import frameworks_row, law_row


BREAKOUT = 'vn$activation$ \\! id > /tmp/x\n'
HOSTILE_SQL = (BREAKOUT, '$activation$', '$$', '\\! id', 'vn\\', 'vn\n', "vn'", "vn';--", 'v$n')
KINDS = (('law', LAW, law, law_row), ('frameworks', FRAMEWORKS, frameworks, frameworks_row))
FIELDS = ('country', 'database_name', 'expected_active_db')
REFUSED = 'invalid release journal row'


def stored(kind, row, step, **changes):
    state = 'complete' if step in ('activated', 'reverted', 'aborted', 'pruned') else 'open'
    return dict(row, step=step, state=state, message=kind.label + ' ' + step, corpus_kind=kind.name, **changes)


def prune_row(row):
    return dict(row, mode='prune', manifest_hash=None, dump_sha256=None, expected_active_db=None, manifest=None,
                step='pruning')


class KindFixture:
    def setUp(self):
        self.work = tempfile.TemporaryDirectory()
        self.addCleanup(self.work.cleanup)
        self.calls = []
        self.scripts = []
        self.rows = []
        self.active = None
        self.journal_reply = None

    def runner(self, args, **kwargs):
        text = (kwargs.get('input') or b'').decode()
        self.calls.append(args)
        self.scripts.append(text)
        if 'INSERT INTO public.release_journal' in text:
            reply = self.journal_reply
            return reply if isinstance(reply, bytes) else json.dumps(reply).encode()
        if 'json_agg(j ORDER BY j.id)' in text:
            return json.dumps(self.rows).encode()
        if 'SELECT active_db' in text:
            return (self.active or '').encode()
        return b'f\n'

    def database(self, kind):
        return Tables(Host(Paths(Path(self.work.name)), self.runner, user_prefix=[]), kind)


class DollarBodyTest(unittest.TestCase):
    def test_helper_refuses_dollar_backslash_and_non_strings(self):
        for value in ('$', 'a$b', '\\', 'a\\b', BREAKOUT, 7, True, ['vn']):
            with self.subTest(value=value), self.assertRaises(ValueError) as raised:
                law.dollar_body(value)
            if isinstance(value, str):
                self.assertNotIn(value, str(raised.exception))

    def test_helper_quotes_plain_values_and_null(self):
        self.assertEqual(law.dollar_body('vn'), "'vn'")
        self.assertEqual(law.dollar_body(None), 'NULL')

    def test_rendered_activation_has_one_dollar_quoted_body(self):
        for name, _, sql, row in KINDS:
            for mode in ('new', 'retained'):
                with self.subTest(kind=name, mode=mode):
                    text = sql.activate(row(mode))
                    self.assertEqual(text.count('$activation$'), 2)
                    self.assertNotIn('\\', text.split('$activation$')[1])

    def test_frameworks_bootstrap_renders_null_expected(self):
        self.assertEqual(frameworks.activate(frameworks_row(expected=None)).count('$activation$'), 2)

    def test_hostile_values_never_render(self):
        for name, _, sql, row in KINDS:
            for field in FIELDS:
                for value in HOSTILE_SQL:
                    for mode in ('new', 'retained'):
                        with self.subTest(kind=name, field=field, value=value, mode=mode):
                            with self.assertRaises(ValueError) as raised:
                                sql.activate(dict(row(mode), **{field: value}))
                            self.assertNotIn(value, str(raised.exception))

    def test_each_kind_uses_its_own_shapes(self):
        self.assertIn("'us'", law.activate(dict(law_row(), country='us')))
        for change in (dict(country='us'), dict(database_name='corpus_vn_20990102_1'),
                       dict(expected_active_db='corpus_vn_20990101_1')):
            with self.subTest(change=change), self.assertRaises(ValueError):
                frameworks.activate(dict(frameworks_row(), **change))
        for change in (dict(database_name='frameworks_20990102'), dict(expected_active_db='frameworks_20990101'),
                       dict(country='VN'), dict(country='vnm'), dict(database_name=None)):
            with self.subTest(change=change), self.assertRaises(ValueError):
                law.activate(dict(law_row(), **change))

    def test_projection_must_be_zero_or_one(self):
        for value in (2, -1, True, '1', '1; DROP', None):
            with self.subTest(value=value), self.assertRaises(ValueError):
                law.activate(law_row(), value)


class ActivationProcessTest(KindFixture, unittest.TestCase):
    def test_hostile_row_read_from_the_table_is_refused_after_one_read(self):
        for name, kind, _, row in KINDS:
            tables = self.database(kind)
            for field in FIELDS:
                for value in HOSTILE_SQL:
                    with self.subTest(kind=name, field=field, value=value):
                        self.calls.clear()
                        self.rows = [dict(row(), **{field: value})]
                        with self.assertRaises(law.JournalRefused) as raised:
                            tables.operations()
                        self.assertEqual(str(raised.exception), REFUSED)
                        self.assertEqual(len(self.calls), 1)

    def test_hostile_prune_row_is_refused(self):
        for name, kind, _, row in KINDS:
            with self.subTest(kind=name):
                self.calls.clear()
                self.rows = [dict(prune_row(row()), database_name=BREAKOUT)]
                with self.assertRaises(law.JournalRefused):
                    self.database(kind).operations()
                self.assertEqual(len(self.calls), 1)


class JournalShapeTest(unittest.TestCase):
    def bad(self, name, sql, row, field, value):
        with self.subTest(kind=name, field=field, value=value):
            with self.assertRaises(law.JournalRefused) as raised:
                sql.journal_row(dict(row, **{field: value}))
            self.assertEqual(str(raised.exception), REFUSED)

    def test_each_field_refuses_every_other_shape(self):
        common = dict(operation_id=('e' * 31, 'E' * 32, 'e' * 32 + '\n', None, 7),
                      mode=('old', 'New', '', None, ['new']),
                      manifest_hash=('a' * 63, 'A' * 64, '../x', None, 7),
                      dump_sha256=('a' * 65, '', None, 7),
                      step=('finished', 'Activated', '', None, ['started']),
                      manifest=(None, [], 'x', 7))
        specific = dict(law=dict(country=('VN', 'vnm', 'v', '', None, 7, 'vn\n'),
                                 rel=('2099010', '20990101_0', '20990101_', 'x', None, 7),
                                 database_name=('frameworks_20990102', 'auth', 'corpus_vn_20990102_0', None),
                                 expected_active_db=('frameworks_20990101', 'postgres', '', 7)),
                        frameworks=dict(country=('us', 'VN', '', None), rel=('20990102_123', 'x', None),
                                        database_name=('corpus_vn_20990102', 'auth', None),
                                        expected_active_db=('corpus_vn_20990101', 'postgres', '', 7)))
        for name, _, sql, row in KINDS:
            for field, values in dict(common, **specific[name]).items():
                for value in values:
                    self.bad(name, sql, row(), field, value)

    def test_non_objects_and_missing_fields_are_refused(self):
        for name, _, sql, row in KINDS:
            for value in (None, [], 'row', 7):
                with self.subTest(kind=name, value=value), self.assertRaises(law.JournalRefused):
                    sql.journal_row(value)
            for field in ('operation_id', 'country', 'rel', 'mode', 'database_name', 'manifest_hash', 'dump_sha256',
                          'step', 'manifest'):
                incomplete = {key: item for key, item in row().items() if key != field}
                with self.subTest(kind=name, missing=field), self.assertRaises(law.JournalRefused):
                    sql.journal_row(incomplete)

    def test_null_digests_and_manifest_are_for_prune_only(self):
        for name, _, sql, row in KINDS:
            for mode in ('new', 'retained'):
                for field in ('manifest_hash', 'dump_sha256', 'manifest'):
                    self.bad(name, sql, row(mode), field, None)

    def test_every_legitimate_shape_passes(self):
        stored = dict(id=9, state='open', at='2026-11-14T00:00:00+00:00', message='corpus started')
        for name, _, sql, row in KINDS:
            for step in ('started', 'creating', 'created', 'restoring', 'restored', 'validated', 'checked',
                         'activated', 'reverted', 'aborted', 'pruning', 'pruned'):
                for value in (dict(row(), step=step), dict(row('retained'), step=step),
                              dict(prune_row(row()), step=step), dict(row(), step=step, **stored)):
                    with self.subTest(kind=name, step=step, mode=value['mode']):
                        self.assertIs(sql.journal_row(value), value)
        for value in (dict(law_row(), rel='20261109', database_name='corpus_vn_20261109',
                           expected_active_db='corpus_vn_20261108_12'),
                      dict(law_row(), country='us', database_name='corpus_us_20990102_1'),
                      dict(prune_row(law_row()), manifest_hash='a' * 64, dump_sha256='b' * 64, manifest={'rel': 'x'})):
            with self.subTest(value=value['database_name']):
                law.journal_row(value)
        for value in (frameworks_row(expected=None), dict(frameworks_row(), rel='20990102_0',
                      database_name='frameworks_20990102_0', expected_active_db='frameworks_20990101_99')):
            with self.subTest(value=value['database_name']):
                frameworks.journal_row(value)


class OperationsBoundaryTest(KindFixture, unittest.TestCase):
    def test_operations_returns_valid_rows_and_refuses_the_whole_read(self):
        for name, kind, _, row in KINDS:
            db = self.database(kind)
            with self.subTest(kind=name):
                self.rows = [row(), prune_row(row())]
                self.assertEqual(db.operations(), self.rows)
                self.rows = [row(), dict(row(), step='unknown')]
                with self.assertRaises(law.JournalRefused):
                    db.operations()

    def test_a_non_list_read_is_refused(self):
        for name, kind, _, _row in KINDS:
            with self.subTest(kind=name):
                self.rows = {'rows': []}
                with self.assertRaises(law.JournalRefused):
                    self.database(kind).operations()


if __name__ == '__main__':
    unittest.main()
