import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock

from updater.deploy_io import atomic, command, CommandError, failure_reason, Result


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


    def test_failed_command_detail_names_program_and_single_line_stderr_tail(self):
        with self.assertRaises(CommandError) as raised:
            command(['/bin/sh', '-c', 'printf "first\\nError: %s\\n" "$(printf x%.0s $(seq 400))" >&2; exit 3'])
        detail = raised.exception.detail
        self.assertTrue(detail.startswith('sh -c: '))
        self.assertNotIn('\n', detail)
        self.assertLessEqual(len(detail), len('sh -c: ') + 300)
        self.assertTrue(detail.endswith('x'))

    def test_failed_command_detail_skips_runuser_prefix(self):
        with self.assertRaises(CommandError) as raised:
            command(['/usr/bin/env', '--', 'sh', '-c', 'echo boom >&2; exit 1'])
        self.assertEqual(raised.exception.detail, 'sh -c: boom')

    def test_ghcr_anonymous_denied_on_rollback_tags_is_classified(self):
        stderr = b'Error response from registry: denied: requested access to the resource is denied\n'
        self.assertEqual(failure_reason(['/usr/local/libexec/trachuan/bin/oras', 'repo', 'tags'], stderr), 'denied')
        self.assertEqual(failure_reason(['/usr/local/libexec/trachuan/bin/oras', 'resolve'], stderr), 'failed')

if __name__ == '__main__':
    unittest.main()
