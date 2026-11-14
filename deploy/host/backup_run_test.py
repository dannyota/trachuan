import configparser
import fcntl
import subprocess
import importlib.util
import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

HOST = Path(__file__).resolve().parent
sys.path.insert(0, str(HOST))
from backup_status_test import info, NOW


def load():
    path = HOST / 'backup.py'
    if not path.exists():
        raise AssertionError('backup runner is missing')
    spec = importlib.util.spec_from_file_location('backup', path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class RunTest(unittest.TestCase):
    def test_missing_state_directory_fails_without_recreating_private_directory(self):
        module = load()
        with tempfile.TemporaryDirectory() as work:
            state = Path(work) / 'absent'
            with patch.object(module, 'work', return_value=('ok', 'synthetic')):
                with self.assertRaisesRegex(FileNotFoundError, 'directory is not provisioned'):
                    module.run('health', state)
            self.assertFalse(state.exists())

    def test_approved_deadlines_stop_work_and_record_failure_for_every_unit(self):
        module = load()
        limits = {'full': 14400, 'diff': 7200, 'verify': 14400, 'health': 300, 'restore-check': 1200}
        for kind, seconds in limits.items():
            with self.subTest(kind=kind), tempfile.TemporaryDirectory() as work:
                path = Path(work) / (kind+'.json')
                path.write_text(json.dumps({'last_success_at': '2026-01-01T00:00:00Z'}))
                def expired(actual_kind, deadline, executor):
                    self.assertEqual(actual_kind, kind)
                    self.assertEqual(deadline, 500+seconds)
                    self.assertEqual(json.loads(path.read_text())['status'], 'running')
                    raise subprocess.TimeoutExpired('synthetic-command', seconds)
                with patch.object(module.time, 'monotonic', return_value=500), patch.object(module, 'work', side_effect=expired):
                    self.assertEqual(module.run(kind, Path(work)), 1)
                result = json.loads(path.read_text())
                self.assertEqual(result['status'], 'failed')
                self.assertIsNotNone(result['finished_at'])
                self.assertEqual(result['last_success_at'], '2026-01-01T00:00:00Z')
                unit = configparser.ConfigParser(interpolation=None)
                unit.read(HOST / 'systemd' / f'trachuan-backup-{kind}.service')
                self.assertEqual(int(unit['Service']['TimeoutStartSec']), seconds)
                self.assertGreaterEqual(int(unit['Service']['TimeoutStopSec']), 30)

    def test_no_command_starts_after_deadline(self):
        module = load()
        with patch.object(module.time, 'monotonic', return_value=500), patch.object(module, 'command') as command:
            with self.assertRaises(subprocess.TimeoutExpired):
                module.work('verify', deadline=499)
            command.assert_not_called()

    def test_container_command_gets_its_own_remaining_deadline(self):
        module = load()
        with patch.object(module.time, 'monotonic', return_value=400), patch.object(module.process.Executor, 'execute', return_value='') as execute:
            module.work('verify', deadline=500, executor=module.process.Executor(Path('/unused'), 500))
        self.assertEqual(execute.call_args.args[0], ['pgbackrest', '--stanza=trachuan', 'verify', '--output=text', '--verbose'])
        self.assertEqual(execute.call_args.kwargs['timeout'], 100)

    def test_command_timeout_terminates_and_reaps_child(self):
        module = load()
        processes = []
        popen = subprocess.Popen
        def launch(*args, **kwargs):
            child = popen(*args, **kwargs)
            processes.append(child)
            return child
        with patch.object(module.subprocess, 'Popen', side_effect=launch):
            with self.assertRaises(subprocess.TimeoutExpired):
                module.command([sys.executable, '-c', 'import time; time.sleep(30)'], timeout=0.01)
        self.assertIsNotNone(processes[0].poll())

    def test_container_timeout_exit_is_reported_as_deadline_failure(self):
        module = load()
        with patch.object(module.subprocess, 'Popen') as popen:
            child = popen.return_value.__enter__.return_value
            child.communicate.return_value = ('', None)
            child.returncode = 124
            with self.assertRaises(subprocess.TimeoutExpired):
                module.command(['podman', 'exec', 'trachuan-postgres', 'timeout',
                                '--signal=TERM', '--kill-after=5s', '5.000s', 'pgbackrest', 'info'], timeout=5)

    def test_full_and_diff_expire_only_after_success(self):
        module = load()
        for kind in ['full', 'diff']:
            for fail in [False, True]:
                with self.subTest(kind=kind, fail=fail), tempfile.TemporaryDirectory() as work:
                    calls = []
                    def command(args, **kwargs):
                        calls.append(args)
                        if args[4] in {'prepare', 'cancel'}:
                            return 'stopped\n' if args[4] == 'cancel' else ''
                        state = json.loads((Path(work) / (kind+'.json')).read_text())
                        self.assertEqual(state['status'], 'running')
                        if args[-1] == '--output=json':
                            return info()
                        if fail:
                            raise module.CommandFailed()
                        return ''
                    with patch.object(module.process, 'command', side_effect=command), patch.object(module.time, 'time', return_value=NOW):
                        code = module.run(kind, Path(work))
                    state = json.loads((Path(work) / (kind+'.json')).read_text())
                    self.assertEqual(state['status'], 'failed' if fail else 'ok')
                    self.assertEqual(code, 1 if fail else 0)
                    self.assertIn(['podman', 'exec', 'trachuan-postgres', 'pgbackrest', '--stanza=trachuan', 'backup', '--type='+kind, '--no-expire-auto'], [args[:3]+args[7:] for args in calls])
                    self.assertEqual(any(args[-1] == 'expire' for args in calls), not fail)

    def test_backup_error_never_updates_last_success_or_expires(self):
        module = load()
        calls = []
        def command(args, **kwargs):
            calls.append(args)
            if args[4] in {'prepare', 'cancel'}:
                return 'stopped\n' if args[4] == 'cancel' else ''
            return info(error=True) if args[-1] == '--output=json' else ''
        with tempfile.TemporaryDirectory() as work:
            path = Path(work) / 'full.json'
            path.write_text(json.dumps({'last_success_at': '2026-01-01T00:00:00Z'}))
            with patch.object(module.process, 'command', side_effect=command), patch.object(module.time, 'time', return_value=NOW):
                self.assertEqual(module.run('full', Path(work)), 1)
            result = json.loads(path.read_text())
            self.assertEqual(result['status'], 'failed')
            self.assertEqual(result['last_success_at'], '2026-01-01T00:00:00Z')
        self.assertFalse(any(args[-1] == 'expire' for args in calls))

    def test_health_queries_archiver_even_when_repository_is_unavailable(self):
        module = load()
        calls = []
        def command(args, **kwargs):
            calls.append(args)
            if args[:4] == ['podman', 'exec', 'trachuan-postgres', '/usr/local/bin/backup-job.sh'] and args[4] in {'prepare', 'cancel'}:
                return 'stopped\n' if args[4] == 'cancel' else ''
            if args[-1] == '--output=json':
                raise module.CommandFailed()
            return json.dumps({'failed_count': 0, 'last_failed_time': None,
                               'last_archived_time': None, 'wal_bytes': 10_000_000_001})
        with tempfile.TemporaryDirectory() as work:
            with patch.object(module.process, 'command', side_effect=command):
                self.assertEqual(module.run('health', Path(work)), 1)
            self.assertEqual(json.loads((Path(work)/'health.json').read_text())['status'], 'failed')
        self.assertEqual(sum(args[4] == 'run' for args in calls), 2)

    def test_shared_lock_skips_every_kind(self):
        module = load()
        with tempfile.TemporaryDirectory() as work:
            root = Path(work)
            with (root / 'backup.lock').open('w') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                for kind in ['full', 'diff', 'verify', 'health', 'restore-check']:
                    with patch.object(module, 'command') as command:
                        self.assertEqual(module.run(kind, root), 0)
                        command.assert_not_called()
                    self.assertEqual(json.loads((root / (kind+'.json')).read_text())['status'], 'skipped')

    def test_info_unreachable_is_unknown(self):
        module = load()
        with tempfile.TemporaryDirectory() as work:
            with patch.object(module.process.Executor, 'execute', side_effect=module.CommandFailed()):
                self.assertEqual(module.run('health', Path(work)), 1)
            self.assertEqual(json.loads((Path(work) / 'health.json').read_text())['status'], 'unknown')

    def test_restore_selects_good_label_and_exact_live_image(self):
        module = load()
        calls = []
        def command(args, **kwargs):
            calls.append(args)
            if args[:4] == ['podman', 'exec', 'trachuan-postgres', '/usr/local/bin/backup-job.sh'] and args[4] in {'prepare', 'cancel'}:
                return 'stopped\n' if args[4] == 'cancel' else ''
            if args[-1] == '--output=json':
                return info()
            if 'inspect' in args:
                return 'sha256:' + '1'*64
            return ''
        with tempfile.TemporaryDirectory() as work:
            with patch.object(module.process, 'command', side_effect=command), patch.object(module.time, 'time', return_value=NOW):
                self.assertEqual(module.run('restore-check', Path(work)), 0)
        self.assertEqual(calls[-1], ['bash', str(HOST / 'restore-check.sh'), '--locked', '20270115-020000F', 'sha256:'+'1'*64])


if __name__ == '__main__':
    unittest.main()
