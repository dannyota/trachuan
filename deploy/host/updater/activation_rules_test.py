import importlib.util
import json
from pathlib import Path
import unittest
from unittest import mock

from updater.deploy_activation_record import stamp
from updater.deploy_corpus_kind import FRAMEWORKS
from updater.deploy_io import canonical
from updater.deploy_release_target import LockUnavailable
from updater.world_test import Crash, FrameworksCase, LawCase, NOW
from watch import observations, signals


spec = importlib.util.spec_from_file_location('integrity_publisher', Path(__file__).parents[1] / 'metrics/publish-metrics.py')
publisher = importlib.util.module_from_spec(spec)
spec.loader.exec_module(publisher)

DAY = 86400


def forged(case, step, name=None, operation='f' * 32, digest=None):
    return dict(operation_id=operation, country='vn', rel=case.m['rel'], mode='new', database_name=name or case.name,
                manifest_hash=digest or case.digest, dump_sha256=case.dump, expected_active_db=case.active_name,
                manifest=case.m, step=step)


class Base:
    def stray(self, rel='20990120'):
        return self.prefix + rel

    def retired(self, name, oid, age=0, dump=None):
        return dict(database=name, oid=oid, dump_sha256=dump or self.dump, retired_at=stamp(NOW - age))

    def statuses(self):
        return {record['hash']: record['status'] for record in self.records_of_kind().values()}


