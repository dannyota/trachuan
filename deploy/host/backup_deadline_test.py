import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

HOST = Path(__file__).resolve().parent
sys.path.insert(0, str(HOST))
import backup
import backup_process as process


class RecoveryDeadlineTest(unittest.TestCase):
    def check_recovery(self, outcome):
        with tempfile.TemporaryDirectory() as work:
            state = Path(work)
            token = 'a'*32
            (state/'active-job').write_text(token)
            result = state/'health.json'
            result.write_text(json.dumps({'last_success_at': '2026-01-01T00:00:00Z'}))
            clock = [0.0]
            calls = []
            def runner(args, timeout, **kwargs):
                self.assertEqual(args[4:], ['cancel', token])
                self.assertEqual(json.loads(result.read_text())['status'], 'running')
                self.assertLess(clock[0], 300, 'cancellation continued past the unit deadline')
                self.assertGreater(timeout, 0)
                self.assertLessEqual(timeout, 300-clock[0])
                calls.append(timeout)
                clock[0] += min(timeout, 2)
                if outcome == 'cannot_execute':
                    raise OSError('synthetic missing helper')
                if outcome == 'confirmed' and len(calls) == 3:
                    return 'stopped\n'
                return 'unconfirmed\n'
            def sleep(seconds):
                clock[0] += seconds
            with patch.object(process.time, 'monotonic', side_effect=lambda: clock[0]), \
                 patch.object(process.time, 'sleep', side_effect=sleep), \
                 patch.object(process, 'command', side_effect=runner), \
                 patch.object(backup, 'work', return_value=('ok', 'synthetic')) as task:
                code = backup.run('health', state)
            final = json.loads(result.read_text())
            self.assertLessEqual(clock[0], 300)
            self.assertIsNotNone(final['finished_at'])
            if outcome == 'confirmed':
                self.assertEqual(code, 0)
                self.assertEqual(final['status'], 'ok')
                self.assertFalse((state/'active-job').exists())
                task.assert_called_once()
                self.assertEqual(task.call_args.args[1], 300)
            else:
                self.assertEqual(code, 1)
                self.assertEqual(final['status'], 'failed')
                self.assertIn('cancellation could not be confirmed', final['reason'])
                self.assertEqual(final['last_success_at'], '2026-01-01T00:00:00Z')
                self.assertEqual((state/'active-job').read_text(), token)
                task.assert_not_called()

    def test_helper_cannot_execute_is_bounded_and_retains_blocking_marker(self):
        self.check_recovery('cannot_execute')

    def test_helper_never_confirms_is_bounded_and_retains_blocking_marker(self):
        self.check_recovery('unconfirmed')

    def test_confirmation_within_deadline_allows_work_without_resetting_deadline(self):
        self.check_recovery('confirmed')

    def test_inflight_cancellation_uses_remaining_unit_time_once(self):
        with tempfile.TemporaryDirectory() as work:
            state = Path(work)
            clock = [0.0]
            actions = []
            def runner(args, timeout, **kwargs):
                action = args[4]
                actions.append(action)
                if action == 'run':
                    clock[0] = 298
                    kwargs['on_cancel']()
                    self.fail('unconfirmed cleanup returned')
                if action == 'cancel':
                    self.assertEqual(timeout, 1)
                    clock[0] += timeout
                    return 'unconfirmed'
                return ''
            with patch.object(process.time, 'monotonic', side_effect=lambda: clock[0]), \
                 patch.object(process, 'command', side_effect=runner):
                self.assertEqual(backup.run('health', state), 1)
            self.assertEqual(actions, ['prepare', 'run', 'cancel'])
            self.assertEqual(clock[0], 299)
            self.assertTrue((state/'active-job').exists())
            result = json.loads((state/'health.json').read_text())
            self.assertEqual(result['status'], 'failed')
            self.assertIn('cancellation could not be confirmed', result['reason'])

    def test_failed_remote_confirmation_still_reaps_host_client_within_deadline(self):
        with patch.object(process.subprocess, 'Popen') as popen, \
             patch.object(process.os, 'killpg') as kill, \
             patch.object(process.time, 'monotonic', return_value=99):
            child = popen.return_value.__enter__.return_value
            child.pid = 42
            child.communicate.side_effect = process.subprocess.TimeoutExpired('synthetic', 1)
            child.wait.side_effect = [process.subprocess.TimeoutExpired('synthetic', 1), 0]
            def cancel():
                raise process.CancellationUnconfirmed('synthetic unconfirmed cleanup')
            with self.assertRaisesRegex(process.CancellationUnconfirmed, 'unconfirmed cleanup'):
                process.command(['synthetic'], timeout=1, on_cancel=cancel, deadline=100)
            self.assertEqual(child.wait.call_args_list[0].kwargs, {'timeout': 1})
            self.assertEqual(kill.call_args_list[0].args, (42, process.signal.SIGTERM))
            self.assertEqual(kill.call_args_list[1].args, (42, process.signal.SIGKILL))
            self.assertEqual(child.wait.call_count, 2)

    def test_cancelled_client_cleanup_fits_the_systemd_stop_grace(self):
        with patch.object(process.subprocess, 'Popen') as popen, \
             patch.object(process.os, 'killpg'), \
             patch.object(process.time, 'monotonic', return_value=100):
            child = popen.return_value.__enter__.return_value
            child.pid = 42
            child.communicate.side_effect = process.subprocess.TimeoutExpired('synthetic', 1)
            child.wait.side_effect = [process.subprocess.TimeoutExpired('synthetic', 1), 0]
            def cancel():
                raise process.CancellationUnconfirmed('synthetic unconfirmed cleanup')
            with self.assertRaises(process.CancellationUnconfirmed):
                process.command(['synthetic'], timeout=1, on_cancel=cancel, deadline=1000)
            self.assertLessEqual(child.wait.call_args_list[0].kwargs['timeout'], 5)

    def test_interrupted_recovery_fails_without_swallowing_cancellation(self):
        with tempfile.TemporaryDirectory() as work:
            state = Path(work)
            (state/'active-job').write_text('a'*32)
            with patch.object(process, 'command', side_effect=[InterruptedError('signal'), AssertionError('retried interruption')]), \
                 patch.object(backup, 'work') as task:
                self.assertEqual(backup.run('health', state), 1)
            task.assert_not_called()
            self.assertEqual(json.loads((state/'health.json').read_text())['status'], 'failed')
            self.assertTrue((state/'active-job').exists())


if __name__ == '__main__':
    unittest.main()
