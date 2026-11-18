import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock

from updater.corpus_verify_test import manifest
from updater.deploy_corpus import Corpus
from updater.deploy_corpus_database import Database
from updater.deploy_corpus_kind import FRAMEWORKS, LAW
from updater.deploy_corpus_lock import AdvisoryLock
from updater.deploy_host import Host
from updater.deploy_store import Paths
from updater.frameworks_steps_test import StepsFixture


CONNECTION = ('host=127.0.0.1 port=5999 dbname=x', 'postgresql://127.0.0.1:5999/x', 'postgres://127.0.0.1:5999/x')
MALFORMED = ('a=b', 'corpus vn', 'corpus_vn_20990102 x', 'corpus/vn', '-h', '--host=x', '', 'corpus_vn_20990102\n')
NON_STRINGS = (None, 7, ['auth'])
HOSTILE = CONNECTION + MALFORMED + NON_STRINGS
SHARED = ('auth', 'postgres')
LAW_NAME, FRAMEWORKS_NAME = 'corpus_vn_20990102_1', 'frameworks_20990102'


class GuardAssertions:
    def assert_refused(self, action, value):
        self.calls.clear()
        with self.assertRaises(ValueError) as raised:
            action()
        self.assertEqual(self.calls, [])
        if isinstance(value, str) and value:
            self.assertNotIn(value, str(raised.exception))


class LawFixture(GuardAssertions):
    def setUp(self):
        self.work = tempfile.TemporaryDirectory()
        self.addCleanup(self.work.cleanup)
        self.m = manifest()
        self.calls = []
        self.pointer = LAW_NAME.encode()
        self.host = Host(Paths(Path(self.work.name)), self.runner, user_prefix=[])
        self.db = Database(self.host, {'extensions': ['vector', 'ltree']})
        self.db.holder = mock.Mock()
        self.db.holder.poll.return_value = None
        self.row = dict(operation_id='e' * 32, country='vn', rel=self.m['rel'], mode='new', database_name=LAW_NAME,
                        manifest_hash='a' * 64, dump_sha256=self.m['dump']['sha256'],
                        expected_active_db='corpus_vn_20990101_1', manifest=self.m, step='checked')

    def runner(self, args, **kwargs):
        self.calls.append(args)
        text = (kwargs.get('input') or b'').decode()
        if 'release-check' in args:
            return b'{}'
        if 'pg_available_extensions' in text:
            return json.dumps(self.m['extensions']).encode()
        if 'FROM release WHERE singleton' in text:
            return json.dumps({key: self.m[key] for key in ('country', 'rel', 'schema_version', 'recipes')}).encode()
        if 'pg_get_userbyid(datdba) FROM pg_database' in text:
            return b'activator\n'
        if 'SELECT active_db' in text:
            return self.pointer
        if 'to_regclass' in text or 'document_summary' in text:
            return b't\n'
        if 'retired_release' in text or 'EXISTS(SELECT FROM pg_database' in text:
            return b'f\n'
        return b''

    def row_paths(self, value):
        row = dict(self.row, database_name=value)
        return dict(validate=lambda: self.db.validate(row),
                    projection=lambda: self.db.projection(dict(row, manifest=dict(self.m, schema_version=8))),
                    smoke=lambda: self.db.smoke(row, 'image'),
                    create=lambda: self.db.create(row),
                    restore=lambda: self.db.restore(row, '/synthetic/dump'))

    def name_paths(self, value):
        paths = dict(make=lambda: self.db.make(value), drop=lambda: self.db.drop(value),
                     load=lambda: self.db.load(value, '/synthetic/dump', '/run/pgbackrest/corpus-x.list'),
                     grant=lambda: self.db.grant(value))
        if value is not None:
            paths['describe'] = lambda: self.db.describe('image', value)
        return paths