class ModeRules(Base):
    def test_new_operation_records_previous_pointer_as_expected(self):
        self.baseline()
        self.assertTrue(self.run_phase())
        first = self.db.rows[0]
        self.assertEqual((first['mode'], first['expected_active_db'], first['database_name']),
                         ('new', self.active_name, self.name))

    def test_retained_needs_an_entry_with_the_same_digest_and_oid(self):
        self.baseline()
        oid = self.db.add(self.name)
        self.write_record(active=self.active_entry(), retired=[self.retired(self.name, oid)])
        self.db.table_retired[self.name] = dict(dump_sha256=self.dump, live=True)
        self.m['previous'] = self.stray()[len(self.prefix):]
        self.write_manifest()
        self.assertTrue(self.run_phase())
        self.assertEqual(self.result()['mode'], 'retained')
        self.assertEqual(self.db.created, 0)
        self.assertEqual(self.db.log, ['validate', 'smoke'])
        data = self.stored()
        self.assertEqual(data['active']['oid'], oid)
        self.assertEqual([entry['database'] for entry in data['retired']], [self.active_name])
        self.assertNotIn(self.name, self.db.table_retired)
        self.assertEqual(self.db.table_retired[self.active_name]['dump_sha256'], self.dump)

    def test_an_aged_entry_with_another_oid_waits_and_spends_nothing(self):
        self.baseline()
        self.db.add(self.name, 1500)
        (self.paths.state / self.kind.marker).write_bytes(canonical('2027-01-15'))
        self.write_record(active=self.active_entry(), retired=[self.retired(self.name, 1600, age=8 * DAY)])
        self.assertTrue(self.run_phase())
        self.assertEqual(self.result()['status'], 'waiting')
        self.assertEqual(self.db.rows, [])
        self.assertEqual(self.status(), 'pending')
        self.assertIn(self.name, self.db.databases)

    def test_retained_needs_a_pointer_row(self):
        self.baseline()
        oid = self.db.add(self.name)
        self.db.pointer_value = None
        self.write_record(active=self.active_entry(), retired=[self.retired(self.name, oid)])
        self.assertTrue(self.run_phase())
        self.assertEqual((self.result()['status'], self.result()['message']),
                         ('waiting', self.kind.label + ' pointer missing'))
        self.assertEqual(self.db.rows, [])

    def test_retained_needs_the_database_to_belong_to_the_activator(self):
        self.baseline()
        oid = self.db.add(self.name, owner='postgres')
        self.write_record(active=self.active_entry(), retired=[self.retired(self.name, oid, age=DAY)])
        self.assertFalse(self.run_phase())
        self.assertEqual(self.result()['message'], 'new ' + self.kind.label + ' target already exists')
        self.assertEqual(self.stored()['retired'], [])
        self.assertEqual(self.db.rows, [])
        self.assertEqual(self.status(), 'pending')
        self.assertIn(self.name, self.db.databases)

    def aged_entry_outcome(self, age):
        self.baseline()
        oid = self.db.add(self.name)
        (self.paths.state / self.kind.marker).write_bytes(canonical('2027-01-15'))
        self.write_record(active=self.active_entry(), retired=[self.retired(self.name, oid, age=age)])
        self.db.table_retired[self.name] = dict(dump_sha256=self.dump, live=True)
        self.m['previous'] = self.stray()[len(self.prefix):]
        self.write_manifest()
        self.assertTrue(self.run_phase())
        return self.result()

    def test_an_entry_just_under_seven_days_still_qualifies_for_retained(self):
        result = self.aged_entry_outcome(7 * DAY - 1)
        self.assertEqual((result['status'], result['mode']), ('ok', 'retained'))
        self.assertEqual(self.db.created, 0)

    def test_an_entry_of_seven_days_or_more_no_longer_qualifies_for_retained(self):
        for age in (7 * DAY, 8 * DAY):
            with self.subTest(age=age):
                self.setUp()
                result = self.aged_entry_outcome(age)
                self.assertEqual(result['status'], 'waiting')
                self.assertEqual(self.db.rows, [])
                self.assertEqual(self.db.swaps, [])
                self.assertEqual(self.db.created, 0)
                self.assertEqual(self.status(), 'pending')
                self.assertIn(self.name, self.db.databases)
                self.assertEqual(len(self.stored()['retired']), 1)

    def test_new_waits_when_previous_differs_from_the_pointer(self):
        self.baseline()
        self.db.add(self.stray())
        self.db.pointer_value = self.stray()
        self.assertTrue(self.run_phase())
        self.assertEqual(self.result()['status'], 'waiting')
        self.assertIn('pointer_changed', self.result()['integrity'])
        self.assertEqual(self.db.rows, [])
        self.assertEqual(self.status(), 'pending')
        self.phase.download.assert_not_called()

    def test_new_waits_when_previous_differs_from_active_even_if_the_pointer_agrees(self):
        self.baseline()
        oid = self.db.add(self.stray())
        self.write_record(active=self.active_entry(oid, self.stray()))
        self.assertTrue(self.run_phase())
        self.assertEqual(self.result()['status'], 'waiting')
        self.assertEqual(self.db.rows, [])

    def test_a_target_that_is_the_active_database_is_never_recreated(self):
        self.baseline()
        self.db.add(self.stray())
        self.db.pointer_value = self.stray()
        self.retarget(self.m['previous'], self.stray()[len(self.prefix):])
        self.assertTrue(self.run_phase())
        self.assertEqual(self.result()['status'], 'waiting')
        self.assertEqual(self.db.created, 0)
        self.assertIn(self.active_name, self.db.databases)

    def test_without_active_only_the_pointer_must_match(self):
        self.baseline()
        self.state.data['accepted']['corpus'].clear() if self.kind is not FRAMEWORKS else self.state.section()['accepted'].clear()
        self.state.save()
        self.assertTrue(self.run_phase())
        self.assertEqual(self.result()['status'], 'ok')
        self.assertEqual(self.db.swaps[-1]['retired'], self.active_name)
        self.assertIsNone(self.db.swaps[-1]['digest'])
        self.assertEqual(self.stored()['retired'], [])

    def test_a_target_another_role_owns_is_refused_and_left_alone(self):
        self.baseline()
        self.db.add(self.name, owner='postgres')
        self.assertFalse(self.run_phase())
        self.assertEqual(self.result()['message'], 'new ' + self.kind.label + ' target already exists')
        self.assertIn(self.name, self.db.databases)
        self.assertEqual(self.db.rows, [])
        self.assertEqual(self.status(), 'pending')

    def test_a_target_with_connections_waits(self):
        self.baseline()
        self.db.add(self.name)
        self.db.foreign_protected.add(self.name)
        self.db.connected.add(self.name)
        self.assertTrue(self.run_phase())
        self.assertEqual(self.result()['status'], 'waiting')
        self.assertEqual(self.db.rows, [])

    def test_only_the_newest_accepted_manifest_starts_an_operation(self):
        self.baseline()
        self.accept()
        self.bind(self.m['sequence'] + 5, 'a' * 64, 'pending')
        self.assertTrue(self.run_phase())
        self.assertEqual(self.result()['status'], 'waiting')
        self.assertEqual(self.db.rows, [])

    def test_leftovers_are_swept_before_the_operation_starts(self):
        self.baseline()
        self.db.add(self.stray())
        self.assertTrue(self.run_phase())
        self.assertEqual(self.db.sweeps[0], 0)
        self.assertIn(self.stray(), self.db.dropped)

    def test_verify_mode_reports_without_changing_anything(self):
        self.baseline()
        self.assertTrue(self.run_phase('verify'))
        self.assertEqual(self.result()['status'], 'verified')
        self.assertEqual((self.db.rows, self.db.sweeps), ([], []))
        self.phase.download.assert_not_called()
        self.assertEqual(self.stored()['active']['database'], self.active_name)


