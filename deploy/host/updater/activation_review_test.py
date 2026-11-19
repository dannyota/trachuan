import json
import unittest

from updater.deploy_activation_record import stamp
from updater.deploy_io import canonical
from updater.deploy_release_target import LockBusy, LockLost, LockUnavailable, Unreachable, Waiting
from updater.world_test import FrameworksCase, LawCase, NOW


DAY = 86400
CATALOG_SESSIONS = ('catalog', 'exists', 'owned', 'connections', 'clear_leftovers', 'drop_database',
                    'other_superuser', 'check_extensions', 'create')
TARGET_SESSIONS = ('inspect', 'restore', 'validate', 'smoke', 'verify_target', 'check_lock')


class Helpers:
    def stray(self, rel='20990120'):
        return self.prefix + rel

    def retired(self, name, oid, age=0, dump=None):
        return dict(database=name, oid=oid, dump_sha256=dump or self.dump, retired_at=stamp(NOW - age))

    def statuses(self):
        return {record['hash']: record['status'] for record in self.records_of_kind().values()}

    def skip_prune(self):
        (self.paths.state / self.kind.marker).write_bytes(canonical('2027-01-15'))

    def own_rows(self, operation='c' * 32):
        return [row['step'] for row in self.db.rows if row['operation_id'] == operation]


class SessionFaults(Helpers):
    def inject(self, names, error):
        self.session_calls, self.fault_at, self.fired = 0, None, False
        for name in names:
            original = getattr(self.db, name)
            def wrapped(*args, original=original, **kwargs):
                self.session_calls += 1
                if self.fault_at == self.session_calls:
                    self.fired = True
                    raise error
                return original(*args, **kwargs)
            setattr(self.db, name, wrapped)

    def no_abort(self):
        self.assertNotIn('aborted', [row['step'] for row in self.db.rows])
        self.assertNotIn(self.digest, self.state.failed(self.kind.key))
        if self.path.exists():
            self.assertNotEqual((self.stored()['operation'] or {}).get('step'), 'aborted')

    def every_fault(self, names, error, prepare):
        position = 1
        while position < 400:
            self.setUp()
            prepare()
            self.inject(names, error)
            self.fault_at = position
            outcome = self.run_phase()
            if not self.fired:
                self.assertEqual(self.result()['status'], 'ok')
                return position
            with self.subTest(position=position, error=type(error).__name__):
                self.assertTrue(outcome)
                status = self.result()['status']
                self.assertIn(status, ('waiting', 'ok'))
                if status == 'ok':
                    self.assertEqual(self.result()['message'], self.kind.label + ' release activated')
                    self.assertGreater(self.db.tidied, 0)
                self.no_abort()
                self.fault_at = None
                self.assertTrue(self.again())
                self.assertEqual(self.result()['status'], 'current' if status == 'ok' else 'ok')
                self.assertEqual(self.db.pointer_value, self.name)
                self.assertEqual(self.status(), 'active')
                self.no_abort()
            position += 1
        self.fail('fault positions never ran out')

    def new_activation(self):
        self.baseline()

    def retained_activation(self):
        self.baseline()
        oid = self.db.add(self.name)
        self.write_record(active=self.active_entry(), retired=[self.retired(self.name, oid)])

    def test_a_catalog_session_that_cannot_connect_at_any_point_aborts_nothing(self):
        for prepare in (self.new_activation, self.retained_activation):
            self.assertGreater(self.every_fault(CATALOG_SESSIONS, Unreachable('activator session unavailable'),
                                                prepare), 5)

    def test_a_lock_session_lost_at_any_point_aborts_nothing(self):
        for prepare in (self.new_activation, self.retained_activation):
            self.assertGreater(self.every_fault(CATALOG_SESSIONS + TARGET_SESSIONS,
                                                LockLost('activation advisory lock lost'), prepare), 5)