class LawGuardTest(LawFixture, unittest.TestCase):
    def test_hostile_names_start_no_process_on_any_law_path(self):
        for value in HOSTILE + (FRAMEWORKS_NAME,):
            paths = dict(self.row_paths(value), **self.name_paths(value),
                         query=lambda: self.db.query('SELECT 1;', value))
            for name, action in paths.items():
                with self.subTest(path=name, value=value):
                    self.assert_refused(action, value)

    def test_shared_databases_are_refused_where_a_release_is_required(self):
        for value in SHARED:
            for name, action in dict(self.row_paths(value), **self.name_paths(value)).items():
                with self.subTest(path=name, value=value):
                    self.assert_refused(action, value)

    def test_valid_name_reaches_each_client(self):
        for name, action in dict(self.row_paths(LAW_NAME), **self.name_paths(LAW_NAME)).items():
            with self.subTest(path=name):
                self.calls.clear()
                action()
                self.assertTrue(any(LAW_NAME in args for args in self.calls))

    def test_query_accepts_only_its_shared_databases_and_the_release(self):
        for value in (*SHARED, LAW_NAME):
            self.calls.clear()
            self.db.query('SELECT 1;', value)
            self.assertEqual(self.calls[0][self.calls[0].index('-d') + 1], value)

    def test_active_pointer_is_checked_before_it_is_returned(self):
        for value in CONNECTION + (FRAMEWORKS_NAME, 'auth'):
            with self.subTest(value=value):
                self.pointer = value.encode()
                self.calls.clear()
                with self.assertRaises(ValueError):
                    self.db.current('vn')
                self.assertEqual(len(self.calls), 1)
                self.assertEqual(self.calls[0][-2:], ['-d', 'auth'])


class SmokeGuardTest(LawFixture, unittest.TestCase):
    def test_smoke_hostile_name_starts_no_postgres_session_on_either_kind(self):
        for kind in (LAW, FRAMEWORKS):
            db = Database(self.host, {'extensions': ['vector', 'ltree']}, kind)
            db.holder = self.db.holder
            for value in ('host=127.0.0.1 port=5999 dbname=x', 'postgresql://127.0.0.1:5999/x'):
                with self.subTest(kind=kind.name, value=value):
                    self.assert_refused(lambda: db.smoke(dict(self.row, database_name=value), 'image'), value)
                    self.assertFalse(any('-d' in args for args in self.calls))


class HostGuardTest(LawFixture, unittest.TestCase):
    def test_admin_psql_refuses_hostile_names_without_a_process(self):
        for value in HOSTILE:
            with self.subTest(value=value):
                self.assert_refused(lambda: self.host.psql(value, 'SELECT 1;'), value)

    def test_admin_psql_accepts_shared_databases_and_both_release_kinds(self):
        for value in (*SHARED, LAW_NAME, FRAMEWORKS_NAME):
            self.calls.clear()
            self.host.psql(value, 'SELECT 1;')
            self.assertEqual(self.calls[0][self.calls[0].index('-d') + 1], value)

    def test_hostile_active_pointer_stops_prewarm_after_the_read(self):
        for value in CONNECTION + ('corpus_vn_20990102 x', '-h', '', FRAMEWORKS_NAME, *SHARED):
            with self.subTest(value=value):
                self.pointer = value.encode() + b'\n'
                self.calls.clear()
                with self.assertRaises(ValueError) as raised:
                    self.host.postgres()
                self.assertEqual(self.calls[-1][self.calls[-1].index('-d') + 1], 'auth')
                self.assertFalse(any(value in args for args in self.calls[:-1] if value))
                if value:
                    self.assertNotIn(value, str(raised.exception))

    def test_valid_active_pointer_is_prewarmed(self):
        for value in (LAW_NAME, 'corpus_vn_20990102'):
            with self.subTest(value=value):
                self.pointer = value.encode() + b'\n'
                self.calls.clear()
                self.host.postgres()
                self.assertEqual(self.calls[-1][self.calls[-1].index('-d') + 1], value)


