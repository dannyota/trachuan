import unittest
from unittest import mock

from updater.deploy_activation_record import BEFORE_SWAPPING, stamp
from updater.deploy_io import CommandError
from updater.deploy_release_tables import TableFailure
from updater.world_test import Crash, FrameworksCase, LawCase, NOW


NEW_STEPS = dict(started=['inspect', 'inspect', 'create', 'restore', 'validate', 'smoke'],
                 creating=['inspect', 'create', 'restore', 'validate', 'smoke'],
                 created=['restore', 'validate', 'smoke'], restoring=['restore', 'validate', 'smoke'],
                 restored=['validate', 'smoke'], validated=['smoke'], checked=['validate', 'smoke'])
RETAINED_STEPS = dict(started=['validate', 'smoke'], restored=['validate', 'smoke'], validated=['smoke'],
                      checked=['validate', 'smoke'])
NEEDS_OID = ('created', 'restoring', 'restored', 'validated', 'checked')
ELSEWHERE = 'somewhere'


class RecoveryChecks:
    @property
    def elsewhere(self):
        return self.prefix + '20990120'

    def finished(self, active_oid=None):
        data = self.stored()
        self.assertEqual(self.result()['status'], 'ok')
        self.assertIsNone(data['operation'])
        self.assertEqual(data['active']['database'], self.name)
        self.assertEqual(data['active']['manifest_hash'], self.digest)
        self.assertEqual(self.status(), 'active')
        self.assertEqual(self.db.pointer_value, self.name)
        self.assertEqual(self.db.rows[-1]['step'], 'activated')
        self.assertIn(self.active_name, [entry['database'] for entry in data['retired']])
        self.assertEqual(self.db.swaps[-1]['retired'], self.active_name)
        self.assertEqual(self.db.swaps[-1]['digest'], self.dump)
        if active_oid:
            self.assertEqual(data['active']['oid'], active_oid)

    def waiting(self, message=None, *alerts):
        result = self.result()
        self.assertEqual(result['status'], 'waiting')
        if message:
            self.assertEqual(result['message'], message)
        self.assertEqual(self.status(), 'pending')
        self.assertLessEqual(set(alerts), set(result['integrity']))
        self.assertNotIn(self.digest, self.state.failed(self.kind.key))