class LockOutcomes(Helpers):
    def test_a_lock_held_by_an_activator_session_only_waits(self):
        self.baseline()
        self.write_record(active=self.active_entry())
        self.db.lock_error = LockBusy('activation advisory lock busy')
        self.assertTrue(self.run_phase())
        self.assertEqual((self.result()['status'], self.result()['integrity']), ('waiting', []))
        self.assertEqual(self.db.rows, [])

    def test_an_unavailable_lock_waits_and_raises_its_alert(self):
        self.baseline()
        self.write_record(active=self.active_entry())
        self.db.lock_error = LockUnavailable('activation lock unavailable')
        self.assertTrue(self.run_phase())
        self.assertEqual((self.result()['status'], self.result()['integrity']), ('waiting', ['lock_unavailable']))
        self.assertIn('lock_unavailable', self.stored()['alerts'])
        self.assertEqual(self.db.rows, [])
        self.registry.fetch.assert_not_called()

    def test_a_first_ever_run_that_meets_a_refused_lock_seeds_the_record_and_keeps_the_alert(self):
        self.baseline()
        self.assertFalse(self.path.exists())
        self.db.lock_error = LockUnavailable('activator cannot connect to auth')
        self.assertTrue(self.run_phase())
        self.assertEqual((self.result()['status'], self.result()['integrity']), ('waiting', ['lock_unavailable']))
        self.assertEqual(self.stored()['active'], self.active_entry())
        self.assertEqual(self.stored()['alerts'], {'lock_unavailable': stamp(NOW)})
        self.assertEqual(self.db.rows, [])
        self.db.lock_error = None
        self.now = NOW + 3600
        self.assertTrue(self.again())
        self.assertEqual((self.result()['status'], self.result()['integrity']), ('ok', ['lock_unavailable']))
        self.now = NOW + DAY
        self.assertTrue(self.again())
        self.assertEqual((self.result()['status'], self.result()['integrity']), ('current', []))

    def test_reconciling_runs_before_the_activation_lock_without_it(self):
        self.baseline()
        order = []
        catalog, lock = self.db.catalog, self.db.lock
        def read(locked=True):
            order.append(('catalog', locked, self.path.exists()))
            return catalog(locked)
        def take():
            order.append(('lock', self.path.exists()))
            return lock()
        self.db.catalog, self.db.lock = read, take
        self.assertTrue(self.run_phase('verify'))
        self.assertEqual(order[:2], [('catalog', False, False), ('lock', True)])

    def test_an_unavailable_lock_in_the_middle_of_an_operation_aborts_nothing(self):
        oid = self.db.add(self.name)
        self.open_operation('validated', oid)
        self.db.lock_error = LockUnavailable('activation lock unavailable')
        self.assertTrue(self.run_phase())
        self.assertEqual(self.result()['status'], 'waiting')
        self.assertEqual(self.stored()['operation']['step'], 'validated')
        self.assertEqual(self.status(), 'pending')


class ModeAlerts(Helpers):
    def test_a_stale_previous_waits_without_an_alert(self):
        self.baseline()
        self.write_record(active=self.active_entry())
        self.m['previous'] = self.stray()[len(self.prefix):]
        self.write_manifest()
        self.assertTrue(self.run_phase())
        self.assertEqual((self.result()['status'], self.result()['integrity']), ('waiting', []))
        self.assertEqual(self.db.rows, [])
        self.assertEqual(self.status(), 'pending')

    def test_an_aged_rollback_re_sign_waits_without_an_alert(self):
        self.baseline()
        oid = self.db.add(self.name)
        self.skip_prune()
        self.write_record(active=self.active_entry(), retired=[self.retired(self.name, oid, age=8 * DAY)])
        self.m['previous'] = self.stray()[len(self.prefix):]
        self.write_manifest()
        self.assertTrue(self.run_phase())
        self.assertEqual((self.result()['status'], self.result()['integrity']), ('waiting', []))
        self.assertEqual((self.db.rows, self.db.created), ([], 0))


class JournalCatchUp(Helpers):
    def test_a_terminal_row_for_the_operation_gets_no_catch_up_row(self):
        for terminal in ('activated', 'reverted', 'aborted', 'pruned'):
            with self.subTest(terminal=terminal):
                self.setUp()
                oid = self.db.add(self.name)
                op = self.open_operation('restored', oid)
                self.db.rows.append(dict(operation_id=op['id'], country='vn', rel=self.m['rel'], mode='new',
                                         database_name=self.name, manifest_hash=self.digest, dump_sha256=self.dump,
                                         expected_active_db=self.active_name, manifest=self.m, step=terminal))
                self.assertTrue(self.run_phase())
                self.assertEqual(self.own_rows(op['id']), [terminal, 'validated', 'checked', 'activated'])


class AbortIdentity(Helpers):
    def test_an_aborted_target_now_with_another_oid_is_not_dropped_by_the_abort_and_the_operation_closes(self):
        self.db.add(self.name, 556)
        self.open_operation('aborted', 555)
        attempts = []
        drop = self.db.drop_database
        def recorded(name, oid=None):
            attempts.append((name, oid))
            return drop(name, oid)
        self.db.drop_database = recorded
        self.assertFalse(self.run_phase())
        self.assertEqual(self.result()['status'], 'failed')
        self.assertEqual(self.status(), 'failed')
        self.assertIsNone(self.stored()['operation'])
        self.assertNotIn((self.name, 555), attempts)
        self.assertNotIn((self.name, 556), attempts)

    def test_an_abort_drops_only_the_recorded_oid(self):
        self.db.add(self.name, 555)
        self.open_operation('aborted', 555)
        def replace(name):
            if name == self.name:
                self.db.databases[name]['oid'] = 999
        self.db.before_drop = replace
        self.run_phase()
        self.assertNotIn(self.name, self.db.dropped)
        self.assertEqual(self.db.databases.get(self.name, {}).get('oid'), 999)

    def test_the_target_is_verified_before_swapping(self):
        oid = self.db.add(self.name)
        self.open_operation('validated', oid)
        self.db.smoke = lambda row, image: self.db.databases[self.name].update(oid=999)
        self.assertFalse(self.run_phase())
        self.assertEqual(self.db.swaps, [])
        self.assertEqual(self.db.pointer_value, self.active_name)
        self.assertNotEqual((self.stored()['active'] or {}).get('database'), self.name)


