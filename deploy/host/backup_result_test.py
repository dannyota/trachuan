import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

HOST = Path(__file__).resolve().parent


def load():
    path = HOST / 'backup_result.py'
    if not path.exists():
        raise AssertionError('result writer is missing')
    spec = importlib.util.spec_from_file_location('backup_result', path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class ResultTest(unittest.TestCase):
    def setUp(self):
        previous = os.umask(0o027)
        self.addCleanup(os.umask, previous)

    def test_temporary_file_requests_acl_compatible_mode_without_chmod(self):
        module = load()
        with tempfile.TemporaryDirectory() as work:
            target = Path(work) / 'health.json'
            result = module.Result(target)
            with patch.object(module.os, 'open', wraps=module.os.open) as create, patch.object(module.os, 'chmod') as chmod:
                result.write('unknown', 'synthetic unavailable repository')
            create.assert_called_once()
            path, flags, mode = create.call_args.args
            self.assertEqual(Path(path).parent, target.parent)
            self.assertNotEqual(Path(path), target)
            self.assertEqual(flags, os.O_CREAT | os.O_EXCL | os.O_WRONLY)
            self.assertEqual(mode, 0o640)
            chmod.assert_not_called()
            self.assertEqual(target.stat().st_mode & 0o777, 0o640)
            self.assertEqual(json.loads(target.read_text())['status'], 'unknown')

    def test_missing_result_directory_fails_without_creating_it(self):
        module = load()
        with tempfile.TemporaryDirectory() as work:
            target = Path(work) / 'absent' / 'health.json'
            with self.assertRaisesRegex(FileNotFoundError, 'directory is not provisioned'):
                module.Result(target).write('running', 'starting')
            self.assertFalse(target.parent.exists())

    def test_running_then_final_preserves_last_success_for_every_status(self):
        module = load()
        with tempfile.TemporaryDirectory() as work:
            path = Path(work) / 'full.json'
            path.write_text(json.dumps({'last_success_at': '2026-01-01T00:00:00Z'}))
            for status in ['ok', 'failed', 'unknown', 'skipped']:
                with self.subTest(status=status):
                    before = json.loads(path.read_text())['last_success_at']
                    result = module.Result(path, now=lambda: '2026-01-02T00:00:00Z')
                    result.write('running', 'starting')
                    running = json.loads(path.read_text())
                    self.assertEqual(running['status'], 'running')
                    self.assertIsNone(running['finished_at'])
                    self.assertEqual(running['last_success_at'], before)
                    result.write(status, 'completed')
                    final = json.loads(path.read_text())
                    self.assertEqual(final['started_at'], running['started_at'])
                    self.assertEqual(final['finished_at'], '2026-01-02T00:00:00Z')
                    self.assertEqual(final['last_success_at'], final['finished_at'] if status == 'ok' else before)
                    self.assertEqual(path.stat().st_mode & 0o777, 0o640)

    def test_replace_failure_leaves_complete_previous_file_and_no_staging_file(self):
        module = load()
        with tempfile.TemporaryDirectory() as work:
            path = Path(work) / 'health.json'
            path.write_text('{"last_success_at":null}')
            original = path.read_bytes()
            with patch.object(module.os, 'replace', side_effect=OSError('synthetic failure')):
                with self.assertRaises(OSError):
                    module.Result(path).write('running', 'starting')
            self.assertEqual(path.read_bytes(), original)
            self.assertEqual(list(Path(work).iterdir()), [path])

    def test_atomic_replace_and_journal_line_for_each_change(self):
        module = load()
        with tempfile.TemporaryDirectory() as work:
            path = Path(work) / 'verify.json'
            replace = module.os.replace
            seen = []
            def observe(source, destination):
                payload = json.loads(Path(source).read_text())
                seen.append(payload['status'])
                self.assertEqual(Path(source).parent, path.parent)
                replace(source, destination)
            with patch.object(module.os, 'replace', side_effect=observe), patch('builtins.print') as journal:
                result = module.Result(path)
                result.write('running', 'starting')
                result.write('failed', 'verification failed')
            self.assertEqual(seen, ['running', 'failed'])
            self.assertEqual(journal.call_count, 2)
            self.assertIn('verify', journal.call_args.args[0])
            self.assertIn('failed', journal.call_args.args[0])


if __name__ == '__main__':
    unittest.main()
