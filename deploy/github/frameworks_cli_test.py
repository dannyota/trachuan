import contextlib
import importlib
import io
import os
from pathlib import Path
import signal
import subprocess
import sys
import unittest
from unittest.mock import patch

SECRET = 'LICENSED-SENTINEL-TEXT'


class FrameworksCliTest(unittest.TestCase):
    def setUp(self):
        self.module = importlib.import_module('frameworks_cli')
        saved = {number: signal.getsignal(number) for number in self.module.SIGNALS}
        self.addCleanup(lambda: [signal.signal(number, handler) for number, handler in saved.items()])
        patcher = patch('frameworks_cli.resource.setrlimit')
        self.setrlimit = patcher.start()
        self.addCleanup(patcher.stop)

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
        self.assertEqual(set(self.module.SIGNALS), {signal.SIGHUP, signal.SIGINT, signal.SIGQUIT, signal.SIGTERM})
        for number in self.module.SIGNALS:
            steps = []

            def function():
                try:
                    os.kill(os.getpid(), number)
                    signal.pause()
                finally:
                    os.kill(os.getpid(), number)
                    steps.append('cleaned')

            with self.subTest(number=number), self.assertRaises(SystemExit) as caught:
                self.module.guarded(function)
            self.assertEqual(caught.exception.code, 128 + number)
            self.assertEqual(steps, ['cleaned'])

    def test_core_dumps_are_disabled_before_the_work_starts(self):
        order = []
        self.setrlimit.side_effect = lambda *args: order.append('limit')
        self.module.guarded(lambda: order.append('work'))
        self.assertEqual(order, ['limit', 'work'])
        self.setrlimit.assert_called_once_with(self.module.resource.RLIMIT_CORE, (0, 0))

    def test_core_limit_reaches_the_process_and_its_children(self):
        script = ('import resource,subprocess,sys;from frameworks_cli import guarded;'
                  'guarded(lambda: print(resource.getrlimit(resource.RLIMIT_CORE), subprocess.check_output('
                  '[sys.executable,"-I","-c","import resource;print(resource.getrlimit(resource.RLIMIT_CORE))"],text=True).strip()))')
        result = subprocess.run([sys.executable, '-B', '-c', script], cwd=Path(__file__).parent, capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.strip(), '(0, 0) (0, 0)')


if __name__ == '__main__':
    unittest.main()
