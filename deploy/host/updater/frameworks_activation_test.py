import unittest

from updater.corpus_activation_test import Database as LawDatabase
from updater.deploy_corpus_activate import Activation
from updater.deploy_corpus_kind import FRAMEWORKS
from updater.frameworks_verify_test import manifest


class Database(LawDatabase):
    def __init__(self):
        super().__init__()
        self.active = None
        self.databases = set()
        self.steps = []
        self.countries = []

    def current(self, country):
        self.countries.append(country)
        return self.active

    def create(self, row):
        self.steps.append('create')
        super().create(row)

    def restore(self, row, dump):
        self.steps.append('restore')

    def validate(self, row):
        self.steps.append('validate')

    def smoke(self, row, image):
        self.steps.append('smoke')

    def inspect(self, row, dump):
        self.steps.append('inspect')


class FrameworksActivationTest(unittest.TestCase):
    def setUp(self):
        self.db = Database()
        self.engine = Activation(self.db, FRAMEWORKS)
        self.m = manifest()
        self.hash = 'e' * 64

    def activate(self):
        return self.engine.run(self.hash, self.m, '/synthetic/dump', 'server-image')

    def test_bootstrap_new_requires_null_previous_without_pointer(self):
        self.m['previous'] = None
        self.assertEqual(self.engine.choose(self.m), ('new', None))
        self.m['previous'] = '20990101'
        with self.assertRaisesRegex(ValueError, 'new frameworks previous does not equal active release'):
            self.engine.choose(self.m)
        self.assertEqual(set(self.db.countries), {'vn'})

    def test_new_with_pointer_requires_previous_active_and_refuses_null(self):
        self.db.active = 'frameworks_20990101'
        self.db.databases.add(self.db.active)
        self.assertEqual(self.engine.choose(self.m), ('new', 'frameworks_20990101'))
        for previous in (None, '20990100'):
            self.m['previous'] = previous
            with self.subTest(previous=previous), self.assertRaisesRegex(ValueError, 'previous does not equal'):
                self.engine.choose(self.m)
        self.assertFalse(self.db.rows)

    def test_retained_needs_pointer_live_retention_and_matching_digest(self):
        name = self.m['database']
        self.db.databases.add(name)
        self.db.retired[name] = dict(dump_sha256=self.m['dump']['sha256'], live=True)
        with self.assertRaisesRegex(ValueError, 'retained database unavailable'):
            self.engine.choose(self.m)
        self.db.active = 'frameworks_20990101'
        self.m['previous'] = None
        self.assertEqual(self.engine.choose(self.m), ('retained', 'frameworks_20990101'))
        self.db.retired[name]['dump_sha256'] = 'f' * 64
        with self.assertRaisesRegex(ValueError, 'digest'):
            self.engine.choose(self.m)

    def test_restore_hooks_refuse_before_creating_any_database(self):
        self.m['previous'] = None
        with self.assertRaisesRegex(ValueError, '^frameworks restore not implemented$'):
            self.activate()
        self.assertEqual(self.db.steps, [])
        self.assertEqual(self.db.created, 0)
        self.assertEqual(self.db.dropped, [])
        self.assertEqual([r['step'] for r in self.db.rows], ['started', 'aborted'])
        row = self.db.rows[0]
        self.assertEqual((row['country'], row['rel'], row['database_name'], row['expected_active_db'], row['mode']),
                         ('vn', self.m['rel'], self.m['database'], None, 'new'))
        self.assertIsNone(self.db.active)

    def test_retained_activation_refuses_at_validation_without_dropping(self):
        name = self.m['database']
        self.db.active = 'frameworks_20990101'
        self.db.databases.update((name, self.db.active))
        self.db.retired[name] = dict(dump_sha256=self.m['dump']['sha256'], live=True)
        with self.assertRaisesRegex(ValueError, '^frameworks restore not implemented$'):
            self.activate()
        self.assertEqual(self.db.steps, [])
        self.assertEqual([r['step'] for r in self.db.rows], ['started', 'restored', 'aborted'])
        self.assertEqual(self.db.dropped, [])
        self.assertEqual(self.db.active, 'frameworks_20990101')

    def test_bootstrap_guard_holds_until_a_pointer_appears(self):
        self.m['previous'] = None
        row = self.engine.begin(self.hash, self.m, 'new', None)
        self.assertTrue(self.engine.guarded(row))
        self.db.active = 'frameworks_20990101'
        self.assertFalse(self.engine.guarded(row))
        self.db.active = None
        self.assertFalse(self.engine.guarded(dict(row, mode='retained')))

    def test_recheck_on_resume_uses_kind_hooks(self):
        self.m['previous'] = None
        row = self.engine.begin(self.hash, self.m, 'new', None)
        row = self.db.append(row, 'checked')
        with self.assertRaisesRegex(ValueError, '^frameworks restore not implemented$'):
            self.engine.resume(row, None, 'server-image', recheck=True)
        self.assertEqual(self.db.steps, [])
        self.assertEqual(self.db.operations()[0]['step'], 'aborted')

    def test_prune_parses_frameworks_names(self):
        expired = 'frameworks_20990101_12'
        self.db.retired[expired] = dict(dump_sha256=None, live=False)
        self.db.databases.add(expired)
        self.engine.prune()
        self.assertEqual(self.db.dropped, [expired])
        pruning = [r for r in self.db.rows if r['step'] == 'pruning'][0]
        self.assertEqual((pruning['country'], pruning['rel'], pruning['mode']), ('vn', '20990101_12', 'prune'))
        self.assertNotIn(expired, self.db.retired)


if __name__ == '__main__':
    unittest.main()