class RecoveryRows(RecoveryChecks):
    def test_before_swapping_with_target_as_recorded_resumes_from_the_step(self):
        for step in BEFORE_SWAPPING:
            with self.subTest(step=step):
                self.setUp()
                oid = self.db.add(self.name) if step in NEEDS_OID else None
                self.open_operation(step, oid)
                self.assertTrue(self.run_phase())
                self.assertEqual(self.db.log, NEW_STEPS[step])
                self.finished()
                self.registry.fetch.assert_not_called()

    def test_retained_operation_resumes_without_a_restore(self):
        for step, expected in RETAINED_STEPS.items():
            with self.subTest(step=step):
                self.setUp()
                oid = self.db.add(self.name)
                self.db.table_retired[self.name] = dict(dump_sha256=self.dump, live=True)
                self.open_operation(step, oid, mode='retained')
                self.assertTrue(self.run_phase())
                self.assertEqual(self.db.log, expected)
                self.finished(oid)
                self.assertNotIn(self.name, self.db.table_retired)
                self.phase.download.assert_not_called()

    def test_before_swapping_with_target_gone_or_replaced_starts_a_new_operation(self):
        for present in (False, True):
            with self.subTest(present=present):
                self.setUp()
                self.open_operation('restored', 555)
                if present:
                    self.db.add(self.name, 556)
                self.assertTrue(self.run_phase())
                self.finished()
                ids = {row['operation_id'] for row in self.db.rows}
                self.assertNotIn('c' * 32, ids)
                self.assertEqual(self.db.created, 1)
                self.registry.fetch.assert_called_once()

    def test_before_swapping_with_pointer_at_the_target_adopts(self):
        for step in BEFORE_SWAPPING:
            with self.subTest(step=step):
                self.setUp()
                oid = self.db.add(self.name)
                self.open_operation(step, oid)
                self.db.pointer_value = self.name
                self.assertTrue(self.run_phase())
                self.assertEqual(self.db.log, ['validate', 'smoke'])
                self.assertEqual(self.db.created, 0)
                self.finished(oid)
                self.assertIn('pointer_changed', self.result()['integrity'])

    def test_adopting_records_the_oid_when_none_was_recorded(self):
        oid = self.db.add(self.name)
        self.open_operation('started')
        self.db.pointer_value = self.name
        self.assertTrue(self.run_phase())
        self.finished(oid)

    def test_swapping_with_target_as_recorded_and_pointer_at_target_finishes(self):
        oid = self.db.add(self.name)
        self.open_operation('swapping', oid)
        self.db.pointer_value = self.name
        self.assertTrue(self.run_phase())
        self.assertEqual(self.db.log, [])
        self.finished(oid)
        self.assertEqual(self.result()['integrity'], [])

    def test_swapping_with_pointer_at_expected_runs_the_swap_again(self):
        oid = self.db.add(self.name)
        self.open_operation('swapping', oid)
        self.assertTrue(self.run_phase())
        self.assertEqual(len(self.db.swaps), 1)
        self.finished(oid)

    def test_swapping_with_target_gone_and_pointer_at_expected_starts_a_new_operation(self):
        for present in (False, True):
            with self.subTest(present=present):
                self.setUp()
                self.open_operation('swapping', 555)
                if present:
                    self.db.add(self.name, 556)
                self.assertTrue(self.run_phase())
                self.finished()
                self.assertEqual(self.db.created, 1)

    def test_swapping_with_target_gone_and_pointer_at_target_waits_and_alarms(self):
        for present in (False, True):
            with self.subTest(present=present):
                self.setUp()
                self.open_operation('swapping', 555)
                self.db.pointer_value = self.name
                if present:
                    self.db.add(self.name, 556)
                self.assertTrue(self.run_phase())
                self.waiting('serving database gone', 'serving_database_gone')
                self.assertEqual(self.stored()['operation']['step'], 'swapping')
                self.assertEqual(self.db.swaps, [])

    def test_swapping_with_pointer_anywhere_else_waits_with_pointer_changed(self):
        for present in (True, False):
            with self.subTest(present=present):
                self.setUp()
                oid = self.db.add(self.name) if present else 555
                self.open_operation('swapping', oid)
                self.db.add(self.elsewhere)
                self.db.pointer_value = self.elsewhere
                self.assertTrue(self.run_phase())
                self.waiting('pointer changed', 'pointer_changed')
                self.assertEqual(self.db.swaps, [])
                self.assertEqual(self.db.pointer_value, self.elsewhere)

    def test_aborted_operation_drops_its_target_fails_the_manifest_and_closes(self):
        oid = self.db.add(self.name)
        self.open_operation('aborted', oid)
        self.assertFalse(self.run_phase())
        self.assertEqual(self.result()['status'], 'failed')
        self.assertEqual(self.status(), 'failed')
        self.assertIn(self.digest, self.state.failed(self.kind.key))
        self.assertIsNone(self.stored()['operation'])
        self.assertEqual(self.db.dropped, [self.name])
        self.assertEqual(self.db.rows[-1]['step'], 'aborted')

    def test_aborted_retained_operation_keeps_its_database(self):
        oid = self.db.add(self.name)
        self.baseline()
        self.accept()
        self.write_record(active=self.active_entry(),
                          retired=[dict(database=self.name, oid=oid, dump_sha256=self.dump, retired_at=stamp(NOW))],
                          operation=self.operation('aborted', oid, mode='retained'))
        self.assertFalse(self.run_phase())
        self.assertEqual(self.db.dropped, [])
        self.assertIn(self.name, self.db.databases)
        self.assertEqual(self.status(), 'failed')

    def test_aborted_operation_never_drops_what_the_tables_protect(self):
        oid = self.db.add(self.name)
        self.open_operation('aborted', oid)
        self.db.table_retired[self.name] = dict(dump_sha256=None, live=True)
        self.assertFalse(self.run_phase())
        self.assertEqual(self.db.dropped, [])
        self.assertEqual(self.status(), 'failed')

    def test_activated_operation_settles_and_closes_without_touching_tables(self):
        oid = self.db.add(self.name)
        self.baseline()
        self.accept()
        self.write_record(active=self.active_entry(oid, self.name, self.digest),
                          retired=[dict(database=self.active_name, oid=100, dump_sha256=self.dump,
                                        retired_at=stamp(NOW))],
                          operation=self.operation('activated', oid))
        self.db.pointer_value = self.name
        self.assertTrue(self.run_phase())
        self.assertEqual(self.result()['status'], 'ok')
        self.assertEqual(self.status(), 'active')
        self.assertIsNone(self.stored()['operation'])
        self.assertEqual(self.db.swaps, [])
        self.assertEqual(self.stored()['active']['database'], self.name)

    def test_no_operation_and_pointer_elsewhere_starts_a_new_operation(self):
        self.baseline()
        self.assertTrue(self.run_phase())
        self.finished()
        self.assertEqual(self.db.created, 1)
        self.assertEqual(self.db.log, NEW_STEPS['started'])

    def test_no_operation_and_pointer_at_the_target_adopts(self):
        self.baseline()
        oid = self.db.add(self.name)
        self.db.pointer_value = self.name
        self.assertTrue(self.run_phase())
        self.assertEqual(self.db.log, ['validate', 'smoke'])
        self.assertEqual(self.db.created, 0)
        self.finished(oid)
        self.phase.download.assert_not_called()

    def test_verify_mode_starts_resumes_adopts_and_finishes_nothing(self):
        for step, pointer_at_target in (('restored', False), ('swapping', True), ('activated', True), ('aborted', False)):
            with self.subTest(step=step):
                self.setUp()
                oid = self.db.add(self.name)
                self.open_operation(step, oid)
                if pointer_at_target:
                    self.db.pointer_value = self.name
                before = self.stored()['operation']
                self.assertTrue(self.run_phase('verify'))
                self.assertEqual(self.result()['status'], 'verified')
                self.assertEqual(self.stored()['operation'], before)
                self.assertEqual((self.db.log, self.db.swaps, self.db.dropped), ([], [], []))
                self.assertEqual(self.status(), 'pending')
                self.assertEqual(self.db.rows, [])


