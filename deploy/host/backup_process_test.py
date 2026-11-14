import fcntl
import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

HOST = Path(__file__).resolve().parent
sys.path.insert(0, str(HOST))


def load():
    path = HOST / 'backup_process.py'
    if not path.exists():
        raise AssertionError('tracked container runner is missing')
    return __import__('backup_process')


class ContainerProcessTest(unittest.TestCase):
    def test_cancellation_confirms_remote_exit_before_final_result_and_lock_release(self):
        module = load()
        import backup
        for failure in [InterruptedError('signal'), subprocess.TimeoutExpired('backup', 100)]:
            with self.subTest(failure=type(failure).__name__), tempfile.TemporaryDirectory() as work:
                state = Path(work)
                calls = []
                def runner(args, timeout, **kwargs):
                    action = args[4]
                    calls.append(action)
                    self.assertEqual(json.loads((state/'verify.json').read_text())['status'], 'running')
                    with (state/'backup.lock').open('a') as lock:
                        with self.assertRaises(BlockingIOError):
                            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                    if action == 'run':
                        self.assertEqual(args[5], (state/'active-job').read_text())
                        self.assertLess(float(args[6]), timeout)
                        raise failure
                    if action == 'cancel':
                        return 'stopped\n'
                    return ''
                with patch.object(module, 'command', side_effect=runner):
                    self.assertEqual(backup.run('verify', state), 1)
                self.assertEqual(calls, ['prepare', 'run', 'cancel'])
                result = json.loads((state/'verify.json').read_text())
                self.assertEqual(result['status'], 'failed')
                self.assertFalse((state/'active-job').exists())
                with (state/'backup.lock').open('a') as lock:
                    fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)

    def test_failed_cancellation_keeps_marker_and_retries_while_lock_is_held(self):
        module = load()
        import backup
        with tempfile.TemporaryDirectory() as work:
            state = Path(work)
            calls = []
            def runner(args, timeout, **kwargs):
                action = args[4]
                calls.append(action)
                if action == 'run':
                    raise InterruptedError('signal')
                if action == 'cancel':
                    self.assertEqual(json.loads((state/'full.json').read_text())['status'], 'running')
                    self.assertTrue((state/'active-job').exists())
                    if calls.count('cancel') == 1:
                        raise module.CommandFailed()
                    return 'stopped\n'
                return ''
            with patch.object(module, 'command', side_effect=runner), patch.object(module.time, 'sleep'):
                self.assertEqual(backup.run('full', state), 1)
            self.assertEqual(calls, ['prepare', 'run', 'cancel', 'cancel'])

    def test_leftover_job_is_stopped_before_new_work(self):
        module = load()
        with tempfile.TemporaryDirectory() as work:
            state = Path(work)
            token = 'a'*32
            (state/'active-job').write_text(token)
            calls = []
            def runner(args, timeout, **kwargs):
                calls.append(args[4:])
                return 'stopped\n' if args[4] == 'cancel' else ''
            with patch.object(module, 'command', side_effect=runner):
                executor = module.Executor(state, module.time.monotonic()+100)
                executor.recover()
                executor.execute(['pgbackrest', '--stanza=trachuan', 'verify'], 100)
            self.assertEqual(calls[0], ['cancel', token])
            self.assertEqual([call[0] for call in calls], ['cancel', 'prepare', 'run', 'cancel'])

    def test_remote_cleanup_precedes_host_client_termination(self):
        module = load()
        events = []
        with patch.object(module.subprocess, 'Popen') as popen, patch.object(module.os, 'killpg', side_effect=lambda *args: events.append('client-stop')):
            child = popen.return_value.__enter__.return_value
            child.pid = 42
            child.communicate.side_effect = [subprocess.TimeoutExpired('restore', 1), ('', 'FATAL: recovery diagnostic')]
            with self.assertRaises(subprocess.TimeoutExpired) as failure:
                module.command(['synthetic'], timeout=1, diagnostics=True,
                               on_cancel=lambda: events.append('remote-stopped'))
            self.assertIn('FATAL: recovery diagnostic', failure.exception.stderr)
        self.assertEqual(events, ['remote-stopped', 'client-stop'])

    def test_restore_diagnostic_reaches_result_reason(self):
        module = load()
        import backup
        from backup_status_test import info, NOW
        with tempfile.TemporaryDirectory() as work:
            def execute(self, args, timeout):
                return info()
            def command(args, timeout, **kwargs):
                if 'inspect' in args:
                    return 'sha256:'+'1'*64
                raise module.CommandFailed('FATAL: synthetic recovery diagnostic')
            with patch.object(module.Executor, 'execute', execute), patch.object(backup, 'command', side_effect=command), patch.object(backup.time, 'time', return_value=NOW):
                self.assertEqual(backup.run('restore-check', Path(work)), 1)
            result = json.loads((Path(work)/'restore-check.json').read_text())
            self.assertIn('FATAL: synthetic recovery diagnostic', result['reason'])


if __name__ == '__main__':
    unittest.main()
