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


    def test_current_refreshes_last_success(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'result.json'
            log = Path(directory) / 'deploy.log'
            Result(path, log, clock=lambda: '2026-01-01T00:00:00Z').write('ok', 'complete', 'activated')
            Result(path, log, clock=lambda: '2026-01-02T00:00:00Z').write('current', 'noop', 'already active')
            self.assertEqual(json.loads(path.read_bytes())['last_success_at'], '2026-01-02T00:00:00Z')

    def test_safe_sdk_code_is_extracted_without_other_error_output(self):
        with self.assertRaises(CommandError) as raised:
            command(['/bin/sh', '-c', 'printf "OCI_ERROR:NotAuthorizedOrNotFound\\n" >&2; exit 1'],
                    error_prefix='OCI_ERROR:')
        self.assertEqual(raised.exception.code, 'NotAuthorizedOrNotFound')
        self.assertNotIn('NotAuthorizedOrNotFound', raised.exception.detail)

    def test_failed_command_detail_omits_stderr_of_other_programs(self):
        with self.assertRaises(CommandError) as raised:
            command(['/bin/sh', '-c', 'echo password=hunter2 >&2; exit 3'])
        self.assertEqual(raised.exception.detail, 'sh -c: exit 3')

    def test_failed_command_detail_skips_runuser_prefix(self):
        with self.assertRaises(CommandError) as raised:
            command(['/usr/bin/env', '--', 'sh', '-c', 'echo boom >&2; exit 1'])
        self.assertEqual(raised.exception.detail, 'sh -c: exit 1')

    def fake_tool(self, directory, name, stderr):
        path = Path(directory) / name
        path.write_text('#!/bin/sh\nprintf "%s\\n" "$STDERR_TEXT" >&2\nexit 1\n')
        path.chmod(0o755)
        return path, dict(STDERR_TEXT=stderr)

    def test_oras_detail_keeps_single_line_bounded_stderr_tail(self):
        with tempfile.TemporaryDirectory() as directory:
            path, env = self.fake_tool(directory, 'oras', 'first line\nError: ' + 'x' * 400)
            with mock.patch.dict('os.environ', env), self.assertRaises(CommandError) as raised:
                command([str(path), 'repo', 'tags', 'ghcr.io/x/y'])
        detail = raised.exception.detail
        self.assertTrue(detail.startswith('oras repo tags: exit 1: '))
        self.assertNotIn('\n', detail)
        self.assertEqual(len(detail), len('oras repo tags: exit 1: ') + 300)

    def test_oras_detail_withholds_stderr_that_mentions_credentials(self):
        with tempfile.TemporaryDirectory() as directory:
            path, env = self.fake_tool(directory, 'oras', 'error decoding auth: user:s3cret')
            with mock.patch.dict('os.environ', env), self.assertRaises(CommandError) as raised:
                command([str(path), 'resolve', 'ghcr.io/x/y:production'])
        self.assertEqual(raised.exception.detail, 'oras resolve: exit 1: error output withheld')

    def test_oras_detail_withholds_credentials_beyond_the_tail_window(self):
        with tempfile.TemporaryDirectory() as directory:
            path, env = self.fake_tool(directory, 'oras', 'password=' + 'x' * 400)
            with mock.patch.dict('os.environ', env), self.assertRaises(CommandError) as raised:
                command([str(path), 'resolve', 'ghcr.io/x/y:production'])
        self.assertEqual(raised.exception.detail, 'oras resolve: exit 1: error output withheld')

    def test_ghcr_anonymous_denied_on_rollback_tags_is_classified(self):
        stderr = b'Error response from registry: denied: requested access to the resource is denied\n'
        self.assertEqual(failure_reason(['/usr/local/libexec/trachuan/bin/oras', 'repo', 'tags'], stderr), 'denied')
        self.assertEqual(failure_reason(['/usr/local/libexec/trachuan/bin/oras', 'resolve'], stderr), 'denied')
        self.assertEqual(failure_reason(['/usr/local/libexec/trachuan/bin/oras', 'manifest', 'fetch'], stderr), 'failed')

if __name__ == '__main__':
    unittest.main()