class ForgedRows(Base):
    def test_a_forged_open_row_is_closed_and_never_resumed(self):
        self.baseline()
        self.db.rows.append(forged(self, 'checked'))
        self.assertTrue(self.run_phase())
        self.assertEqual(self.db.latest()['f' * 32]['step'], 'aborted')
        self.assertEqual(self.result()['status'], 'ok')
        self.assertEqual(self.db.created, 1)
        self.assertEqual(self.db.log[:2], ['inspect', 'inspect'])

    def test_a_forged_activated_row_finishes_nothing(self):
        self.baseline()
        self.accept()
        self.db.rows.append(forged(self, 'activated'))
        self.db.failures['restore'] = ValueError('stop here')
        self.assertFalse(self.run_phase())
        self.assertEqual(self.db.pointer_value, self.active_name)
        self.assertEqual(self.stored()['active']['database'], self.active_name)

    def test_a_forged_aborted_row_spends_nothing_and_drops_nothing(self):
        self.baseline()
        self.db.add(self.name)
        self.accept()
        self.db.rows.append(forged(self, 'aborted'))
        self.db.rows.append(forged(self, 'aborted', self.active_name, 'e' * 32))
        self.assertTrue(self.run_phase())
        self.assertEqual(self.result()['status'], 'ok')
        self.assertNotIn(self.digest, self.state.failed(self.kind.key))
        self.assertNotIn(self.active_name, self.db.dropped)

    def test_a_forged_row_cannot_redirect_the_operation(self):
        self.baseline()
        other = self.stray()
        self.db.add(other, 1999, owner='postgres')
        self.accept()
        self.db.rows.append(forged(self, 'restoring', other))
        self.assertTrue(self.run_phase())
        self.assertEqual(self.stored()['active']['database'], self.name)
        self.assertIn(other, self.db.databases)
        self.assertNotIn(other, self.db.dropped)

    def test_a_forged_row_cannot_spend_a_manifest(self):
        self.baseline()
        self.accept()
        self.db.rows.append(forged(self, 'aborted'))
        self.db.superuser = True
        self.assertTrue(self.run_phase())
        self.assertEqual(self.status(), 'pending')

    def test_a_forged_row_for_our_own_id_is_ignored(self):
        oid = self.db.add(self.name)
        op = self.open_operation('restored', oid)
        self.db.rows.append(forged(self, 'activated', self.stray(), op['id']))
        self.assertTrue(self.run_phase())
        self.assertEqual(self.db.log, ['validate', 'smoke'])
        self.assertEqual(self.stored()['active']['database'], self.name)
        self.assertEqual(self.db.pointer_value, self.name)

    def test_refused_when_the_row_returned_differs(self):
        self.baseline()
        self.db.tamper = True
        self.assertTrue(self.run_phase())
        self.assertIn('journal_row_refused', self.result()['integrity'])
        self.assertEqual(self.status(), 'pending')
        self.assertEqual(self.stored()['operation']['step'], 'started')
        self.db.tamper = False
        self.assertTrue(self.again())
        self.assertEqual(self.result()['status'], 'ok')


