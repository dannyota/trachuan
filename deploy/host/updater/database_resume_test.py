import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock

from updater.corpus_verify_test import manifest
from updater.database_guard_test import CONNECTION, LAW_NAME
from updater.deploy_corpus_activate import Activation
from updater.deploy_corpus_database import Database
from updater.deploy_corpus_sql import JournalRefused
from updater.deploy_host import Host
from updater.deploy_store import Paths


ACTIVE = 'corpus_vn_20990101_1'
OPEN_STEPS = ('started', 'creating', 'created', 'restoring', 'restored', 'validated', 'checked')


class ResumeGuardTest(unittest.TestCase):
    def setUp(self):
        self.work = tempfile.TemporaryDirectory()
        self.addCleanup(self.work.cleanup)
        self.m = manifest()
        self.calls, self.scripts = [], []
        self.returning = None
        self.retired = b''
        self.db = Database(Host(Paths(Path(self.work.name)), self.runner, user_prefix=[]),
                           {'extensions': ['vector', 'ltree']})
        self.db.holder = mock.Mock()
        self.db.holder.poll.return_value = None
        self.engine = Activation(self.db)
        self.row = dict(operation_id='e' * 32, country='vn', rel=self.m['rel'], mode='new', database_name=LAW_NAME,
                        manifest_hash='a' * 64, dump_sha256=self.m['dump']['sha256'],
                        expected_active_db=ACTIVE, manifest=self.m, step='started')

    def runner(self, args, **kwargs):
        text = (kwargs.get('input') or b'').decode()
        self.calls.append(args)
        self.scripts.append(text)
        if 'release-check' in args:
            return b'{}'
        if 'INSERT INTO release_journal' in text:
            return json.dumps(self.returning).encode()
        if 'FROM release WHERE singleton' in text:
            return json.dumps({key: self.m[key] for key in ('country', 'rel', 'schema_version', 'recipes')}).encode()
        if 'DISTINCT ON' in text and 'row_to_json' not in text and 'json_agg' in text:
            return b'[]'
        if "json_build_object('dump_sha256'" in text:
            return self.retired
        if 'SELECT active_db' in text:
            return ACTIVE.encode()
        if 'EXISTS(SELECT FROM pg_database' in text:
            return b't\n'
        return b'f\n'

    def inserted_steps(self):
        return [step for text in self.scripts if 'INSERT INTO release_journal' in text
                for step in ('aborted', 'validated', 'checked', 'restoring') if "'" + step + "'" in text]

    def assert_no_client_for(self, value):
        for args in self.calls:
            self.assertFalse(any(value in arg for arg in args), args)
            self.assertNotIn('dropdb', args)
            if '-d' in args:
                self.assertIn(args[args.index('-d') + 1], ('auth', 'postgres', LAW_NAME))

    def test_row_with_hostile_name_is_refused_before_any_process(self):
        for value in CONNECTION:
            for step in OPEN_STEPS + ('aborted', 'activated', 'reverted'):
                for mode in ('new', 'retained'):
                    with self.subTest(value=value, step=step, mode=mode):
                        self.calls.clear()
                        with self.assertRaises(JournalRefused) as raised:
                            self.engine.resume(dict(self.row, database_name=value, step=step, mode=mode), '/d', 'image')
                        self.assertEqual(str(raised.exception), 'invalid release journal row')
                        self.assertEqual(self.calls, [])

    def test_name_swapped_by_the_journal_insert_stops_without_abort_client_or_drop(self):
        for value in CONNECTION:
            for step, returned in (('created', 'restoring'), ('restored', 'validated'), ('validated', 'checked')):
                with self.subTest(value=value, step=step):
                    self.calls.clear()
                    self.scripts.clear()
                    self.returning = dict(self.row, database_name=value, step=returned)
                    with self.assertRaises(JournalRefused):
                        self.engine.resume(dict(self.row, step=step), '/d', 'image')
                    self.assertNotIn('aborted', self.inserted_steps())
                    self.assert_no_client_for(value)

    def test_retained_row_swapped_before_validate_starts_no_client(self):
        self.retired = json.dumps(dict(dump_sha256=self.row['dump_sha256'], live=True)).encode()
        for value in CONNECTION:
            with self.subTest(value=value):
                self.calls.clear()
                self.scripts.clear()
                self.returning = dict(self.row, mode='retained', database_name=value, step='restored')
                with self.assertRaises(JournalRefused):
                    self.engine.resume(dict(self.row, mode='retained'), '/d', 'image')
                self.assertNotIn('aborted', self.inserted_steps())
                self.assert_no_client_for(value)


if __name__ == '__main__':
    unittest.main()
