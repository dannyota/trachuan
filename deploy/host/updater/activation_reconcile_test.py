import hashlib
import json
from pathlib import Path
import tempfile
import unittest

from updater.deploy_activation_reconcile import OWNER, Records, evaluate
from updater.deploy_activation_record import stamp
from updater.deploy_corpus_kind import FRAMEWORKS, LAW
from updater.deploy_io import canonical
from updater.deploy_state import State


NOW = 1800000000
DAY = 86400
DUMP, OTHER_DUMP = 'b' * 64, 'd' * 64
ACTIVE, OLD, HAND, NEXT = 'corpus_vn_20261118_1', 'corpus_vn_20261116_1', 'corpus_vn_20261114_3', 'corpus_vn_20261120_1'


def owned(oid):
    return dict(oid=oid, owner=OWNER)


class Fixture(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.root = Path(directory.name)
        self.now = NOW
        self.notes = []
        self.state = State.bootstrap(self.root / 'state.json', 'f' * 64, 1, 7, 'e' * 64)
        self.catalog = {}
        self.records = self.make(LAW)

    def make(self, kind):
        return Records(self.root, kind, self.state, lambda: self.now, self.notes.append)

    def manifest(self, name, dump=DUMP, kind=LAW, salt=''):
        raw = canonical(dict(database=name, dump=dict(sha256=dump), salt=salt))
        digest = hashlib.sha256(raw).hexdigest()
        directory = self.root / kind.artifacts / digest
        directory.mkdir(parents=True)
        (directory / 'manifest.json').write_bytes(raw)
        return digest

    def accepted(self, *items, key='corpus'):
        section = self.state.data['accepted']['corpus'] if key == 'corpus' else self.state.section()['accepted']
        for sequence, (digest, status) in enumerate(items, 1):
            section[str(sequence)] = dict(hash=digest, status=status)

    def reconcile(self):
        self.records.reconcile(self.catalog)
        return self.records.own.data

    def seed(self, data):
        record = self.records.own
        record.data = dict(dict(version=1, active=None, retired=[], operation=None, alerts={}), **data)
        record.save()

    def active_entry(self, digest, name=ACTIVE, oid=100):
        return dict(database=name, oid=oid, manifest_hash=digest, dump_sha256=DUMP)

    def retired_entry(self, name=OLD, oid=90, age=0):
        return dict(database=name, oid=oid, dump_sha256=DUMP, retired_at=stamp(NOW - age))

    def operation(self, digest, step='started', name=NEXT, oid=None, expected=ACTIVE):
        return dict(id='c' * 32, mode='new', database=name, manifest_hash=digest, dump_sha256=DUMP,
                    expected=expected, oid=oid, step=step)


class SeedTest(Fixture):
    def test_first_run_seeds_from_a_real_state_shape_without_any_alert(self):
        active = self.manifest(ACTIVE)
        retained = self.manifest(OLD)
        by_hand = self.manifest(HAND)
        self.accepted((by_hand, 'retained'), (retained, 'retained'), ('9' * 64, 'failed'), (active, 'active'))
        self.catalog = {ACTIVE: owned(100), OLD: owned(90), HAND: dict(oid=80, owner='postgres')}
        data = self.reconcile()
        self.assertEqual(data['active'], self.active_entry(active))
        self.assertEqual(data['retired'], [self.retired_entry()])
        self.assertIsNone(data['operation'])
        self.assertEqual(data['alerts'], {})
        self.assertEqual(self.notes, [])
        self.assertEqual(json.loads(self.records.own.path.read_bytes()), data)

    def test_a_retained_database_another_role_owns_gets_no_entry(self):
        by_hand = self.manifest(HAND)
        self.accepted((by_hand, 'retained'))
        self.catalog = {HAND: dict(oid=80, owner='postgres')}
        self.assertEqual(self.reconcile()['retired'], [])

    def test_manifests_whose_database_is_gone_or_artifact_is_missing_get_no_entry(self):
        gone = self.manifest(OLD)
        self.accepted(('7' * 64, 'active'), (gone, 'retained'))
        data = self.reconcile()
        self.assertIsNone(data['active'])
        self.assertEqual(data['retired'], [])

    def test_a_tampered_artifact_gives_no_entry(self):
        digest = self.manifest(ACTIVE)
        (self.root / LAW.artifacts / digest / 'manifest.json').write_bytes(canonical(dict(database=OLD)))
        self.accepted((digest, 'active'))
        self.catalog = {ACTIVE: owned(100), OLD: owned(90)}
        self.assertIsNone(self.reconcile()['active'])

    def test_newest_retained_manifest_wins_for_one_database(self):
        first = self.manifest(OLD, OTHER_DUMP, salt='1')
        second = self.manifest(OLD, DUMP, salt='2')
        self.accepted((first, 'retained'), (second, 'retained'))
        self.catalog = {OLD: owned(90)}
        self.assertEqual(self.reconcile()['retired'], [self.retired_entry()])

    def test_active_and_retained_of_one_database_keep_only_active(self):
        first = self.manifest(ACTIVE, salt='1')
        second = self.manifest(ACTIVE, salt='2')
        self.accepted((first, 'retained'), (second, 'active'))
        self.catalog = {ACTIVE: owned(100)}
        data = self.reconcile()
        self.assertEqual(data['active']['manifest_hash'], second)
        self.assertEqual(data['retired'], [])

    def test_unreadable_file_is_rebuilt_with_a_warning_and_an_alert(self):
        active = self.manifest(ACTIVE)
        self.accepted((active, 'active'))
        self.catalog = {ACTIVE: owned(100)}
        self.records.own.path.write_text('{not json')
        data = self.reconcile()
        self.assertEqual(data['active'], self.active_entry(active))
        self.assertEqual(data['alerts'], {'record_rebuilt': stamp(NOW)})
        self.assertEqual(len(self.notes), 1)
        self.assertEqual(self.reconcile()['alerts'], {'record_rebuilt': stamp(NOW)})
        self.assertEqual(len(self.notes), 1)

    def test_frameworks_seeds_from_its_own_state_section(self):
        name = 'frameworks_20990102'
        digest = self.manifest(name, kind=FRAMEWORKS)
        self.accepted((digest, 'active'), key='frameworks')
        self.catalog = {name: owned(70)}
        self.records = self.make(FRAMEWORKS)
        data = self.reconcile()
        self.assertEqual(data['active']['database'], name)
        self.assertTrue(str(self.records.own.path).endswith('frameworks-activation.json'))

    def test_frameworks_without_a_state_section_seeds_empty(self):
        self.records = self.make(FRAMEWORKS)
        self.assertEqual(self.reconcile()['active'], None)
        self.assertNotIn('frameworks', self.state.data)


class ActiveMovedTest(Fixture):
    def test_rule_2_moves_the_old_active_and_rebuilds_it(self):
        old = self.manifest(ACTIVE)
        rollback = self.manifest(OLD)
        self.accepted((rollback, 'active'), (old, 'retained'))
        self.seed(dict(active=self.active_entry(old)))
        self.catalog = {ACTIVE: owned(100), OLD: owned(90)}
        data = self.reconcile()
        self.assertEqual(data['active'], dict(self.active_entry(rollback, OLD, 90)))
        self.assertEqual(data['retired'], [dict(self.retired_entry(ACTIVE, 100))])

    def test_rule_2_keeps_no_entry_when_the_old_oid_changed(self):
        old = self.manifest(ACTIVE)
        rollback = self.manifest(OLD)
        self.accepted((rollback, 'active'), (old, 'retained'))
        self.seed(dict(active=self.active_entry(old)))
        self.catalog = {ACTIVE: owned(101), OLD: owned(90)}
        self.assertEqual(self.reconcile()['retired'], [])

    def test_rule_2_does_not_apply_while_the_operation_is_at_activated(self):
        previous = self.manifest(ACTIVE)
        target = self.manifest(NEXT)
        self.accepted((previous, 'active'), (target, 'pending'))
        self.seed(dict(active=self.active_entry(target, NEXT, 110),
                       operation=self.operation(target, 'activated', oid=110)))
        self.catalog = {ACTIVE: owned(100), NEXT: owned(110)}
        data = self.reconcile()
        self.assertEqual(data['active'], self.active_entry(target, NEXT, 110))
        self.assertEqual(data['retired'], [])
        self.assertEqual(data['operation']['step'], 'activated')

    def test_rule_2_is_silent_when_the_state_names_no_active_manifest(self):
        old = self.manifest(ACTIVE)
        self.accepted((old, 'retained'))
        self.seed(dict(active=self.active_entry(old)))
        self.catalog = {ACTIVE: owned(100)}
        self.assertEqual(self.reconcile()['active'], self.active_entry(old))

    def test_a_cleared_active_is_not_rebuilt(self):
        digest = self.manifest(ACTIVE)
        self.accepted((digest, 'active'))
        self.seed(dict(active=None))
        self.catalog = {ACTIVE: owned(100)}
        self.assertIsNone(self.reconcile()['active'])


class OperationClosedTest(Fixture):
    def test_rule_3_closes_an_operation_whose_manifest_is_not_pending(self):
        for status in ('active', 'retained', 'failed', None):
            with self.subTest(status=status):
                self.setUp()
                digest = self.manifest(NEXT)
                if status:
                    self.accepted((digest, status))
                self.seed(dict(operation=self.operation(digest)))
                self.assertIsNone(self.reconcile()['operation'])

    def test_rule_3_keeps_a_pending_operation(self):
        digest = self.manifest(NEXT)
        self.accepted((digest, 'pending'))
        self.seed(dict(operation=self.operation(digest)))
        self.assertEqual(self.reconcile()['operation'], self.operation(digest))


class RetiredEntriesTest(Fixture):
    def setUp(self):
        super().setUp()
        self.digest = self.manifest(ACTIVE)
        self.accepted((self.digest, 'active'))
        self.catalog = {ACTIVE: owned(100), OLD: owned(90)}

    def retired_after(self, entry):
        self.seed(dict(active=self.active_entry(self.digest), retired=[entry]))
        return self.reconcile()['retired']

    def test_rule_4_removes_the_entry_of_the_active_database(self):
        self.assertEqual(self.retired_after(self.retired_entry(ACTIVE, 100)), [])

    def test_rule_4_keeps_a_young_entry_that_still_matches(self):
        entry = self.retired_entry(age=DAY)
        self.assertEqual(self.retired_after(entry), [entry])

    def test_rule_4_removes_a_young_entry_whose_database_changed(self):
        for catalog in ({}, {OLD: owned(91)}, {OLD: dict(oid=90, owner='postgres')}):
            with self.subTest(catalog=catalog):
                self.catalog = {ACTIVE: owned(100), **catalog}
                self.assertEqual(self.retired_after(self.retired_entry(age=6 * DAY)), [])

    def test_rule_4_leaves_older_entries_for_prune(self):
        entry = self.retired_entry(age=7 * DAY)
        for catalog in ({}, {OLD: owned(91)}, {OLD: dict(oid=90, owner='postgres')}):
            with self.subTest(catalog=catalog):
                self.catalog = {ACTIVE: owned(100), **catalog}
                self.assertEqual(self.retired_after(entry), [entry])


class ActiveClearedTest(Fixture):
    def setUp(self):
        super().setUp()
        self.digest = self.manifest(ACTIVE)
        self.accepted((self.digest, 'active'))
        self.seed(dict(active=self.active_entry(self.digest)))

    def test_rule_5_clears_a_missing_or_replaced_active_with_both_alerts(self):
        for catalog in ({}, {ACTIVE: owned(101)}):
            with self.subTest(catalog=catalog):
                self.seed(dict(active=self.active_entry(self.digest)))
                self.catalog = catalog
                data = self.reconcile()
                self.assertIsNone(data['active'])
                self.assertEqual(set(data['alerts']), {'active_cleared', 'serving_database_gone'})

    def test_rule_5_clears_an_active_another_role_owns_without_saying_it_is_gone(self):
        self.catalog = {ACTIVE: dict(oid=100, owner='postgres')}
        data = self.reconcile()
        self.assertIsNone(data['active'])
        self.assertEqual(set(data['alerts']), {'active_cleared'})

    def test_active_stays_null_after_the_database_returns(self):
        self.catalog = {}
        self.reconcile()
        self.catalog = {ACTIVE: owned(100)}
        data = self.reconcile()
        self.assertIsNone(data['active'])
        self.assertEqual(set(data['alerts']), {'active_cleared', 'serving_database_gone'})

    def test_matching_active_stays_and_raises_nothing(self):
        self.catalog = {ACTIVE: owned(100)}
        data = self.reconcile()
        self.assertEqual(data['active'], self.active_entry(self.digest))
        self.assertEqual(data['alerts'], {})


class PointerAlertTest(Fixture):
    def setUp(self):
        super().setUp()
        self.digest = self.manifest(ACTIVE)
        self.accepted((self.digest, 'active'))
        self.catalog = {ACTIVE: owned(100), NEXT: owned(110)}
        self.seed(dict(active=self.active_entry(self.digest)))
        self.reconcile()

    def alerts(self, pointer):
        self.records.own.data['alerts'] = {}
        evaluate(self.records.own, pointer, self.catalog)
        return set(self.records.own.data['alerts'])

    def test_pointer_at_active_raises_nothing(self):
        self.assertEqual(self.alerts(ACTIVE), set())

    def test_any_other_pointer_raises_pointer_changed(self):
        self.assertEqual(self.alerts(NEXT), {'pointer_changed'})
        self.assertEqual(self.alerts(None), {'pointer_changed'})

    def test_a_swapping_operation_explains_its_own_pointer_values(self):
        self.records.own.data['operation'] = self.operation('9' * 64, 'swapping', oid=110)
        self.assertEqual(self.alerts(ACTIVE), set())
        self.assertEqual(self.alerts(NEXT), set())
        self.assertEqual(self.alerts(OLD), {'pointer_changed', 'serving_database_gone'})

    def test_an_operation_before_swapping_explains_nothing(self):
        self.records.own.data['operation'] = self.operation('9' * 64, 'checked', oid=110)
        self.assertEqual(self.alerts(NEXT), {'pointer_changed'})

    def test_no_active_means_no_pointer_comparison(self):
        self.records.own.data['active'] = None
        self.assertEqual(self.alerts(NEXT), set())

    def test_the_pointer_database_must_exist_with_its_known_oid(self):
        self.assertEqual(self.alerts(OLD), {'pointer_changed', 'serving_database_gone'})
        self.catalog = {ACTIVE: owned(101)}
        self.assertEqual(self.alerts(ACTIVE), {'serving_database_gone'})
        self.catalog = {ACTIVE: owned(100)}
        self.records.own.data['operation'] = self.operation('9' * 64, 'swapping', oid=110)
        self.assertEqual(self.alerts(NEXT), {'serving_database_gone'})

    def test_a_retired_entry_gives_the_known_oid_too(self):
        self.catalog = {ACTIVE: owned(100), OLD: owned(91)}
        self.records.own.data['retired'] = [self.retired_entry()]
        self.assertIn('serving_database_gone', self.alerts(OLD))
        self.catalog[OLD] = owned(90)
        self.assertNotIn('serving_database_gone', self.alerts(OLD))


class ProtectedNamesTest(Fixture):
    def setUp(self):
        super().setUp()
        self.digest = self.manifest(ACTIVE)
        self.accepted((self.digest, 'active'))
        self.catalog = {ACTIVE: owned(100), OLD: owned(90), NEXT: owned(110)}
        self.seed(dict(active=self.active_entry(self.digest), retired=[self.retired_entry()],
                       operation=self.operation('9' * 64, oid=110)))
        self.accepted((self.digest, 'active'), ('9' * 64, 'pending'))
        self.reconcile()

    def frameworks_file(self, data):
        other = Records(self.root, FRAMEWORKS, self.state, lambda: self.now, self.notes.append)
        other.own.data = dict(dict(version=1, active=None, retired=[], operation=None, alerts={}), **data)
        other.own.save()

    def test_active_retired_and_open_target_of_both_kinds_are_protected(self):
        fw = 'frameworks_20990102'
        self.frameworks_file(dict(active=dict(database=fw, oid=70, manifest_hash='8' * 64, dump_sha256=DUMP),
                                  retired=[dict(database='frameworks_20990101', oid=60, dump_sha256=DUMP,
                                                retired_at=stamp(NOW))],
                                  operation=dict(id='d' * 32, mode='new', database='frameworks_20990103',
                                                 manifest_hash='7' * 64, dump_sha256=DUMP, expected=fw, oid=None,
                                                 step='started')))
        self.assertEqual(self.records.names(self.catalog),
                         {ACTIVE, OLD, NEXT, fw, 'frameworks_20990101', 'frameworks_20990103'})

    def test_a_missing_other_file_counts_as_empty(self):
        self.assertEqual(self.records.names(self.catalog), {ACTIVE, OLD, NEXT})
        self.assertFalse(self.records.other.path.exists())

    def test_an_unreadable_other_file_is_rebuilt_before_it_protects(self):
        fw = 'frameworks_20990102'
        digest = self.manifest(fw, kind=FRAMEWORKS)
        self.accepted((digest, 'active'), key='frameworks')
        self.catalog[fw] = owned(70)
        self.records.other.path.write_text('{')
        self.assertIn(fw, self.records.names(self.catalog))
        data = json.loads(self.records.other.path.read_bytes())
        self.assertEqual(data['active']['database'], fw)
        self.assertEqual(set(data['alerts']), {'record_rebuilt'})

    def test_an_operation_is_exempt_only_for_its_own_target(self):
        self.assertNotIn(NEXT, self.records.names(self.catalog, own='c' * 32))
        self.assertIn(NEXT, self.records.names(self.catalog, own='d' * 32))

    def test_pruning_exempts_only_the_named_entries(self):
        names = self.records.names(self.catalog, pruning={OLD})
        self.assertNotIn(OLD, names)
        self.assertIn(ACTIVE, names)


if __name__ == '__main__':
    unittest.main()