class Drops(Base):
    def setUp(self):
        super().setUp()
        self.baseline()
        self.m['sequence'] += 0
        self.registry.fetch.side_effect = None

    def sweep(self):
        self.phase.records.reconcile(self.db.catalog())
        self.db.clear_leftovers()

    def other_prefix(self):
        return 'corpus_vn_' if self.prefix == 'frameworks_' else 'frameworks_'

    def other_file(self, **values):
        from updater.deploy_activation_reconcile import record_path
        from updater.deploy_activation_record import Record, blank
        kind = self.phase.records.other_kind
        record = Record(record_path(self.paths.state, kind), kind.sql.database, lambda: self.now)
        record.data = dict(blank(), **values)
        record.save()

    def test_host_active_and_retired_of_both_kinds_are_kept(self):
        other_active, other_retired = self.other_prefix() + '20990110', self.other_prefix() + '20990111'
        retired = self.stray('20990112')
        for name in (other_active, other_retired, retired):
            self.db.add(name)
        self.write_record(active=self.active_entry(), retired=[self.retired(retired, self.db.databases[retired]['oid'])])
        self.other_file(active=dict(database=other_active, oid=self.db.databases[other_active]['oid'],
                                    manifest_hash='a' * 64, dump_sha256=self.dump),
                        retired=[self.retired(other_retired, self.db.databases[other_retired]['oid'])])
        self.sweep()
        self.assertEqual(self.db.dropped, [])

    def test_an_entry_older_than_seven_days_stays_protected_from_the_sweep(self):
        retired = self.stray()
        oid = self.db.add(retired)
        self.write_record(active=self.active_entry(), retired=[self.retired(retired, oid, age=9 * DAY)])
        self.sweep()
        self.assertEqual(self.db.dropped, [])

    def test_a_forged_pointer_away_from_the_active_database_never_drops_it(self):
        stray = self.stray()
        self.db.add(stray)
        self.write_record(active=self.active_entry())
        self.db.pointer_value = stray
        self.sweep()
        self.assertEqual(self.db.dropped, [])
        self.assertIn(self.active_name, self.db.databases)
        self.db.pointer_value = None
        self.sweep()
        self.assertEqual(self.db.dropped, [stray])
        self.assertIn(self.active_name, self.db.databases)

    def test_the_pointer_and_table_protection_are_kept(self):
        for name in ('pointer', 'retired', 'journal', 'other'):
            with self.subTest(name=name):
                self.setUp()
                stray = self.stray()
                self.db.add(stray)
                self.write_record(active=self.active_entry())
                if name == 'pointer':
                    self.db.pointer_value = stray
                elif name == 'retired':
                    self.db.table_retired[stray] = dict(dump_sha256=None, live=True)
                elif name == 'journal':
                    self.db.rows.append(forged(self, 'restoring', stray))
                else:
                    self.db.foreign_protected.add(stray)
                self.sweep()
                self.assertEqual(self.db.dropped, [])

    def test_an_expired_table_row_does_not_protect(self):
        stray = self.stray()
        self.db.add(stray)
        self.write_record(active=self.active_entry())
        self.db.table_retired[stray] = dict(dump_sha256=None, live=False)
        self.sweep()
        self.assertEqual(self.db.dropped, [stray])

    def test_an_open_operation_protects_its_target_except_from_itself(self):
        oid = self.db.add(self.name)
        self.accept()
        op = self.operation('restored', oid)
        self.write_record(active=self.active_entry(), operation=op)
        self.sweep()
        self.assertEqual(self.db.dropped, [])
        row = dict(operation_id=op['id'], database_name=self.name, oid=oid)
        self.db.drop_target(row)
        self.assertEqual(self.db.dropped, [self.name])

    def test_the_sweep_ignores_databases_of_other_owners_whatever_their_names(self):
        self.db.add('not_a_release', 9, 'postgres')
        self.db.add('anything_goes', 8)
        self.write_record(active=self.active_entry())
        self.sweep()
        self.assertEqual(self.db.dropped, ['anything_goes'])
        self.assertIn('not_a_release', self.db.databases)

    def test_any_failed_read_stops_every_drop(self):
        stray = self.stray()
        self.db.add(stray)
        self.write_record(active=self.active_entry())
        self.db.unavailable = True
        self.assertTrue(self.run_phase())
        self.assertEqual(self.result()['status'], 'waiting')
        self.assertEqual(self.db.dropped, [])
        self.db.unavailable = False
        self.other_file()
        self.phase.records.other.path.write_text('{')
        self.phase.records.reconcile(self.db.catalog())
        with mock.patch.object(self.db, 'catalog', side_effect=OSError('catalog unavailable')):
            with self.assertRaises(OSError):
                self.db.protected_names()
        self.assertEqual(self.db.dropped, [])


