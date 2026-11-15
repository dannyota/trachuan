from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timezone
import fcntl
import importlib.util
import json
from pathlib import Path
import sys
import tempfile
import threading
import unittest
from unittest.mock import patch

HOST = Path(__file__).resolve().parent
sys.path.insert(0, str(HOST))
import backup


class LockTest(unittest.TestCase):
    def test_health_skips_while_full_runs(self):
        self.check_pair('full', 'health', ['full'], 'skipped')

    def test_full_waits_for_health(self):
        self.check_pair('health', 'full', ['health', 'full'], 'ok')

    def test_non_health_units_run_in_order(self):
        self.check_pair('full', 'diff', ['full', 'diff'], 'ok')

    def test_restore_check_waits_for_full(self):
        self.check_pair('full', 'restore-check', ['full', 'restore-check'], 'ok')

    def check_pair(self, first, second, expected, status):
        with tempfile.TemporaryDirectory() as directory:
            state = Path(directory)
            (state / 'backup.log').touch()
            entered, release, waiting, finished = (threading.Event() for _ in range(4))
            calls = []
            def work(kind, deadline, executor):
                calls.append(kind)
                if kind == first:
                    entered.set()
                    if not release.wait(2):
                        raise AssertionError('first unit was not released')
                return 'ok', 'synthetic work completed'
            def sleep(seconds):
                waiting.set()
                if not finished.wait(2):
                    raise AssertionError('first unit did not finish')
            def run_first():
                try:
                    return backup.run(first, state, log=state/'backup.log')
                finally:
                    finished.set()
            with patch.object(backup, 'work', side_effect=work), \
                 patch.object(backup.time, 'sleep', side_effect=sleep), \
                 ThreadPoolExecutor(max_workers=2) as pool:
                one = pool.submit(run_first)
                try:
                    self.assertTrue(entered.wait(2))
                    two = pool.submit(backup.run, second, state, state/'backup.log')
                    if second == 'health':
                        self.assertEqual(two.result(timeout=2), 0)
                        self.assertFalse(waiting.is_set())
                    else:
                        self.assertTrue(waiting.wait(2))
                        row = json.loads((state / (second+'.json')).read_text())
                        self.assertEqual(row['status'], 'running')
                        self.assertEqual(row['reason'], 'waiting for backup lock')
                        self.assertFalse((state / 'active-job').exists())
                        self.assertEqual(calls, [first])
                finally:
                    release.set()
                self.assertEqual(one.result(timeout=2), 0)
                self.assertEqual(two.result(timeout=2), 0)
            self.assertEqual(calls, expected)
            self.assertEqual(json.loads((state / (second+'.json')).read_text())['status'], status)
            rows = [json.loads(line) for line in (state / 'backup.log').read_text().splitlines()]
            self.assertEqual(len(rows), 2)
            self.assertEqual({row['kind']: row['status'] for row in rows}, {first: 'ok', second: status})

    def test_wait_deadline_fails_and_publishes_job_failure(self):
        spec = importlib.util.spec_from_file_location('lock_publisher', HOST / 'metrics/publish-metrics.py')
        publisher = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(publisher)
        for kind, limit in [('full', 14400), ('diff', 7200), ('verify', 14400), ('restore-check', 1200)]:
            with self.subTest(kind=kind), tempfile.TemporaryDirectory() as directory:
                state = Path(directory)
                path = state / (kind+'.json')
                path.write_text('{"last_success_at":"2026-01-01T00:00:00Z"}')
                clock = [0.0]
                def sleep(seconds):
                    self.assertGreater(seconds, 0)
                    self.assertLessEqual(seconds, 1)
                    clock[0] = limit
                with (state / 'backup.lock').open('a') as lock:
                    fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                    with patch.object(backup.time, 'monotonic', side_effect=lambda: clock[0]), \
                         patch.object(backup.time, 'sleep', side_effect=sleep), \
                         patch.object(backup, 'work') as work:
                        self.assertEqual(backup.run(kind, state, log=state/'backup.log'), 1)
                    work.assert_not_called()
                row = json.loads(path.read_text())
                self.assertEqual(row['status'], 'failed')
                self.assertEqual(row['reason'], 'backup lock not acquired before the deadline')
                self.assertIsNotNone(row['finished_at'])
                self.assertEqual(publisher.backup(path, datetime(2026, 1, 2, tzinfo=timezone.utc)), [
                    ('backup_failed', 1), ('backup_last_success_age_seconds', 86400)])

    def test_interrupted_wait_finishes_without_starting_work_or_leaving_a_marker(self):
        with tempfile.TemporaryDirectory() as directory:
            state = Path(directory)
            with (state / 'backup.lock').open('a') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                with patch.object(backup.time, 'sleep', side_effect=InterruptedError('synthetic signal')), \
                     patch.object(backup, 'work') as work:
                    self.assertEqual(backup.run('full', state, log=state/'backup.log'), 1)
                work.assert_not_called()
            row = json.loads((state / 'full.json').read_text())
            self.assertEqual(row['status'], 'failed')
            self.assertIsNotNone(row['finished_at'])
            self.assertFalse((state / 'active-job').exists())


if __name__ == '__main__':
    unittest.main()