class LockGuardTest(LawFixture, unittest.TestCase):
    def test_lock_session_passes_the_guard_before_starting(self):
        self.db.target = mock.Mock(side_effect=ValueError('invalid corpus database name'))
        with mock.patch('updater.deploy_corpus_lock.subprocess.Popen') as start, self.assertRaises(ValueError):
            AdvisoryLock(self.db).__enter__()
        start.assert_not_called()


class ManifestHashGuardTest(unittest.TestCase):
    def test_journal_hash_is_checked_before_any_path_or_signature_command(self):
        with tempfile.TemporaryDirectory() as work:
            host = Host(Paths(Path(work)), mock.Mock(), user_prefix=[])
            phase = Corpus(mock.Mock(), mock.Mock(), host, {}, mock.Mock(), database=mock.Mock())
            phase.kind = mock.Mock()
            for value in ('../../etc', '/etc', 'A' * 64, 'a' * 63, '-x', '', None, 7):
                with self.subTest(value=value), self.assertRaises(ValueError):
                    phase.load(dict(manifest_hash=value))
            phase.kind.verify.assert_not_called()
            host.runner.assert_not_called()
            self.assertIsNone(phase.hash)


class FrameworksGuardTest(GuardAssertions, StepsFixture, unittest.TestCase):
    def step_paths(self, value):
        row = dict(self.row, database_name=value)
        paths = dict(validate=lambda: self.steps.validate(row), smoke=lambda: self.steps.smoke(row, 'image'),
                     create=lambda: self.steps.create(row), restore=lambda: self.steps.restore(row, '/d', 'image'),
                     session=lambda: self.steps.session('SELECT 1;', value),
                     settings=lambda: self.steps.settings(value),
                     admin=lambda: self.steps.admin(value, 'SELECT 1;'),
                     make=lambda: self.db.make(value), drop=lambda: self.db.drop(value),
                     load=lambda: self.db.load(value, '/d', '/run/pgbackrest/corpus-x.list'))
        if value is not None:
            paths['describe'] = lambda: self.db.describe('image', value)
        return paths

    def test_hostile_and_law_names_start_no_process(self):
        for value in HOSTILE + (LAW_NAME, *SHARED):
            for name, action in self.step_paths(value).items():
                with self.subTest(path=name, value=value):
                    self.assert_refused(action, value)

    def test_activator_query_refuses_hostile_and_law_names(self):
        for value in HOSTILE + (LAW_NAME,):
            with self.subTest(value=value):
                self.assert_refused(lambda: self.db.query('SELECT 1;', value), value)

    def test_active_pointer_is_checked_before_it_is_returned(self):
        for value in CONNECTION + (LAW_NAME, 'auth'):
            with self.subTest(value=value):
                self.calls.clear()
                with mock.patch.object(self.db, 'user', return_value=value.encode()) as user, \
                        self.assertRaises(ValueError):
                    self.db.current('vn')
                user.assert_called_once()

    def test_valid_name_reaches_each_client(self):
        self.steps.settings(self.name)
        self.steps.session("SELECT to_regclass('public.release');", self.name)
        self.steps.admin(self.name, 'SELECT 1;')
        self.steps.validate(self.row)
        self.steps.smoke(self.row, 'image')
        self.db.make(self.name)
        self.db.load(self.name, '/d', '/run/pgbackrest/corpus-x.list')
        for name in ('effective', 'restored', 'postgres', 'validate', 'release-check', 'createdb', 'pg_restore'):
            with self.subTest(client=name):
                self.assertIn(self.name, self.call(name)[0])

    def test_schema_version_must_be_a_positive_integer_before_the_container_starts(self):
        for value in ('1', '--help', '1 2', '', None, True, 0, -1, 1.5, [1]):
            with self.subTest(value=value):
                self.assert_refused(lambda: self.steps.schema('image', value), None)
        self.steps.schema('image', 1)
        self.assertEqual(self.call('schema')[0][-2:], ['--version', '1'])


if __name__ == '__main__':
    unittest.main()