class Adoption(RecoveryChecks):
    def fails(self):
        self.assertTrue(self.run_phase())
        self.waiting('adoption check failed', 'adoption_failed')
        self.assertEqual(self.db.swaps, [])

    def test_a_missing_target_cannot_be_adopted(self):
        self.baseline()
        self.db.pointer_value = self.name
        self.fails()
        self.assertIn('serving_database_gone', self.result()['integrity'])

    def test_a_target_another_role_owns_cannot_be_adopted(self):
        self.baseline()
        self.db.add(self.name, owner='postgres')
        self.db.pointer_value = self.name
        self.fails()

    def test_a_recorded_oid_must_match(self):
        self.db.add(self.name, 556)
        self.open_operation('checked', 555)
        self.db.pointer_value = self.name
        self.fails()

    def test_each_read_only_check_must_pass(self):
        for step, error in (('validate', ValueError('restored release row differs from manifest')),
                            ('smoke', CommandError('command failed', detail='podman run: exit 1'))):
            with self.subTest(step=step):
                self.setUp()
                self.baseline()
                self.db.add(self.name)
                self.db.pointer_value = self.name
                self.db.failures[step] = error
                self.fails()
                self.assertIsNone(self.stored()['operation'])

    def test_adoption_uses_retained_style_checks_and_writes_nothing_first(self):
        self.baseline()
        oid = self.db.add(self.name)
        self.db.pointer_value = self.name
        seen = []
        original = self.db.validate
        def validate(row):
            seen.append((row['mode'], row['oid'], len(self.db.rows)))
            original(row)
        self.db.validate = validate
        self.assertTrue(self.run_phase())
        self.assertEqual(seen, [('retained', oid, 0)])

    def test_re_signed_active_release_retires_nothing(self):
        self.baseline()
        self.retarget(self.m['previous'])
        self.assertTrue(self.run_phase())
        self.assertEqual(self.result()['status'], 'ok')
        self.assertEqual(self.db.swaps[-1]['retired'], None)
        data = self.stored()
        self.assertEqual(data['active'], self.active_entry(digest=self.digest))
        self.assertEqual(data['retired'], [])
        self.assertEqual(self.status(), 'active')

    def test_failed_adoption_recovers_on_the_next_run(self):
        self.baseline()
        self.db.add(self.name)
        self.db.pointer_value = self.name
        self.db.failures['smoke'] = ValueError('not yet')
        self.assertTrue(self.run_phase())
        del self.db.failures['smoke']
        self.assertTrue(self.again())
        self.finished()


