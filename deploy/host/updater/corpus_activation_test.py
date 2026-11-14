import copy
from contextlib import nullcontext
import unittest

from updater.corpus_verify_test import manifest
from updater import deploy_corpus_activate as a


class Crash(BaseException):
    pass


class Database:
    def __init__(self):
        self.active = 'corpus_vn_20990101_1'
        self.databases = {self.active}
        self.retired = {}
        self.rows = []
        self.dropped = []
        self.after = None
        self.before = None
        self.bad_release = False
        self.created = 0

    def lock(self):
        return nullcontext()

    def append(self, row, step):
        row = dict(row, step=step)
        self.rows.append(row)
        if step == self.after:
            self.after = None
            raise Crash()
        return row

    def operations(self):
        latest = {}
        for row in self.rows:
            latest[row['operation_id']] = row
        return list(latest.values())

    def current(self, country):
        return self.active

    def retirement(self, name):
        return self.retired.get(name)

    def protected(self, name):
        return (name == self.active or self.retired.get(name, {}).get('live', False)
                or any(r['database_name'] == name and r['mode'] != 'prune'
                       and r['step'] not in a.COMPLETE for r in self.operations()))

    def exists(self, name):
        return name in self.databases

    def connections(self, name):
        return False

    def drop(self, name):
        if self.protected(name):
            raise AssertionError('protected database dropped')
        self.databases.discard(name)
        self.dropped.append(name)
        if self.after == 'drop':
            self.after = None
            raise Crash()

    def create(self, row):
        self.created += 1
        self.databases.add(row['database_name'])

    def inspect(self, row, dump):
        pass

    def restore(self, row, dump):
        if self.before == 'restore':
            self.before = None
            raise Crash()

    def validate(self, row):
        if self.bad_release:
            raise ValueError('restored release row differs from manifest')

    def smoke(self, row, image):
        pass

    def activate(self, row):
        if self.active != row['expected_active_db']:
            raise ValueError('active database changed')
        if self.active:
            self.retired[self.active] = dict(dump_sha256=None, live=True)
        self.active = row['database_name']
        self.retired.pop(self.active, None)
        return self.append(row, 'activated')

    def expired(self):
        return [name for name, row in self.retired.items() if not row['live']]

    def forget(self, name):
        self.retired.pop(name, None)
        if self.after == 'forget':
            self.after = None
            raise Crash()


