import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock

from updater.deploy_io import atomic, Result


class IOTest(unittest.TestCase):
    def test_atomic_rename_failure_leaves_previous_bytes_and_no_temporary_file(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'state.json'
            path.write_bytes(b'previous')
            with mock.patch('updater.deploy_io.os.replace', side_effect=OSError('synthetic')):
                with self.assertRaises(OSError):
                    atomic(path, b'next')
            self.assertEqual(path.read_bytes(), b'previous')
            self.assertEqual(list(Path(directory).iterdir()), [path])

    def test_invalid_previous_result_does_not_block_failure_reporting(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'result.json'
            log = Path(directory) / 'deploy.log'
            path.write_text('[]')
            result = Result(path, log, clock=lambda: '2026-01-01T00:00:00Z')
            result.write('failed', 'manifest', 'manifest rejected')
            self.assertEqual(json.loads(path.read_bytes())['status'], 'failed')
            self.assertEqual(path.stat().st_mode & 0o777, 0o640)

    def test_failure_preserves_last_success_without_logging_command_output(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'result.json'
            log = Path(directory) / 'deploy.log'
            Result(path, log, clock=lambda: '2026-01-01T00:00:00Z').write('ok', 'complete', 'activated')
            Result(path, log, clock=lambda: '2026-01-02T00:00:00Z').write('failed', 'manifest', 'manifest rejected')
            value = json.loads(path.read_bytes())
            self.assertEqual(value['last_success_at'], '2026-01-01T00:00:00Z')
            self.assertEqual(value['finished_at'], '2026-01-02T00:00:00Z')


if __name__ == '__main__':
    unittest.main()