class CrashPoints(RecoveryChecks):
    def finishing_crash(self, point=None, settle=False, clear=False):
        oid = self.db.add(self.name)
        self.open_operation('checked', oid)
        if point:
            self.db.crashes.add(point)
        patches = []
        if settle:
            patches.append(mock.patch.object(self.state, 'settle', side_effect=Crash('settle')))
        if clear:
            patches.append(mock.patch('updater.deploy_activation_record.Record.clear_operation',
                                      side_effect=Crash('clear')))
        for patch in patches:
            patch.start()
        try:
            with self.assertRaises(Crash):
                self.run_phase()
        finally:
            for patch in patches:
                patch.stop()
        return oid

    def test_crash_after_the_record_says_swapping_runs_the_swap_again(self):
        oid = self.finishing_crash('swap:before')
        self.assertEqual(self.stored()['operation']['step'], 'swapping')
        self.assertEqual(self.db.pointer_value, self.active_name)
        self.db.log.clear()
        self.assertTrue(self.again())
        self.assertEqual(self.db.log, [])
        self.finished(oid)

    def test_crash_after_the_transaction_finishes_from_the_pointer(self):
        oid = self.finishing_crash('swap:after')
        self.assertEqual(self.stored()['operation']['step'], 'swapping')
        self.assertEqual(self.db.pointer_value, self.name)
        self.assertTrue(self.again())
        self.assertEqual(len(self.db.swaps), 2)
        self.assertEqual(self.db.swaps[-1]['retired'], self.active_name)
        self.finished(oid)

    def test_crash_after_the_record_says_activated_settles_only(self):
        oid = self.finishing_crash(settle=True)
        self.assertEqual(self.stored()['operation']['step'], 'activated')
        self.assertEqual(self.status(), 'pending')
        swaps = len(self.db.swaps)
        self.assertTrue(self.again())
        self.assertEqual(len(self.db.swaps), swaps)
        self.finished(oid)

    def test_crash_after_state_was_settled_closes_the_operation(self):
        oid = self.finishing_crash(clear=True)
        self.assertEqual(self.status(), 'active')
        self.assertIsNotNone(self.stored()['operation'])
        self.assertTrue(self.again())
        self.assertIsNone(self.stored()['operation'])
        self.assertEqual(self.stored()['active']['oid'], oid)

    def aborting_crash(self, point=None, fail=False, clear=False):
        oid = self.db.add(self.name)
        self.open_operation('validated', oid)
        self.db.failures['smoke'] = ValueError('smoke failed')
        if point:
            self.db.crashes.add(point)
        patches = []
        if fail:
            patches.append(mock.patch.object(self.state, 'fail', side_effect=Crash('fail')))
        if clear:
            patches.append(mock.patch('updater.deploy_activation_record.Record.clear_operation',
                                      side_effect=Crash('clear')))
        for patch in patches:
            patch.start()
        try:
            with self.assertRaises(Crash):
                self.run_phase()
        finally:
            for patch in patches:
                patch.stop()

    def assert_abort_completes(self):
        del self.db.failures['smoke']
        self.assertFalse(self.again())
        self.assertEqual(self.result()['status'], 'failed')
        self.assertEqual(self.status(), 'failed')
        self.assertIsNone(self.stored()['operation'])
        self.assertNotIn(self.name, self.db.databases)
        self.assertEqual(self.db.pointer_value, self.active_name)
        self.assertEqual(self.db.latest()['c' * 32]['step'], 'aborted')

    def test_crash_after_the_record_says_aborted(self):
        self.aborting_crash('append:aborted')
        self.assertEqual(self.stored()['operation']['step'], 'aborted')
        self.assert_abort_completes()

    def test_crash_after_the_aborted_row(self):
        self.aborting_crash('drop:' + self.name)
        self.assertEqual(self.db.latest()['c' * 32]['step'], 'aborted')
        self.assertIn(self.name, self.db.databases)
        self.assert_abort_completes()

    def test_crash_after_the_target_was_dropped(self):
        self.aborting_crash(fail=True)
        self.assertNotIn(self.name, self.db.databases)
        self.assertEqual(self.status(), 'pending')
        self.assert_abort_completes()

    def test_crash_after_the_manifest_was_failed(self):
        self.aborting_crash(clear=True)
        self.assertEqual(self.status(), 'failed')
        self.assertIsNotNone(self.stored()['operation'])
        self.assertTrue(self.again() is not None)
        self.assertIsNone(self.stored()['operation'])

    def test_crash_between_every_step_of_a_new_activation_resumes_it(self):
        for step in ('started', 'creating', 'created', 'restoring', 'restored', 'validated', 'checked'):
            with self.subTest(step=step):
                self.setUp()
                self.baseline()
                self.db.crashes.add('append:' + step)
                with self.assertRaises(Crash):
                    self.run_phase()
                self.assertEqual(self.stored()['operation']['step'], step)
                self.assertTrue(self.again())
                self.finished()
                self.assertEqual(self.db.created, 1)
                self.assertEqual([row['step'] for row in self.db.rows].count(step), 1)

    def test_journal_gets_every_row_the_record_is_ahead_of(self):
        oid = self.db.add(self.name)
        self.open_operation('validated', oid)
        self.assertTrue(self.run_phase())
        steps = [row['step'] for row in self.db.rows if row['operation_id'] == 'c' * 32]
        self.assertEqual(steps, ['started', 'creating', 'created', 'restoring', 'restored', 'validated', 'checked',
                                 'activated'])

    def test_interruption_never_aborts(self):
        oid = self.db.add(self.name)
        self.open_operation('validated', oid)
        try:
            raise InterruptedError('updater interrupted')
        except InterruptedError as cause:
            error = CommandError('command failed')
            error.__cause__ = cause
        self.db.failures['smoke'] = error
        self.assertFalse(self.run_phase())
        self.assertTrue(self.phase.interrupted)
        self.assertEqual(self.stored()['operation']['step'], 'validated')
        self.assertEqual(self.status(), 'pending')