class RetiredDigest(Helpers):
    def refused(self, age):
        self.baseline()
        oid = self.db.add(self.name)
        self.skip_prune()
        self.write_record(active=self.active_entry(),
                          retired=[self.retired(self.name, oid, age=age, dump='f' * 64)])
        self.db.table_retired[self.name] = dict(dump_sha256='f' * 64, live=True)
        self.assertFalse(self.run_phase())
        self.assertEqual(self.result()['status'], 'failed')
        self.assertEqual(self.status(), 'failed')
        self.assertIn(self.digest, self.state.failed(self.kind.key))
        self.assertEqual((self.db.rows, self.db.swaps, self.db.created), ([], [], 0))
        self.assertEqual(self.db.pointer_value, self.active_name)
        self.assertIn(self.name, self.db.databases)

    def test_a_young_entry_with_another_digest_refuses_and_fails_the_manifest(self):
        self.refused(DAY)

    def test_an_aged_entry_with_another_digest_refuses_and_fails_the_manifest(self):
        self.refused(8 * DAY)

    def test_the_refused_manifest_stays_failed_on_the_next_run(self):
        self.refused(DAY)
        self.assertFalse(self.again())
        self.assertEqual(self.result()['message'], self.kind.label + ' manifest failed; re-sign the release to retry')


class RefusalModes(Helpers):
    def other_digest(self, adoptable):
        self.baseline()
        oid = self.db.add(self.name)
        self.skip_prune()
        self.write_record(active=self.active_entry(),
                          retired=[self.retired(self.name, oid, age=DAY, dump='f' * 64)])
        if adoptable:
            self.db.pointer_value = self.name

    def refused_in(self, mode):
        self.assertFalse(self.run_phase(mode))
        self.assertEqual((self.result()['status'], self.result()['message']),
                         ('failed', 'retained database dump digest differs'))
        self.assertEqual((self.db.rows, self.db.swaps, self.db.log, self.db.created), ([], [], [], 0))
        self.assertIn(self.name, self.db.databases)

    def test_mode_refusal_fails_the_manifest_only_in_activate_mode(self):
        for mode, status in (('verify', 'pending'), ('activate', 'failed')):
            with self.subTest(mode=mode):
                self.setUp()
                self.other_digest(adoptable=False)
                self.refused_in(mode)
                self.assertEqual(self.status(), status)
                self.assertEqual(self.digest in self.state.failed(self.kind.key), mode == 'activate')

    def test_adoption_refuses_a_retired_entry_with_another_digest(self):
        for mode, status in (('verify', 'pending'), ('activate', 'failed')):
            with self.subTest(mode=mode):
                self.setUp()
                self.other_digest(adoptable=True)
                self.refused_in(mode)
                self.assertEqual(self.status(), status)
                self.assertEqual(self.db.pointer_value, self.name)
                self.assertIsNone(self.stored()['operation'])

    def test_adoption_with_an_open_operation_refuses_a_retired_entry_with_another_digest(self):
        oid = self.db.add(self.name)
        self.open_operation('validated', oid)
        self.write_record(active=self.active_entry(), operation=self.operation('validated', oid),
                          retired=[self.retired(self.name, oid, age=DAY, dump='f' * 64)])
        self.db.pointer_value = self.name
        self.refused_in('activate')
        self.assertEqual(self.status(), 'failed')
        self.assertFalse(self.again())
        self.assertIsNone(self.stored()['operation'])


class PruneIdentity(Helpers):
    def test_prune_drops_only_the_recorded_oid(self):
        self.baseline()
        old = self.stray()
        oid = self.db.add(old)
        self.write_record(active=self.active_entry(), retired=[self.retired(old, oid, age=8 * DAY)])
        def replace(name):
            if name == old:
                self.db.databases[name]['oid'] = 999
        self.db.before_drop = replace
        self.phase.records.reconcile(self.db.catalog())
        with self.assertRaises(Waiting):
            self.phase.engine.prune(self.db.catalog())
        self.assertEqual(self.db.databases[old]['oid'], 999)
        self.assertNotIn(old, self.db.dropped)


def concrete(name, *bases):
    return type(name, bases, {})


for mixin in (SessionFaults, LockOutcomes, ModeAlerts, JournalCatchUp, AbortIdentity, RetiredDigest, RefusalModes,
              PruneIdentity):
    for label, case in (('Law', LawCase), ('Frameworks', FrameworksCase)):
        globals()[label + mixin.__name__] = concrete(label + mixin.__name__, mixin, case, unittest.TestCase)


if __name__ == '__main__':
    unittest.main()
