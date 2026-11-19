import json
import os
from pathlib import Path
import stat
import tempfile
import unittest

from updater import deploy_corpus_sql as law
from updater import deploy_frameworks_sql as frameworks
from updater.deploy_activation_record import ALERTS, Record, STEPS, Unreadable, parse, stamp


NOW = 1800000000
HASH, DUMP, OPERATION = 'a' * 64, 'b' * 64, 'c' * 32
LAW = 'corpus_vn_20990102_1'
OLD = 'corpus_vn_20990101_1'


def sample():
    return dict(version=1,
                active=dict(database=LAW, oid=41001, manifest_hash=HASH, dump_sha256=DUMP),
                retired=[dict(database=OLD, oid=39000, dump_sha256=DUMP, retired_at='2026-11-18T01:00:00Z')],
                operation=dict(id=OPERATION, mode='new', database='corpus_vn_20990103_1', manifest_hash=HASH,
                               dump_sha256=DUMP, expected=LAW, oid=None, step='started'),
                alerts={'pointer_changed': stamp(NOW)})


class RecordFixture(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.path = Path(directory.name) / 'corpus-activation.json'
        self.now = NOW
        self.record = Record(self.path, law.database, lambda: self.now)


class RecordFileTest(RecordFixture):
    def test_save_is_canonical_root_only_and_atomic(self):
        self.record.data = sample()
        self.record.save()
        raw = self.path.read_bytes()
        self.assertEqual(raw, json.dumps(sample(), sort_keys=True, separators=(',', ':')).encode() + b'\n')
        self.assertEqual(stat.S_IMODE(self.path.stat().st_mode), 0o600)
        self.assertEqual(os.listdir(self.path.parent), [self.path.name])

    def test_load_reports_missing_ok_and_unreadable(self):
        self.assertEqual(self.record.load(), 'missing')
        self.record.data = sample()
        self.record.save()
        fresh = Record(self.path, law.database, lambda: self.now)
        self.assertEqual(fresh.load(), 'ok')
        self.assertEqual(fresh.data, sample())
        self.path.write_text('{')
        self.assertEqual(fresh.load(), 'unreadable')

    def test_saving_identical_content_does_not_rewrite(self):
        self.record.data = sample()
        self.record.save()
        before = self.path.stat().st_mtime_ns
        os.utime(self.path, ns=(1, 1))
        self.record.save()
        self.assertNotEqual(self.path.stat().st_mtime_ns, before)
        self.assertEqual(self.path.stat().st_mtime_ns, 1)


class ValidationTest(unittest.TestCase):
    def refused(self, value, check=law.database):
        with self.assertRaises(Unreadable):
            parse(json.dumps(value).encode(), check)

    def test_valid_record_and_null_sections_pass(self):
        self.assertEqual(parse(json.dumps(sample()).encode(), law.database), sample())
        empty = dict(sample(), active=None, operation=None, retired=[], alerts={})
        self.assertEqual(parse(json.dumps(empty).encode(), law.database), empty)

    def test_every_step_and_both_modes_are_accepted(self):
        for step in STEPS:
            for mode in ('new', 'retained'):
                value = sample()
                value['operation'].update(step=step, mode=mode, oid=41002)
                self.assertEqual(parse(json.dumps(value).encode(), law.database), value)

    def test_top_level_shape_is_exact(self):
        self.refused([])
        self.refused(dict(sample(), extra=1))
        for key in sample():
            self.refused({k: v for k, v in sample().items() if k != key})
        for version in (2, 0, '1', True, 1.0, None):
            self.refused(dict(sample(), version=version))

    def test_duplicate_keys_are_unreadable(self):
        raw = json.dumps(sample()).replace('"version": 1', '"version": 1, "version": 1').encode()
        with self.assertRaises(Unreadable):
            parse(raw, law.database)

    def section_cases(self):
        names = ('corpus_vn_20990102_0', 'frameworks_20990102', 'auth', 'postgres', '', None, 7,
                 'host=127.0.0.1 dbname=x', 'corpus_vn_20990102\n')
        digests = ('a' * 63, 'A' * 64, '', None, 7)
        oids = (0, -1, '41001', 1.5, True, None)
        times = ('2026-11-18', '2026-11-18T01:00:00', '2026-11-18T01:00:00+00:00', '2026-13-18T01:00:00Z', None, 7)
        return names, digests, oids, times

    def test_active_fields(self):
        names, digests, oids, _ = self.section_cases()
        self.refused(dict(sample(), active=[]))
        self.refused(dict(sample(), active=dict(sample()['active'], extra=1)))
        for field, values in (('database', names), ('manifest_hash', digests), ('dump_sha256', digests), ('oid', oids)):
            for value in values:
                with self.subTest(field=field, value=value):
                    self.refused(dict(sample(), active=dict(sample()['active'], **{field: value})))

    def test_retired_fields(self):
        names, digests, oids, times = self.section_cases()
        entry = sample()['retired'][0]
        self.refused(dict(sample(), retired=None))
        self.refused(dict(sample(), retired=[dict(entry, extra=1)]))
        self.refused(dict(sample(), retired=[entry, entry]))
        for field, values in (('database', names), ('dump_sha256', digests), ('oid', oids), ('retired_at', times)):
            for value in values:
                with self.subTest(field=field, value=value):
                    self.refused(dict(sample(), retired=[dict(entry, **{field: value})]))

    def test_operation_fields(self):
        names, digests, oids, _ = self.section_cases()
        op = sample()['operation']
        self.refused(dict(sample(), operation=dict(op, extra=1)))
        cases = (('id', ('c' * 31, 'C' * 32, '', None)), ('mode', ('prune', 'New', '', None)),
                 ('database', names), ('manifest_hash', digests), ('dump_sha256', digests),
                 ('expected', [name for name in names if name is not None]),
                 ('oid', [value for value in oids if value is not None]),
                 ('step', ('pruning', 'reverted', 'Started', '', None)))
        for field, values in cases:
            for value in values:
                with self.subTest(field=field, value=value):
                    self.refused(dict(sample(), operation=dict(op, **{field: value})))

    def test_expected_and_oid_may_be_null(self):
        value = sample()
        value['operation'].update(expected=None, oid=None)
        self.assertEqual(parse(json.dumps(value).encode(), law.database), value)

    def test_alert_codes_and_times_are_exact(self):
        for alerts in ([], {'other': '2026-11-18T01:05:00Z'}, {'pointer_changed': 'yesterday'}, {'pointer_changed': None}):
            with self.subTest(alerts=alerts):
                self.refused(dict(sample(), alerts=alerts))
        for code in ALERTS:
            value = dict(sample(), alerts={code: '2026-11-18T01:05:00Z'})
            self.assertEqual(parse(json.dumps(value).encode(), law.database), value)

    def test_names_are_checked_for_the_kind_of_the_file(self):
        value = dict(sample(), retired=[], operation=None,
                     active=dict(sample()['active'], database='frameworks_20990102'))
        self.assertEqual(parse(json.dumps(value).encode(), frameworks.database), value)
        self.refused(value, law.database)
        self.refused(sample(), frameworks.database)


class AlertTest(RecordFixture):
    def setUp(self):
        super().setUp()
        self.record.data = dict(sample(), operation=None, alerts={})

    def test_an_alert_stores_the_time_it_was_last_seen(self):
        self.record.alert('pointer_changed')
        self.assertEqual(self.record.data['alerts'], {'pointer_changed': stamp(NOW)})
        self.now += 60
        self.record.alert('pointer_changed')
        self.assertEqual(self.record.data['alerts'], {'pointer_changed': stamp(NOW + 60)})

    def test_unknown_alert_codes_are_a_program_error(self):
        with self.assertRaises(ValueError):
            self.record.alert('database_on_fire')

    def test_integrity_lists_codes_for_24_hours_after_last_seen(self):
        self.record.alert('record_rebuilt')
        self.now += 3600
        self.record.alert('adoption_failed')
        self.assertEqual(self.record.integrity(), ['adoption_failed', 'record_rebuilt'])
        self.now = NOW + 86399
        self.assertEqual(self.record.integrity(), ['adoption_failed', 'record_rebuilt'])
        self.now = NOW + 86400
        self.assertEqual(self.record.integrity(), ['adoption_failed'])
        self.now = NOW + 3600 + 86400
        self.assertEqual(self.record.integrity(), [])

    def test_save_forgets_codes_older_than_24_hours(self):
        self.record.alert('record_rebuilt')
        self.now += 86400
        self.record.save()
        self.assertEqual(json.loads(self.path.read_bytes())['alerts'], {})

    def test_integrity_reads_the_file_before_the_record_is_loaded(self):
        self.record.alert('journal_row_refused')
        self.record.save()
        fresh = Record(self.path, law.database, lambda: self.now)
        self.assertEqual(fresh.integrity(), ['journal_row_refused'])
        self.path.write_text('{')
        self.assertEqual(Record(self.path, law.database, lambda: self.now).integrity(), [])
        self.path.unlink()
        self.assertEqual(Record(self.path, law.database, lambda: self.now).integrity(), [])


class OperationEditTest(RecordFixture):
    def setUp(self):
        super().setUp()
        self.record.data = dict(sample(), operation=None, alerts={})

    def op(self, **changes):
        return dict(sample()['operation'], **changes)

    def on_disk(self):
        return json.loads(self.path.read_bytes())

    def test_begin_writes_the_operation_before_anything_else(self):
        self.record.begin(self.op())
        self.assertEqual(self.on_disk()['operation'], self.op())

    def test_begin_refuses_a_second_operation(self):
        self.record.begin(self.op())
        with self.assertRaises(ValueError):
            self.record.begin(self.op(id='d' * 32))

    def test_step_and_oid_are_written_at_once(self):
        self.record.begin(self.op())
        self.record.step('creating')
        self.assertEqual(self.on_disk()['operation']['step'], 'creating')
        self.record.set_oid(41002)
        self.assertEqual(self.on_disk()['operation']['oid'], 41002)
        with self.assertRaises(ValueError):
            self.record.step('pruning')

    def test_activate_moves_active_to_retired_in_one_write(self):
        self.record.begin(self.op(oid=41002, step='swapping'))
        self.record.activate()
        value = self.on_disk()
        self.assertEqual(value['active'], dict(database='corpus_vn_20990103_1', oid=41002, manifest_hash=HASH,
                                               dump_sha256=DUMP))
        self.assertEqual(value['operation']['step'], 'activated')
        self.assertEqual(value['retired'][-1], dict(database=LAW, oid=41001, dump_sha256=DUMP, retired_at=stamp(NOW)))

    def test_activate_on_a_re_signed_active_release_retires_nothing(self):
        self.record.begin(self.op(database=LAW, oid=41001, step='swapping', manifest_hash='d' * 64))
        self.record.activate()
        value = self.on_disk()
        self.assertEqual(value['active'], dict(database=LAW, oid=41001, manifest_hash='d' * 64, dump_sha256=DUMP))
        self.assertEqual([entry['database'] for entry in value['retired']], [OLD])

    def test_activate_removes_a_retained_target_from_retired(self):
        self.record.begin(self.op(database=OLD, mode='retained', oid=39000, step='swapping'))
        self.record.activate()
        self.assertEqual([entry['database'] for entry in self.on_disk()['retired']], [LAW])

    def test_activate_without_active_records_no_retired_entry(self):
        self.record.data['active'] = None
        self.record.begin(self.op(oid=41002, step='swapping'))
        self.record.activate()
        self.assertEqual([entry['database'] for entry in self.on_disk()['retired']], [OLD])

    def test_clear_operation_keeps_everything_else(self):
        self.record.begin(self.op())
        self.record.clear_operation()
        self.assertIsNone(self.on_disk()['operation'])
        self.assertEqual(self.on_disk()['active']['database'], LAW)


if __name__ == '__main__':
    unittest.main()
