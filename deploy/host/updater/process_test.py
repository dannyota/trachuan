import os
import select
import signal
import subprocess
import sys
import unittest
from unittest import mock

from updater import deploy_io as io


class ProcessTest(unittest.TestCase):
    def test_only_cosign_certificate_identity_mismatch_is_an_identity_failure(self):
        for message, expected in [
                ('none of the expected identities matched what was in the certificate', 'identity'),
                ('no matching CertificateIdentity found, last error: expected SAN value to match regex', 'identity'),
                ('certificate chain verification failed', 'failed'),
                ('failed to download attestation', 'failed')]:
            self.assertEqual(io.failure_reason(['cosign', 'verify-attestation'], message.encode()), expected)

    def test_only_explicit_oras_repository_absence_is_classified_as_missing(self):
        real_popen = subprocess.Popen
        errors = [('Error response from registry: name unknown: repository name not known', 'not_found'),
                  ('Error response from registry: repository not found', 'not_found'),
                  ('Error: GET "https://ghcr.io/v2/example/release/tags/list": response status code 404: Not Found', 'not_found'),
                  ('Error response from registry: denied: requested access is denied', 'failed'),
                  ('Error: GET "https://ghcr.io/token": response status code 404: Not Found', 'failed'),
                  ('Error: connection refused', 'failed')]
        for stderr, expected in errors:
            def spawn(args, **kwargs):
                return real_popen([sys.executable, '-c', 'import sys; sys.stderr.write(sys.argv[1]); sys.exit(1)', stderr], **kwargs)
            with self.subTest(stderr=stderr), mock.patch.object(io.subprocess, 'Popen', side_effect=spawn):
                with self.assertRaises(io.CommandError) as raised:
                    io.command(['/usr/local/libexec/trachuan/bin/oras', 'repo', 'tags', '--format', 'json', 'ghcr.io/example/release'])
                self.assertEqual(getattr(raised.exception, 'reason', None), expected)

    def test_stream_limit_stops_the_writer_before_it_can_finish(self):
        real_popen = subprocess.Popen
        children = []
        def spawn(*args, **kwargs):
            process = real_popen(*args, **kwargs)
            children.append(process)
            return process
        source = 'import os, signal; os.write(1, b"x" * 1048576); signal.pause()'
        with mock.patch.object(io.subprocess, 'Popen', side_effect=spawn):
            with self.assertRaisesRegex(io.CommandError, 'output exceeds'):
                io.command([sys.executable, '-c', source], stdout_limit=64)
        self.assertEqual(children[0].returncode, -signal.SIGKILL)
        self.assertEqual(io.command([sys.executable, '-c', 'print("ok", end="")'], stdout_limit=2), b'ok')

    def test_timeout_kills_nested_child_before_returning(self):
        processes = []
        real_popen = subprocess.Popen
        observed = []
        reader, writer = os.pipe()
        def spawn(args, **kwargs):
            observed.append(kwargs)
            process = real_popen(args, pass_fds=(writer,), **kwargs)
            os.close(writer)
            communicate = process.communicate
            def expire(input=None, timeout=None):
                if not processes:
                    processes.extend(int(value) for value in process.stdout.readline().split())
                    raise subprocess.TimeoutExpired(args, timeout)
                return communicate(input=input, timeout=timeout)
            process.communicate = expire
            return process
        source = ('import os, signal, subprocess, sys\n'
                  'child = subprocess.Popen([sys.executable, "-c", "import signal; signal.pause()"], '
                  f'pass_fds=({writer},), stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)\n'
                  f'os.close({writer})\n'
                  'print(os.getpid(), child.pid, flush=True)\n'
                  'signal.pause()\n')
        try:
            with mock.patch.object(io.subprocess, 'Popen', side_effect=spawn):
                with self.assertRaises(io.CommandError):
                    io.command([sys.executable, '-c', source], timeout=0.01)
            self.assertEqual(len(processes), 2)
            ready = select.select([reader], [], [], 0.08)[0]
            self.assertTrue(ready, 'grandchild still holds its pipe after timeout')
            self.assertEqual(os.read(reader, 1), b'')
            self.assertTrue(observed[0].get('start_new_session'))
        finally:
            for pid in processes:
                try:
                    os.kill(pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
            os.close(reader)


if __name__ == '__main__':
    unittest.main()
