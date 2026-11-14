import importlib.util
import json
from pathlib import Path
import unittest

HOST = Path(__file__).resolve().parent
NOW = 1800000000


def load():
    path = HOST / 'backup_status.py'
    if not path.exists():
        raise AssertionError('backup status classifier is missing')
    spec = importlib.util.spec_from_file_location('backup_status', path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def info(age=60, error=False):
    return json.dumps([{'name': 'trachuan', 'status': {'code': 0, 'message': 'ok'},
                        'backup': [{'label': '20270115-020000F', 'error': error,
                                    'timestamp': {'start': NOW-age-10, 'stop': NOW-age}}]}])


class ClassificationTest(unittest.TestCase):
    def test_health_never_accepts_bad_or_missing_backup(self):
        module = load()
        for text, expected in [(info(), 'ok'), (info(error=True), 'failed'),
                               (info(age=26*3600+1), 'failed'), ('[]', 'unknown'),
                               ('broken', 'unknown'), (info().replace(str(NOW-60), '"bad"'), 'unknown')]:
            with self.subTest(text=text):
                self.assertEqual(module.backup_health(text, NOW)[0], expected)

    def test_newest_good_excludes_errored_backup(self):
        module = load()
        data = json.loads(info())
        data[0]['backup'].append({'label': '20270116-020000F', 'error': True,
                                  'timestamp': {'start': NOW-10, 'stop': NOW}})
        self.assertEqual(module.newest_good(json.dumps(data), NOW)['label'], '20270115-020000F')
        self.assertEqual(module.backup_health(json.dumps(data), NOW)[0], 'failed')

    def test_invalid_future_or_missing_timestamp_is_unknown(self):
        module = load()
        for stamp in [None, {}, {'stop': NOW+1}, {'stop': True}, {'stop': -1}]:
            data = json.loads(info())
            data[0]['backup'][0]['timestamp'] = stamp
            self.assertEqual(module.backup_health(json.dumps(data), NOW)[0], 'unknown')

    def test_archiver_and_wal_boundaries(self):
        module = load()
        for row, expected in [({'failed_count': 0, 'last_failed_time': None, 'last_archived_time': None, 'wal_bytes': 10_000_000_000}, 'ok'),
                              ({'failed_count': 1, 'last_failed_time': NOW-1801, 'last_archived_time': NOW-1900, 'wal_bytes': 0}, 'failed'),
                              ({'failed_count': 1, 'last_failed_time': NOW-10, 'last_archived_time': NOW-1900, 'wal_bytes': 0}, 'failed'),
                              ({'failed_count': 1, 'last_failed_time': NOW-10, 'last_archived_time': NOW-100, 'wal_bytes': 0}, 'ok'),
                              ({'failed_count': 1, 'last_failed_time': NOW-20, 'last_archived_time': NOW-10, 'wal_bytes': 0}, 'ok'),
                              ({'failed_count': 0, 'last_failed_time': None, 'last_archived_time': None, 'wal_bytes': 10_000_000_001}, 'failed'),
                              ({'failed_count': 1, 'last_failed_time': 'invalid', 'wal_bytes': 0}, 'unknown')]:
            with self.subTest(row=row):
                self.assertEqual(module.archiver_health(json.dumps(row), NOW)[0], expected)

    def test_verify_260_verbose_results(self):
        module = load()
        good = ('stanza: trachuan\nstatus: ok\n'
                '  archiveId: 11-2, total WAL checked: 2, total valid WAL: 2\n'
                '    missing: 0, checksum invalid: 0, size invalid: 0, other: 0\n'
                '  backup: 20181119-153300F, status: valid, total files checked: 1, total valid files: 1\n'
                '    missing: 0, checksum invalid: 0, size invalid: 0, other: 0\n')
        self.assertEqual(module.verify_status(good)[0], 'ok')
        for output, expected in [(good.replace('status: ok', 'status: error'), 'failed'),
                                 (good.replace('checksum invalid: 0', 'checksum invalid: 1'), 'failed'),
                                 (good.replace('status: valid', 'status: invalid'), 'failed'),
                                 ('status: ok', 'unknown'), (good.replace('stanza: trachuan', 'stanza: other'), 'unknown')]:
            with self.subTest(output=output):
                self.assertEqual(module.verify_status(output)[0], expected)


if __name__ == '__main__':
    unittest.main()