class Prune(Base):
    def setUp(self):
        super().setUp()
        self.baseline()
        self.old = self.stray()
        self.oid = self.db.add(self.old)
        self.db.table_retired[self.old] = dict(dump_sha256=self.dump, live=False)
        self.m_digest = self.digest

    def prune(self, entry=None):
        self.write_record(active=self.active_entry(), retired=[entry or self.retired(self.old, self.oid, age=8 * DAY)])
        self.bind(2, self.digest, 'active')
        self.run_prune()

    def run_prune(self):
        self.phase = self.make_phase()
        self.bind(1, self.active_hash, 'active')
        self.state.records(self.kind.key).pop('2', None)
        self.state.save()
        with self.db.lock():
            self.phase.enter('activate')
            self.phase.engine.prune(self.phase.catalog)

    def steps(self):
        return [row['step'] for row in self.db.rows if row['mode'] == 'prune']

    def test_an_aged_entry_is_dropped_journalled_forgotten_and_removed_last(self):
        self.prune()
        self.assertEqual(self.db.dropped, [self.old])
        self.assertEqual(self.steps(), ['pruning', 'pruned'])
        self.assertNotIn(self.old, self.db.table_retired)
        self.assertEqual(self.stored()['retired'], [])

    def test_a_young_entry_is_kept(self):
        entry = self.retired(self.old, self.oid, age=6 * DAY)
        self.prune(entry)
        self.assertEqual(self.db.dropped, [])
        self.assertEqual(self.stored()['retired'], [entry])

    def test_an_entry_whose_database_is_gone_is_finished(self):
        del self.db.databases[self.old]
        self.prune()
        self.assertEqual(self.db.dropped, [])
        self.assertEqual(self.steps(), ['pruning', 'pruned'])
        self.assertEqual(self.stored()['retired'], [])

    def test_an_entry_with_another_oid_or_owner_is_removed_without_a_drop(self):
        for change in (dict(oid=9999), dict(owner='postgres')):
            with self.subTest(change=change):
                self.setUp()
                self.db.databases[self.old].update(change)
                self.prune()
                self.assertEqual(self.db.dropped, [])
                self.assertIn(self.old, self.db.databases)
                self.assertEqual(self.stored()['retired'], [])
                self.assertEqual(self.steps(), [])

    def test_pointer_table_protection_and_connections_keep_the_entry(self):
        for name in ('pointer', 'table', 'connected'):
            with self.subTest(name=name):
                self.setUp()
                if name == 'pointer':
                    self.db.pointer_value = self.old
                elif name == 'table':
                    self.db.table_retired[self.old]['live'] = True
                else:
                    self.db.connected.add(self.old)
                self.prune()
                self.assertEqual(self.db.dropped, [])
                self.assertEqual(len(self.stored()['retired']), 1)

    def test_every_crash_point_repeats_harmlessly(self):
        for point in ('append:pruning', 'drop:' + self.old, 'forget:' + self.old, 'append:pruned'):
            with self.subTest(point=point):
                self.setUp()
                self.db.crashes.add(point)
                with self.assertRaises(Crash):
                    self.prune()
                self.run_prune()
                self.assertNotIn(self.old, self.db.databases)
                self.assertNotIn(self.old, self.db.table_retired)
                self.assertEqual(self.stored()['retired'], [])

    def test_a_table_row_alone_drops_nothing(self):
        self.write_record(active=self.active_entry())
        self.run_prune()
        self.assertEqual(self.db.dropped, [])
        self.assertIn(self.old, self.db.table_retired)

    def test_prune_runs_once_a_day_and_never_in_verify(self):
        self.write_record(active=self.active_entry(), retired=[self.retired(self.old, self.oid, age=8 * DAY)])
        self.assertTrue(self.run_phase('verify'))
        self.assertEqual(self.db.dropped, [])
        self.assertTrue(self.run_phase())
        self.assertEqual(self.db.dropped, [self.old])
        marker = json.loads((self.paths.state / self.kind.marker).read_bytes())
        self.assertEqual(marker, '2027-01-15')
        other = self.stray('20990121')
        oid = self.db.add(other)
        self.db.pointer_value = self.name
        data = self.stored()
        data['retired'].append(self.retired(other, oid, age=8 * DAY))
        self.write_record(**{key: data[key] for key in ('active', 'retired', 'alerts')})
        self.assertTrue(self.again())
        self.assertNotIn(other, self.db.dropped)


