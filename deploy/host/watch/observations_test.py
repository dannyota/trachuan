import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from . import observations, signals, storage
from .signals_test import NOW


class ObservationTest(unittest.TestCase):
    def setUp(self):
        self.work = tempfile.TemporaryDirectory()
        self.addCleanup(self.work.cleanup)
        self.root = Path(self.work.name)
        (self.root / 'deploy').mkdir()
        (self.root / 'backup').mkdir()

    def test_results_validate_status_timestamps_identity_and_bounded_files(self):
        path = self.root / 'result.json'
        self.assertIsNone(observations.result(path, 'deploy'))
        for value in ({'status': 'bogus'}, {'status': 'failed'}, {'status': 'ok', 'started_at': 'bad'},
                      {'status': 'ok', 'last_success_at': '2026-01-01T01:00:00+01:00'},
                      {'status': 'waiting'}, {'status': 'current', 'sequence': True}, [], None):
            path.write_text(json.dumps(value))
            with self.subTest(value=value), self.assertRaises(ValueError):
                observations.result(path, 'release' if isinstance(value, dict) and value.get('status') == 'waiting' else 'deploy')
        path.write_text('{"status":"failed","started_at":"2026-11-17T01:00:00Z"}')
        self.assertEqual(observations.result(path, 'deploy')['started_at'], NOW)
        path.write_bytes(b'x'*65537)
        with self.assertRaises(ValueError):
            observations.result(path, 'deploy')
        path.unlink()
        path.symlink_to(self.root / 'deploy')
        with self.assertRaises(OSError):
            observations.result(path, 'deploy')

    def test_readiness_skips_active_or_activating_but_reads_public_status(self):
        for deploy_state in ('active', 'activating', 'inactive', 'failed'):
            calls = []
            def get(path, timeout):
                calls.append((path, timeout))
                return (503, {}) if path == '/readyz' else (200, {'release': '20990101'})
            value = observations.collect(self.root, lambda: deploy_state, get,
                                         lambda path: 71, lambda: 9)
            self.assertEqual(value['ready'], None if deploy_state in ('active', 'activating') else 503)
            self.assertEqual(value['corpus'], '20990101')
            self.assertIn(('/api/v1/public/status', 5), calls)
            self.assertEqual(value['disk'], {'/': 71, '/srv/trachuan/data': 71})
            self.assertEqual(value['memory'], 9)
            self.assertEqual(len(calls), 1 if deploy_state in ('active', 'activating') else 2)

    def test_failed_public_status_is_indeterminate_and_malformed_result_fails(self):
        for response in ((500, {'release': '20990101'}), (200, {}), (200, {'release': None})):
            value = observations.collect(self.root, lambda: 'active', lambda *args: response,
                                         lambda path: 20, lambda: 50)
            self.assertIsNone(value['corpus'])
        def broken(*args):
            raise OSError('synthetic error')
        value = observations.collect(self.root, lambda: 'inactive', broken, lambda path: 20, lambda: 50)
        self.assertEqual(value['ready'], 0)
        self.assertIsNone(value['corpus'])
        (self.root / 'deploy/secrets.json').write_text('{')
        with self.assertRaises(ValueError):
            observations.collect(self.root, lambda: 'active', broken, lambda path: 20, lambda: 50)

    def test_memory_and_filesystem_use_available_capacity(self):
        memory = self.root / 'meminfo'
        memory.write_text('MemTotal:       1000 kB\nMemFree: 5 kB\nMemAvailable: 90 kB\n')
        self.assertEqual(observations.memory(memory), 9)
        memory.write_text('MemTotal: 0 kB\nMemAvailable: 3 kB\n')
        with self.assertRaises(ValueError):
            observations.memory(memory)
        class Stat:
            f_blocks, f_bfree, f_bavail = 100, 20, 10
        with patch.object(observations.os, 'statvfs', return_value=Stat()):
            self.assertAlmostEqual(observations.filesystem('/'), 80/90*100)

    def test_state_missing_signal_fields_is_invalid_and_starts_fresh(self):
        state = signals.fresh()
        from .signals_test import sample
        signals.observe(state, sample() | {'ready': 503}, NOW)
        del state['signals']['site']['since']
        path = self.root / 'state.json'
        storage.save(path, state)
        self.assertEqual(storage.load_state(path), signals.fresh())

    def test_invalid_state_starts_fresh_and_atomic_modes_do_not_leak(self):
        path = self.root / 'state.json'
        self.assertEqual(storage.load_state(path), signals.fresh())
        for raw in ('{', '[]', '{}', '{"version":1,"signals":null}',
                    '{"version":1,"signals":{},"events":{},"sends":[true],"retry_at":0}'):
            path.write_text(raw)
            self.assertEqual(storage.load_state(path), signals.fresh())
        state = signals.fresh()
        storage.save(path, state)
        self.assertEqual(storage.load_state(path), state)
        self.assertEqual(path.stat().st_mode & 0o777, 0o600)
        storage.result(self.root, True, NOW)
        self.assertEqual((self.root/'result.json').stat().st_mode & 0o777, 0o640)
        self.assertEqual(sorted(p.name for p in self.root.iterdir()), ['backup', 'deploy', 'result.json', 'state.json'])


if __name__ == '__main__':
    unittest.main()
