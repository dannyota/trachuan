import contextlib
import importlib
import io
import os
import signal
import subprocess
import unittest

SECRET = 'LICENSED-SENTINEL-TEXT'


class FrameworksCliTest(unittest.TestCase):
    def setUp(self):
        self.module = importlib.import_module('frameworks_cli')
        saved = {number: signal.getsignal(number) for number in self.module.SIGNALS}
        self.addCleanup(lambda: [signal.signal(number, handler) for number, handler in saved.items()])

    def failure(self, error):
        def function():
            raise error
        output = io.StringIO()
        with contextlib.redirect_stderr(output), self.assertRaises(SystemExit) as caught:
            self.module.guarded(function)
        self.assertEqual(caught.exception.code, 1)
        return output.getvalue().strip()

    def test_own_refusals_print_their_fixed_message(self):
        self.assertEqual(self.failure(ValueError('report quality_pass must be true')), 'report quality_pass must be true')

    def test_other_exceptions_print_only_their_class(self):
        self.assertEqual(self.failure(OSError('/work/' + SECRET)), 'OSError')
        self.assertEqual(self.failure(KeyError(SECRET)), 'KeyError')
        self.assertEqual(self.failure(UnicodeDecodeError('utf-8', SECRET.encode(), 0, 1, SECRET)), 'UnicodeDecodeError')

    def test_failed_commands_print_tool_and_status_only(self):
        error = subprocess.CalledProcessError(3, ['/usr/bin/aws', 's3api', SECRET], stderr=SECRET)
        self.assertEqual(self.failure(error), 'command failed: aws exit 3')

    def test_termination_unwinds_through_cleanup_once(self):
        steps = []

        def function():
            try:
                os.kill(os.getpid(), signal.SIGTERM)
                signal.pause()
            finally:
                os.kill(os.getpid(), signal.SIGTERM)
                steps.append('cleaned')

        with self.assertRaises(SystemExit) as caught:
            self.module.guarded(function)
        self.assertEqual(caught.exception.code, 128 + signal.SIGTERM)
        self.assertEqual(steps, ['cleaned'])


if __name__ == '__main__':
    unittest.main()