class CorpusActivationTest(unittest.TestCase):
    def setUp(self):
        self.db = Database()
        self.engine = a.Activation(self.db)
        self.m = manifest()
        self.hash = 'e'*64

    def activate(self):
        return self.engine.run(self.hash, self.m, '/synthetic/dump', 'server-image')

    def test_each_journal_step_resumes_without_repeating_complete_activation(self):
        for step in ('started', 'creating', 'created', 'restoring', 'restored', 'validated', 'checked', 'activated'):
            with self.subTest(step=step):
                self.setUp()
                self.db.after = step
                with self.assertRaises(Crash):
                    self.activate()
                self.activate()
                self.assertEqual(self.db.active, self.m['database'])
                self.assertEqual(self.db.operations()[0]['step'], 'activated')
                self.assertEqual(self.db.retired['corpus_vn_20990101_1']['dump_sha256'], None)
                before = copy.deepcopy(self.db.rows)
                self.activate()
                self.assertEqual(self.db.rows, before)

    def test_crash_during_transactional_restore_retries_target(self):
        self.db.before = 'restore'
        with self.assertRaises(Crash):
            self.activate()
        self.activate()
        self.assertEqual(self.db.created, 1)
        self.assertEqual(self.db.dropped, [])

    def test_changed_guards_abort_after_every_open_step_without_dropping_protected(self):
        for step in ('started', 'creating', 'created', 'restoring', 'restored', 'validated', 'checked'):
            for protected in ('active', 'retired', 'another_journal', 'none'):
                with self.subTest(step=step, protected=protected):
                    self.setUp()
                    self.db.after = step
                    with self.assertRaises(Crash):
                        self.activate()
                    if protected == 'active':
                        self.db.active = self.m['database']
                    else:
                        self.db.active = 'corpus_vn_20990103_1'
                    if protected == 'retired':
                        self.db.retired[self.m['database']] = dict(dump_sha256=None, live=True)
                    if protected == 'another_journal':
                        self.db.rows.append(dict(self.db.rows[0], operation_id='f'*32))
                    existed = self.db.exists(self.m['database'])
                    self.engine.resume(self.db.operations()[0], '/synthetic/dump', 'server-image')
                    self.assertEqual(self.db.operations()[0]['step'], 'aborted')
                    self.assertEqual(bool(self.db.dropped), protected == 'none' and existed)

    def test_mode_refuses_null_or_different_digest_and_new_wrong_previous(self):
        for digest in (None, 'f'*64):
            self.db.retired[self.m['database']] = dict(dump_sha256=digest, live=True)
            with self.subTest(digest=digest), self.assertRaises(ValueError):
                self.activate()
        self.db.retired.clear()
        self.m['previous'] = '20990105_1'
        with self.assertRaises(ValueError):
            self.activate()
        self.assertFalse(self.db.rows)

    def test_retained_rollback_requires_live_retention_and_matching_digest(self):
        self.db.retired[self.m['database']] = dict(dump_sha256=self.m['dump']['sha256'], live=True)
        self.db.databases.add(self.m['database'])
        self.m['previous'] = '20990105_1'
        self.assertEqual(self.activate()['mode'], 'retained')
        self.assertEqual(self.db.created, 0)
        self.assertEqual(self.db.active, self.m['database'])

    def test_release_row_mismatch_closes_journal_then_drops_target(self):
        self.db.bad_release = True
        with self.assertRaises(ValueError):
            self.activate()
        self.assertEqual(self.db.operations()[0]['step'], 'aborted')
        self.assertEqual(self.db.dropped, [self.m['database']])

    def test_prune_resumes_drop_row_delete_gap_and_never_drops_protected(self):
        expired = 'corpus_vn_20990104_1'
        self.db.retired[expired] = dict(dump_sha256=None, live=False)
        self.db.retired[self.db.active] = dict(dump_sha256=None, live=False)
        self.db.retired[self.m['database']] = dict(dump_sha256=None, live=True)
        self.db.databases.add(expired)
        self.db.after = 'drop'
        with self.assertRaises(Crash):
            self.engine.prune()
        self.engine.prune()
        self.assertNotIn(expired, self.db.retired)
        self.assertEqual(self.db.dropped, [expired])
        self.assertIn(self.db.active, self.db.retired)
        self.assertIn(self.m['database'], self.db.retired)

    def test_prune_interrupted_before_and_after_each_journal_row(self):
        for step in ('pruning', 'forget', 'pruned'):
            self.setUp()
            name = 'corpus_vn_20990104_1'
            self.db.retired[name] = dict(dump_sha256=None, live=False)
            self.db.databases.add(name)
            self.db.after = step
            with self.assertRaises(Crash):
                self.engine.prune()
            self.engine.prune()
            self.assertNotIn(name, self.db.retired)
            self.assertEqual(self.db.dropped, [name])

    def test_retained_guard_abort_keeps_target_even_after_retirement_row_is_lost(self):
        self.db.retired[self.m['database']] = dict(dump_sha256=self.m['dump']['sha256'], live=True)
        self.db.databases.add(self.m['database'])
        self.db.after = 'validated'
        with self.assertRaises(Crash):
            self.activate()
        self.db.retired.clear()
        result = self.activate()
        self.assertEqual(result['step'], 'aborted')
        self.assertTrue(self.db.exists(self.m['database']))
        self.assertEqual(self.db.dropped, [])

    def test_prune_keeps_open_activation_target_and_databases_with_connections(self):
        name = self.m['database']
        self.db.retired[name] = dict(dump_sha256=self.m['dump']['sha256'], live=False)
        self.db.databases.add(name)
        row = self.engine.begin(self.hash, self.m, 'new', self.db.active)
        self.engine.prune()
        self.assertFalse(self.db.dropped)
        self.db.append(row, 'aborted')
        self.db.connections = lambda _: True
        self.engine.prune()
        self.assertFalse(self.db.dropped)
