import os
import re
import selectors
import signal
import subprocess

from .deploy_corpus_sql import SCHEMA_PATH
from .deploy_io import Busy, CommandError


class AdvisoryLock:
    def __init__(self, database):
        self.db = database

    def __enter__(self):
        args = self.db.host.user_prefix + self.db.arguments('env', 'PGOPTIONS=' + SCHEMA_PATH, 'psql', '--no-password', '-XqAt', '-h', '127.0.0.1',
            '-U', 'activator', '-v', 'ON_ERROR_STOP=1', '-d', 'auth')
        self.process = subprocess.Popen(args, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                        stderr=subprocess.DEVNULL, start_new_session=True)
        try:
            self.process.stdin.write(b"SELECT pg_catalog.pg_try_advisory_lock(pg_catalog.hashtextextended('activation',0)),"
                                     b"pg_catalog.pg_backend_pid();\n")
            self.process.stdin.flush()
            held, _, backend = self.answer().partition(b'|')
            if held == b'f':
                raise Busy('activation advisory lock busy')
            if held != b't' or not re.fullmatch(rb'[1-9][0-9]*', backend):
                raise CommandError('activation advisory lock failed')
            self.db.holder, self.db.backend = self.process, int(backend)
            return self
        except BaseException:
            self.__exit__()
            raise

    def answer(self):
        data = b''
        with selectors.DefaultSelector() as selector:
            selector.register(self.process.stdout, selectors.EVENT_READ)
            while b'\n' not in data and len(data) < 64:
                if not selector.select(10):
                    raise CommandError('activation advisory lock timed out')
                chunk = os.read(self.process.stdout.fileno(), 64)
                if not chunk:
                    break
                data += chunk
        if data.count(b'\n') != 1 or not data.endswith(b'\n'):
            raise CommandError('activation advisory lock failed')
        return data[:-1]

    def __exit__(self, *args):
        self.db.holder, self.db.backend = None, None
        self.process.stdin.close()
        try:
            self.process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            os.killpg(self.process.pid, signal.SIGKILL)
            self.process.wait()
        self.process.stdout.close()
