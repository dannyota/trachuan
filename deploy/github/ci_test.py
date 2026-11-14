import importlib
from pathlib import Path
import unittest


class CITest(unittest.TestCase):
    def setUp(self):
        self.assertTrue(Path(__file__).with_name('wait_ci.py').exists(), 'CI waiter missing')
        self.ci = importlib.import_module('wait_ci')
        self.now = 0
        self.run = dict(id=4, head_sha='a' * 40, head_branch='main', event='push',
                        status='completed', conclusion='success')

    def sleep(self, seconds):
        self.now += seconds

    def wait(self, fetch):
        return self.ci.wait('a' * 40, fetch, lambda: self.now, self.sleep, timeout=30)

    def test_waits_for_exact_main_push_commit(self):
        responses = iter([[self.run | {'head_sha': 'b' * 40},
                           self.run | {'head_branch': 'other'}, self.run | {'event': 'pull_request'}],
                          [self.run | {'status': 'in_progress', 'conclusion': None}], [self.run]])
        self.wait(lambda commit: next(responses))
        self.assertEqual(self.now, 30)

    def test_latest_run_controls_gate(self):
        with self.assertRaisesRegex(ValueError, 'failure'):
            self.wait(lambda commit: [self.run, self.run | {'id': 5, 'conclusion': 'failure'}])

    def test_rejects_every_non_success_conclusion(self):
        for conclusion in ('failure', 'cancelled', 'neutral', 'skipped', 'timed_out', 'action_required', None):
            with self.subTest(conclusion=conclusion), self.assertRaises(ValueError):
                self.wait(lambda commit: [self.run | {'conclusion': conclusion}])

    def test_times_out_without_matching_ci(self):
        with self.assertRaises(TimeoutError):
            self.wait(lambda commit: [])
        self.assertEqual(self.now, 30)

    def test_success_after_deadline_is_rejected(self):
        def fetch(commit):
            self.now = 31
            return [self.run]

        with self.assertRaises(TimeoutError):
            self.wait(fetch)


if __name__ == '__main__':
    unittest.main()
