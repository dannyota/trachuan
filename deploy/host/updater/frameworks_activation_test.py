import unittest

from updater.deploy_corpus_kind import FRAMEWORKS
from updater.deploy_frameworks_steps import Steps
from updater.deploy_io import CommandError
from updater.world_test import FrameworksCase, World


STEPS = ['inspect', 'inspect', 'create', 'restore', 'validate', 'smoke']


class FrameworksActivationTest(FrameworksCase, unittest.TestCase):
    def bootstrap(self):
        self.m['previous'] = None
        self.write_manifest()

    def test_kind_uses_frameworks_steps_without_a_restore_refusal(self):
        self.assertIsInstance(FRAMEWORKS.steps(World(FRAMEWORKS)), Steps)
        self.assertFalse(hasattr(FRAMEWORKS, 'admit'))

    def test_bootstrap_new_needs_null_previous_and_no_pointer(self):
        self.bootstrap()
        self.assertTrue(self.run_phase())
        self.assertEqual(self.result()['status'], 'ok')
        self.assertEqual(self.db.log, STEPS)
        self.assertEqual([row['step'] for row in self.db.rows],
                         ['started', 'creating', 'created', 'restoring', 'restored', 'validated', 'checked', 'activated'])
        first = self.db.rows[0]
        self.assertEqual((first['country'], first['rel'], first['database_name'], first['expected_active_db'], first['mode']),
                         ('vn', self.m['rel'], self.m['database'], None, 'new'))
        self.assertEqual(self.db.pointer_value, self.m['database'])
        self.assertEqual(self.db.dropped, [])
        self.assertIsNone(self.db.swaps[-1]['retired'])

    def test_new_without_a_pointer_refuses_a_previous_release(self):
        self.assertTrue(self.run_phase())
        self.assertEqual(self.result()['status'], 'waiting')
        self.assertEqual(self.result()['message'], 'new frameworks previous does not equal active release')
        self.assertEqual(self.db.rows, [])

    def test_a_pointer_refuses_a_null_previous(self):
        self.baseline()
        self.bootstrap()
        self.assertTrue(self.run_phase())
        self.assertEqual(self.result()['status'], 'waiting')
        self.assertEqual(self.db.rows, [])

    def test_retained_needs_a_pointer(self):
        oid = self.db.add(self.name)
        self.write_record(retired=[dict(database=self.name, oid=oid, dump_sha256=self.dump,
                                        retired_at='2027-01-15T07:00:00Z')])
        self.m['previous'] = None
        self.write_manifest()
        self.assertTrue(self.run_phase())
        self.assertEqual((self.result()['status'], self.result()['message']), ('waiting', 'frameworks pointer missing'))
        self.assertEqual(self.db.rows, [])

    def test_failed_restore_aborts_and_drops_only_the_new_target(self):
        self.bootstrap()
        self.db.failures['restore'] = CommandError('command failed', detail='pg_restore: exit 1')
        self.assertFalse(self.run_phase())
        self.assertEqual(self.db.rows[-1]['step'], 'aborted')
        self.assertEqual(self.db.dropped, [self.m['database']])
        self.assertEqual(self.db.lists_removed, [self.db.rows[0]['operation_id']])
        self.assertIsNone(self.db.pointer_value)
        self.assertEqual(self.status(), 'failed')

    def test_retained_activation_validates_and_smokes_without_restore(self):
        self.baseline()
        oid = self.db.add(self.name)
        self.write_record(active=self.active_entry(),
                          retired=[dict(database=self.name, oid=oid, dump_sha256=self.dump,
                                        retired_at='2027-01-15T07:00:00Z')])
        self.assertTrue(self.run_phase())
        self.assertEqual(self.db.log, ['validate', 'smoke'])
        self.assertEqual([row['step'] for row in self.db.rows], ['started', 'restored', 'validated', 'checked', 'activated'])
        self.assertEqual(self.db.dropped, [])
        self.assertEqual(self.db.pointer_value, self.name)


if __name__ == '__main__':
    unittest.main()