class FirstRun(Base):
    def test_seeds_from_real_state_and_leaves_a_postgres_owned_database_alone(self):
        self.baseline()
        by_hand = self.prefix + '20261114' + ('_3' if self.prefix != 'frameworks_' else '')
        self.db.add(by_hand, 77, 'postgres')
        self.bind(2, self.stored_manifest(by_hand, salt='by-hand'), 'retained')
        self.db.table_retired[by_hand] = dict(dump_sha256=None, live=False)
        self.assertTrue(self.run_phase())
        self.assertEqual(self.result()['status'], 'ok')
        self.assertEqual(self.result()['integrity'], [])
        data = self.stored()
        self.assertNotIn(by_hand, [entry['database'] for entry in data['retired']])
        self.assertIn(by_hand, self.db.databases)
        self.assertNotIn(by_hand, self.db.dropped)

    def test_an_operation_open_at_the_change_is_closed_and_restarted(self):
        self.baseline()
        self.accept()
        self.db.add(self.name)
        self.db.rows.append(forged(self, 'restored', operation='a' * 32))
        self.assertTrue(self.run_phase())
        self.assertEqual(self.db.latest()['a' * 32]['step'], 'aborted')
        self.assertEqual(self.result()['status'], 'ok')
        self.assertEqual(self.db.created, 1)

    def test_old_target_files_are_deleted(self):
        self.baseline()
        directory = self.paths.state / (self.kind.key + '-targets')
        directory.mkdir()
        (directory / ('c' * 32 + '.json')).write_text('{}')
        self.assertTrue(self.run_phase('verify'))
        self.assertTrue(directory.exists())
        self.assertTrue(self.run_phase())
        self.assertFalse(directory.exists())


class RollbackAcrossVersions(Base):
    def test_an_older_updater_activating_during_a_rollback_is_followed(self):
        self.baseline()
        self.assertTrue(self.run_phase())
        self.assertEqual(self.stored()['active']['database'], self.name)
        target_oid = self.stored()['active']['oid']
        rollback = self.prefix + self.m['rel'].replace('20990102', '20990103')
        rolled = self.stored_manifest(rollback, salt='older-updater')
        self.db.add(rollback, 3000)
        self.db.pointer_value = rollback
        self.bind(self.m['sequence'] + 1, rolled, 'active')
        self.bind(self.m['sequence'], self.digest, 'retained')
        self.assertTrue(self.again('verify') or True)
        data = self.stored()
        self.assertEqual(data['active'], dict(database=rollback, oid=3000, manifest_hash=rolled, dump_sha256=self.dump))
        self.assertIn(dict(database=self.name, oid=target_oid), [
            dict(database=entry['database'], oid=entry['oid']) for entry in data['retired']])
        self.assertNotIn('pointer_changed', self.result()['integrity'])

    def test_rolling_forward_continues_from_what_the_older_updater_activated(self):
        self.baseline()
        rollback_rel = self.m['rel'].replace('20990102', '20990103')
        rollback = self.prefix + rollback_rel
        rolled = self.stored_manifest(rollback, salt='older-updater')
        self.write_record(active=self.active_entry())
        self.db.add(rollback, 3000)
        self.db.pointer_value = rollback
        self.bind(1, self.active_hash, 'retained')
        self.bind(2, rolled, 'active')
        self.retarget(self.m['rel'].replace('20990102', '20990104'), rollback_rel, self.m['sequence'] + 2)
        self.assertTrue(self.run_phase())
        self.assertEqual(self.result()['status'], 'ok')
        data = self.stored()
        self.assertEqual(data['active']['database'], self.name)
        self.assertEqual(self.db.swaps[-1]['retired'], rollback)
        self.assertEqual({entry['database'] for entry in data['retired']}, {self.active_name, rollback})

    def test_journal_rows_stay_readable_by_an_older_updater(self):
        from updater.deploy_corpus_sql import STEPS
        self.baseline()
        self.assertTrue(self.run_phase())
        self.assertLessEqual({row['step'] for row in self.db.rows}, set(STEPS))
        for row in self.db.latest().values():
            self.assertEqual(self.kind.sql.journal_row(row), row)


