import os
import selectors
import signal
import subprocess

from .deploy_io import Busy, CommandError


class AdvisoryLock:
    def __init__(self, database):
        self.db = database

    def __enter__(self):
        args = self.db.host.user_prefix + self.db.arguments('psql', '--no-password', '-XqAt', '-h', '127.0.0.1',
            '-U', 'activator', '-v', 'ON_ERROR_STOP=1', '-d', 'auth')
        self.process = subprocess.Popen(args, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                        stderr=subprocess.DEVNULL, start_new_session=True)
        try:
            self.process.stdin.write(b"SELECT pg_try_advisory_lock(hashtextextended('activation',0));\n")
            self.process.stdin.flush()
            with selectors.DefaultSelector() as selector:
                selector.register(self.process.stdout, selectors.EVENT_READ)
                if not selector.select(10):
                    raise CommandError('activation advisory lock timed out')
                answer = os.read(self.process.stdout.fileno(), 16).strip()
            if answer == b'f':
                raise Busy('activation advisory lock busy')
            if answer != b't':
                raise CommandError('activation advisory lock failed')
            self.db.holder = self.process
            return self
        except BaseException:
            self.__exit__()
            raise

    def __exit__(self, *args):
        self.db.holder = None
        self.process.stdin.close()
        try:
            self.process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            os.killpg(self.process.pid, signal.SIGKILL)
            self.process.wait()
        self.process.stdout.close()