class TableFailures(RecoveryChecks):
    def test_unreadable_tables_wait_without_spending_the_manifest(self):
        self.baseline()
        self.db.unavailable = True
        self.assertTrue(self.run_phase())
        self.assertEqual(self.result()['status'], 'waiting')
        self.assertEqual(self.db.rows, [])
        self.phase.download.assert_not_called()

    def test_a_failure_while_writing_a_step_keeps_the_operation(self):
        oid = self.db.add(self.name)
        self.open_operation('restored', oid)
        original = self.db.append
        def append(row, step):
            if step == 'validated':
                raise TableFailure('auth tables unavailable')
            return original(row, step)
        self.db.append = append
        self.assertTrue(self.run_phase())
        self.waiting('auth tables unavailable')
        self.assertEqual(self.stored()['operation']['step'], 'validated')
        del self.db.append
        self.assertTrue(self.again())
        self.finished(oid)

    def test_a_failed_swap_waits_and_the_next_run_finishes(self):
        oid = self.db.add(self.name)
        self.open_operation('checked', oid)
        self.db.swap_error = TableFailure('auth tables unavailable')
        self.assertTrue(self.run_phase())
        self.waiting('auth tables unavailable')
        self.assertEqual(self.stored()['operation']['step'], 'swapping')
        self.db.swap_error = None
        self.assertTrue(self.again())
        self.finished(oid)

    def test_pointer_moved_before_the_swap_waits_instead_of_aborting(self):
        oid = self.db.add(self.name)
        self.open_operation('checked', oid)
        self.db.add(self.elsewhere)
        self.db.pointer_value = self.elsewhere
        self.assertTrue(self.run_phase())
        self.waiting('pointer changed', 'pointer_changed')
        self.assertEqual(self.stored()['operation']['step'], 'checked')
        self.db.pointer_value = self.active_name
        self.assertTrue(self.again())
        self.finished(oid)

    def test_a_refused_journal_row_waits_and_raises_its_alert(self):
        oid = self.db.add(self.name)
        self.open_operation('restored', oid)
        self.db.tamper = True
        self.assertTrue(self.run_phase())
        self.waiting('invalid release journal row', 'journal_row_refused')
        self.assertEqual(self.stored()['operation']['oid'], oid)
        self.assertEqual(self.db.dropped, [])

    def test_a_malformed_open_row_waits_and_raises_its_alert(self):
        self.baseline()
        self.accept()
        self.db.rows.append(dict(operation_id='f' * 32, country='vn', rel=self.m['rel'], mode='new',
                                 database_name='host=127.0.0.1 dbname=x', manifest_hash=self.digest,
                                 dump_sha256=self.dump, expected_active_db=None, manifest=self.m, step='started'))
        self.assertTrue(self.run_phase())
        self.waiting('invalid release journal row', 'journal_row_refused')
        self.assertEqual(self.db.created, 0)

    def test_an_unusable_pointer_waits_and_alarms(self):
        self.baseline()
        self.db.pointer_bad = True
        self.assertTrue(self.run_phase())
        self.assertEqual(self.result()['status'], 'waiting')
        self.assertIn('pointer_changed', self.result()['integrity'])
        self.assertEqual(self.db.rows, [])


