import unittest
from unittest import mock

from updater.corpus_activation_test import Crash, Database as LawDatabase
from updater.deploy_corpus_activate import Activation
from updater.deploy_corpus_kind import FRAMEWORKS
from updater.deploy_frameworks_steps import Steps
from updater.deploy_io import CommandError
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

    def restore(self, row, dump, image=None):
        self.steps.append('restore')
        super().restore(row, dump, image)

    def validate(self, row):
        self.steps.append('validate')
        super().validate(row)

    def smoke(self, row, image):
        self.steps.append('smoke')

    def inspect(self, row, dump, image=None):
        self.steps.append('inspect')


class FrameworksActivationTest(unittest.TestCase):
    def setUp(self):
        self.db = Database()
        self.engine = Activation(self.db, FRAMEWORKS)
        self.engine.steps = self.db
        self.m = manifest()
        self.hash = 'e' * 64

    def test_target_activator_does_not_own_is_refused_and_survives_an_abort(self):
        self.m['previous'] = None
        name = self.m['database']
        self.db.databases.add(name)
        self.db.foreign.add(name)
        with self.assertRaisesRegex(ValueError, '^new frameworks target already exists$'):
            self.engine.choose(self.m)
        self.assertFalse(self.db.rows)
        self.engine.abort(self.engine.begin(self.hash, self.m, 'new', None))
        self.assertEqual(self.db.operations()[0]['step'], 'aborted')
        self.assertEqual(self.db.dropped, [])
        self.assertTrue(self.db.exists(name))

    def test_owned_unprotected_target_reaches_the_create_step(self):
        self.m['previous'] = None
        self.db.databases.add(self.m['database'])
        self.assertEqual(self.engine.choose(self.m), ('new', None))

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

    def test_kind_uses_frameworks_steps_without_a_restore_refusal(self):
        self.assertIsInstance(FRAMEWORKS.steps(self.db), Steps)
        self.assertFalse(hasattr(FRAMEWORKS, 'admit'))

    def test_bootstrap_new_restores_validates_smokes_then_inserts_pointer(self):
        self.m['previous'] = None
        row = self.activate()
        self.assertEqual(row['step'], 'activated')
        self.assertEqual(self.db.steps, ['inspect', 'inspect', 'create', 'restore', 'validate', 'smoke'])
        self.assertEqual([r['step'] for r in self.db.rows], ['started', 'creating', 'created', 'restoring', 'restored',
                                                             'validated', 'checked', 'activated'])
        first = self.db.rows[0]
        self.assertEqual((first['country'], first['rel'], first['database_name'], first['expected_active_db'], first['mode']),
                         ('vn', self.m['rel'], self.m['database'], None, 'new'))
        self.assertEqual(self.db.active, self.m['database'])
        self.assertEqual(self.db.dropped, [])

    def test_failed_restore_aborts_and_drops_only_the_new_target(self):
        self.m['previous'] = None
        self.db.restore = mock.Mock(side_effect=CommandError('command failed', detail='pg_restore: exit 1'))
        with self.assertRaises(CommandError):
            self.activate()
        self.assertEqual([r['step'] for r in self.db.rows][-1], 'aborted')
        self.assertEqual(self.db.dropped, [self.m['database']])
        self.assertEqual(self.db.lists_removed, [self.db.rows[0]['operation_id']])
        self.assertIsNone(self.db.active)
        del self.db.restore
        self.db.bad_release = True
        self.db.rows.clear()
        self.db.dropped.clear()
        with self.assertRaisesRegex(ValueError, 'release row differs'):
            self.activate()
        self.assertEqual(self.db.dropped, [self.m['database']])
        self.assertIsNone(self.db.active)

    def test_retained_activation_validates_and_smokes_without_restore(self):
        name = self.m['database']
        self.db.active = 'frameworks_20990101'
        self.db.databases.update((name, self.db.active))
        self.db.retired[name] = dict(dump_sha256=self.m['dump']['sha256'], live=True)
        self.activate()
        self.assertEqual(self.db.steps, ['validate', 'smoke'])
        self.assertEqual([r['step'] for r in self.db.rows], ['started', 'restored', 'validated', 'checked', 'activated'])
        self.assertEqual(self.db.dropped, [])
        self.assertEqual(self.db.active, name)

    def test_bootstrap_guard_holds_until_a_pointer_appears(self):
        self.m['previous'] = None
        row = self.engine.begin(self.hash, self.m, 'new', None)
        self.assertTrue(self.engine.guarded(row))
        self.db.active = 'frameworks_20990101'
        self.assertFalse(self.engine.guarded(row))
        self.db.active = None
        self.assertFalse(self.engine.guarded(dict(row, mode='retained')))

    def test_recheck_on_resume_validates_and_smokes_again_before_the_pointer(self):
        self.m['previous'] = None
        row = self.engine.begin(self.hash, self.m, 'new', None)
        row = self.db.append(row, 'checked')
        self.assertEqual(self.engine.resume(row, None, 'server-image', recheck=True)['step'], 'activated')
        self.assertEqual(self.db.steps, ['validate', 'smoke'])
        self.assertEqual(self.db.active, self.m['database'])

    def test_crash_after_each_new_step_resumes_with_law_recovery_rules(self):
        self.m['previous'] = None
        for step in ('started', 'creating', 'created', 'restoring', 'restored', 'validated', 'checked'):
            with self.subTest(step=step):
                self.setUp()
                self.m['previous'] = None
                self.db.after = step
                with self.assertRaises(Crash):
                    self.activate()
                row = self.db.operations()[0]
                self.assertTrue(self.engine.guarded(row))
                self.assertEqual(self.engine.resume(row, '/synthetic/dump', 'server-image', recheck=True)['step'],
                                 'activated')
                self.assertEqual(self.db.active, self.m['database'])
                self.assertEqual(self.db.dropped, [])

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