class Alerts(Base):
    CASES = ('pointer_changed', 'serving_database_gone', 'active_cleared', 'adoption_failed', 'record_rebuilt',
             'journal_row_refused', 'lock_unavailable')

    def raise_code(self, code):
        self.baseline()
        if code == 'pointer_changed':
            self.db.add(self.stray())
            self.write_record(active=self.active_entry())
            self.db.pointer_value = self.stray()
        elif code == 'serving_database_gone':
            self.write_record(active=self.active_entry())
            self.db.pointer_value = self.stray()
        elif code == 'active_cleared':
            self.write_record(active=self.active_entry(oid=4242))
        elif code == 'adoption_failed':
            self.accept()
            self.db.add(self.name)
            self.db.failures['smoke'] = ValueError('no')
            self.db.pointer_value = self.name
            self.write_record(active=self.active_entry())
        elif code == 'record_rebuilt':
            self.path.write_text('{')
        elif code == 'lock_unavailable':
            self.write_record(active=self.active_entry())
            self.db.lock_error = LockUnavailable('activator cannot connect to auth')
        else:
            self.db.tamper = True

    def metric(self):
        path = self.paths.result.with_name(self.kind.result + '.json')
        return dict((name, value) for name, value in publisher.release(path, publisher.datetime.now(publisher.timezone.utc)))

    def watch(self, now):
        path = self.paths.result.with_name(self.kind.result + '.json')
        key = 'frameworks' if self.kind is FRAMEWORKS else 'release'
        data = dict(deploy=None, release=None, frameworks=None, secrets=None, backups={}, ready=None, corpus=None,
                    disk={}, memory=None)
        data[key] = observations.result(path, 'release')
        signals.observe(self.signals, data, now)
        return self.signals['signals'][key + '_integrity']

    def test_every_code_reaches_the_result_metric_and_watch_then_clears_after_24_hours(self):
        for code in self.CASES:
            with self.subTest(code=code):
                self.setUp()
                self.signals = signals.fresh()
                self.raise_code(code)
                self.assertIsNotNone(self.run_phase())
                self.assertIn(code, self.result()['integrity'])
                self.assertEqual(self.metric()['release_integrity'], 1)
                item = self.watch(NOW)
                self.assertTrue(item['firing'])
                self.assertIn(code, item['facts'].split(','))
                for word in (self.name, self.active_name, '/'):
                    self.assertNotIn(word, item['facts'])
                due = [message for message in signals.due(self.signals, NOW) if message['key'].endswith('_integrity')]
                self.assertEqual(due[0]['priority'], 0)
                self.db.failures.clear()
                self.db.tamper, self.db.lock_error = False, None
                active = self.stored()['active']
                if active:
                    self.db.pointer_value = active['database']
                self.now = NOW + DAY + 1
                self.assertIsNotNone(self.again())
                self.assertEqual(self.result()['integrity'], [])
                self.assertEqual(self.metric()['release_integrity'], 0)
                self.assertFalse(self.watch(NOW + DAY + 1)['firing'])

    def test_a_code_stays_while_it_keeps_being_seen(self):
        self.signals = signals.fresh()
        self.raise_code('pointer_changed')
        self.run_phase()
        self.now = NOW + DAY + 1
        self.again()
        self.assertEqual(self.result()['integrity'], ['pointer_changed'])

    def test_results_without_alerts_carry_an_empty_list(self):
        self.baseline()
        self.run_phase('verify')
        self.assertEqual(self.result()['integrity'], [])
        self.assertEqual(self.metric()['release_integrity'], 0)

    def test_an_alert_survives_when_the_next_run_stops_early(self):
        self.raise_code('pointer_changed')
        self.run_phase()
        self.db.unavailable = True
        self.assertTrue(self.again())
        self.assertEqual(self.result()['integrity'], ['pointer_changed'])


def concrete(name, *bases):
    return type(name, bases, {})


for mixin in (ModeRules, ForgedRows, Drops, Prune, FirstRun, RollbackAcrossVersions, Alerts):
    for label, case in (('Law', LawCase), ('Frameworks', FrameworksCase)):
        name = label + mixin.__name__
        globals()[name] = concrete(name, mixin, case, unittest.TestCase)


if __name__ == '__main__':
    unittest.main()