class LeavingAWait(RecoveryChecks):
    def waiting_operation(self):
        oid = self.db.add(self.name)
        self.open_operation('swapping', oid)
        self.db.add(self.elsewhere)
        self.db.pointer_value = self.elsewhere
        self.assertTrue(self.run_phase())
        self.waiting('pointer changed')
        return oid

    def test_setting_the_pointer_back_resumes_the_operation(self):
        oid = self.waiting_operation()
        self.db.pointer_value = self.active_name
        self.assertTrue(self.again())
        self.finished(oid)

    def test_the_phase_still_fetches_while_waiting(self):
        self.waiting_operation()
        self.registry.fetch.assert_called_once()

    def test_a_fetch_failure_keeps_the_waiting_result(self):
        oid = self.db.add(self.name)
        self.open_operation('swapping', oid)
        self.db.add(self.elsewhere)
        self.db.pointer_value = self.elsewhere
        self.registry.fetch.side_effect = CommandError('registry down')
        self.assertTrue(self.run_phase())
        self.waiting('pointer changed')

    def test_a_newer_manifest_replaces_the_operation_and_the_older_stays_pending(self):
        self.waiting_operation()
        older = self.digest
        self.retarget(self.m['rel'].replace('20990102', '20990103'), self.elsewhere[len(self.prefix):],
                      self.m['sequence'] + 1)
        self.assertTrue(self.again())
        self.assertEqual(self.result()['manifest_hash'], self.digest)
        self.assertEqual(self.result()['status'], 'waiting')
        self.assertIsNone(self.stored()['operation'])
        records = {record['hash']: record['status'] for record in self.records_of_kind().values()}
        self.assertEqual(records[older], 'pending')
        self.assertEqual(records[self.digest], 'pending')

    def test_the_older_manifest_fails_when_the_newer_one_becomes_active(self):
        self.waiting_operation()
        older = self.digest
        self.retarget(self.m['rel'].replace('20990102', '20990103'), self.m['previous'], self.m['sequence'] + 1)
        self.assertTrue(self.again())
        self.assertEqual(self.result()['status'], 'waiting')
        self.assertIsNone(self.stored()['operation'])
        self.db.pointer_value = self.active_name
        self.assertTrue(self.again())
        self.assertEqual(self.result()['status'], 'ok')
        records = {record['hash']: record['status'] for record in self.records_of_kind().values()}
        self.assertEqual(records[older], 'failed')
        self.assertEqual(records[self.digest], 'active')


class LawRecoveryRows(RecoveryRows, LawCase, unittest.TestCase):
    pass


class FrameworksRecoveryRows(RecoveryRows, FrameworksCase, unittest.TestCase):
    pass


class LawAdoption(Adoption, LawCase, unittest.TestCase):
    pass


class FrameworksAdoption(Adoption, FrameworksCase, unittest.TestCase):
    pass


class LawCrashPoints(CrashPoints, LawCase, unittest.TestCase):
    pass


class FrameworksCrashPoints(CrashPoints, FrameworksCase, unittest.TestCase):
    pass


class LawTableFailures(TableFailures, LawCase, unittest.TestCase):
    pass


class FrameworksTableFailures(TableFailures, FrameworksCase, unittest.TestCase):
    pass


class LawLeavingAWait(LeavingAWait, LawCase, unittest.TestCase):
    pass


class FrameworksLeavingAWait(LeavingAWait, FrameworksCase, unittest.TestCase):
    pass


if __name__ == '__main__':
    unittest.main()
