import unittest

from updater import deploy_corpus_sql as law
from updater.deploy_corpus_activate import Activation
from updater.journal_row_test import BREAKOUT, KINDS, KindFixture, prune_row, stored


STORED = dict(id=9, at='2026-11-14T00:00:00+00:00')
CHANGES = dict(law=dict(operation_id='f' * 32, country='us', rel='20990103_1', mode='retained',
                        database_name='corpus_vn_20990103_1', manifest_hash='b' * 64, dump_sha256='c' * 64,
                        expected_active_db='corpus_vn_20990100_1', step='validated', manifest={'rel': 'changed'},
                        state='complete', message='forged', corpus_kind='frameworks'),
               frameworks=dict(operation_id='f' * 32, rel='20990103', mode='retained',
                               database_name='frameworks_20990103', manifest_hash='b' * 64, dump_sha256='c' * 64,
                               expected_active_db='frameworks_20990100', step='validated', manifest={'rel': 'changed'},
                               state='complete', message='forged', corpus_kind='law'))


class ReturnedRowTest(KindFixture, unittest.TestCase):
    def test_append_refuses_a_change_in_any_single_field(self):
        for name, kind, _, row in KINDS:
            db = self.database(kind)
            inserted = stored(kind, row(), 'restoring')
            for field, value in [*CHANGES[name].items(), ('country', BREAKOUT)]:
                with self.subTest(kind=name, field=field, value=value):
                    self.calls.clear()
                    self.journal_reply = dict(inserted, **{field: value})
                    with self.assertRaises(law.JournalRefused) as raised:
                        db.append(row(), 'restoring')
                    self.assertEqual(str(raised.exception), 'invalid release journal row')
                    self.assertEqual(len(self.calls), 1)

    def test_activate_refuses_a_change_in_any_single_field(self):
        for name, kind, _, row in KINDS:
            db = self.database(kind)
            inserted = stored(kind, row(), 'activated')
            for field, value in dict(CHANGES[name], step='checked', state='open').items():
                with self.subTest(kind=name, field=field):
                    self.calls.clear()
                    self.journal_reply = dict(inserted, **{field: value})
                    with self.assertRaises(law.JournalRefused):
                        db.activate(row())
                    self.assertEqual(len(self.calls), 1)

    def test_written_fields_must_be_present(self):
        for name, kind, _, row in KINDS:
            for field in ('state', 'message', 'corpus_kind'):
                with self.subTest(kind=name, field=field):
                    reply = stored(kind, row(), 'restoring')
                    del reply[field]
                    self.journal_reply = reply
                    with self.assertRaises(law.JournalRefused):
                        self.database(kind).append(row(), 'restoring')

    def test_json_that_is_not_a_row_is_refused(self):
        for name, kind, _, row in KINDS:
            db = self.database(kind)
            for reply in (None, [], [stored(kind, row(), 'restoring')], 'row', 7, True):
                with self.subTest(kind=name, reply=reply):
                    self.journal_reply = reply
                    with self.assertRaises(law.JournalRefused) as raised:
                        db.append(row(), 'restoring')
                    self.assertEqual(str(raised.exception), 'invalid release journal row')
                    self.journal_reply = reply
                    with self.assertRaises(law.JournalRefused):
                        db.activate(row())

    def test_null_returned_for_a_supplied_value_is_refused(self):
        for name, kind, _, row in KINDS:
            with self.subTest(kind=name):
                self.journal_reply = dict(stored(kind, row(), 'restoring'), expected_active_db=None)
                with self.assertRaises(law.JournalRefused):
                    self.database(kind).append(row(), 'restoring')

    def test_insert_that_returns_no_row_is_refused(self):
        for name, kind, _, row in KINDS:
            with self.subTest(kind=name):
                self.journal_reply = b''
                with self.assertRaises(law.JournalRefused):
                    self.database(kind).append(row(), 'restoring')

    def test_updater_continues_with_its_own_values(self):
        for name, kind, _, row in KINDS:
            db = self.database(kind)
            with self.subTest(kind=name):
                self.journal_reply = dict(stored(kind, row(), 'restoring'), **STORED)
                result = db.append(row(), 'restoring')
                self.assertEqual(result, dict(row(), step='restoring'))
                self.assertNotIn('id', result)
                self.journal_reply = dict(stored(kind, row(), 'activated'), **STORED)
                self.assertEqual(db.activate(row()), dict(row(), step='activated'))

    def test_prune_rows_round_trip(self):
        for name, kind, _, row in KINDS:
            with self.subTest(kind=name):
                self.journal_reply = dict(stored(kind, prune_row(row()), 'pruned'), **STORED)
                self.assertEqual(self.database(kind).append(prune_row(row()), 'pruned'),
                                 dict(prune_row(row()), step='pruned'))


class TamperedResumeTest(KindFixture, unittest.TestCase):
    def inserted(self):
        return [text for text in self.scripts if 'INSERT INTO release_journal' in text]

    def test_changed_insert_stops_resume_without_abort_or_cleanup(self):
        for name, kind, _, row in KINDS:
            engine = Activation(self.database(kind), kind)
            self.active = row()['expected_active_db']
            start = dict(row(), step='created')
            for field, value in dict(CHANGES[name], country=BREAKOUT).items():
                with self.subTest(kind=name, field=field):
                    self.calls.clear()
                    self.scripts.clear()
                    self.journal_reply = dict(stored(kind, start, 'restoring'), **{field: value})
                    with self.assertRaises(law.JournalRefused):
                        engine.resume(start, '/d', 'image')
                    self.assertEqual(len(self.inserted()), 1)
                    self.assertNotIn("'aborted'", self.inserted()[0])
                    self.assertFalse(any('dropdb' in args or 'rm' in args for args in self.calls))
                    self.assertIn('INSERT INTO release_journal', self.scripts[-1])

    def test_changed_abort_insert_stops_before_any_drop(self):
        for name, kind, _, row in KINDS:
            engine = Activation(self.database(kind), kind)
            with self.subTest(kind=name):
                self.calls.clear()
                self.scripts.clear()
                self.journal_reply = dict(stored(kind, row(), 'aborted'), rel=CHANGES[name]['rel'])
                with self.assertRaises(law.JournalRefused):
                    engine.abort(row())
                self.assertFalse(any('dropdb' in args or 'rm' in args for args in self.calls))
                self.assertIn('INSERT INTO release_journal', self.scripts[-1])

    def test_changed_prune_insert_drops_nothing(self):
        for name, kind, _, row in KINDS:
            db = self.database(kind)
            with self.subTest(kind=name):
                self.calls.clear()
                self.journal_reply = dict(stored(kind, prune_row(row()), 'pruning'), rel=CHANGES[name]['rel'])
                with self.assertRaises(law.JournalRefused):
                    db.append(prune_row(row()), 'pruning')
                self.assertEqual(len(self.calls), 1)


if __name__ == '__main__':
    unittest.main()
