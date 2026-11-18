import os
import re
import selectors
import signal
import subprocess

from .deploy_corpus_sql import LOCK_OPTIONS
from .deploy_io import CommandError
from .deploy_release_target import LockBusy, LockUnavailable


KEY = b"pg_catalog.hashtextextended('activation',0)"
TAKE = b'SELECT pg_catalog.pg_try_advisory_lock(' + KEY + b'),pg_catalog.pg_backend_pid();\n'
HOLDERS = (b'SELECT pg_catalog.count(*),pg_catalog.count(*) FILTER (WHERE a.usename=session_user) '
           b'FROM pg_catalog.pg_locks l JOIN pg_catalog.pg_stat_activity a ON a.pid=l.pid '
           b"WHERE l.locktype='advisory' AND l.granted AND l.objsubid=1 AND l.pid<>pg_catalog.pg_backend_pid() "
           b'AND l.database=(SELECT d.oid FROM pg_catalog.pg_database d WHERE d.datname=pg_catalog.current_database()) '
           b'AND l.classid::pg_catalog.int8=((' + KEY + b' >> 32) & 4294967295) '
           b'AND l.objid::pg_catalog.int8=(' + KEY + b' & 4294967295);\n')


class AdvisoryLock:
    def __init__(self, database):
        self.db = database

    def __enter__(self):
        args = self.db.host.user_prefix + self.db.arguments(*self.db.session('auth', LOCK_OPTIONS))
        self.process = subprocess.Popen(args, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                        stderr=subprocess.DEVNULL, start_new_session=True)
        try:
            held, _, backend = self.ask(TAKE).partition(b'|')
            if held == b'f':
                self.busy()
            if held != b't' or not re.fullmatch(rb'[1-9][0-9]*', backend):
                raise CommandError('activation advisory lock failed')
            self.db.holder, self.db.backend = self.process, int(backend)
            return self
        except BaseException:
            self.__exit__()
            raise

    def busy(self):
        found = re.fullmatch(rb'([0-9]+)\|([0-9]+)', self.ask(HOLDERS))
        if not found:
            raise CommandError('activation advisory lock failed')
        if found[1] != found[2]:
            raise LockUnavailable('activation lock held by another role')
        raise LockBusy('activation advisory lock busy')

    def ask(self, statement):
        try:
            self.process.stdin.write(statement)
            self.process.stdin.flush()
        except BrokenPipeError:
            self.closed()
        return self.answer()

    def closed(self):
        try:
            code = self.process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            code = None
        if code == 2:
            raise LockUnavailable('activator cannot connect to auth')
        raise CommandError('activation advisory lock failed')

    def answer(self):
        data = b''
        with selectors.DefaultSelector() as selector:
            selector.register(self.process.stdout, selectors.EVENT_READ)
            while b'\n' not in data and len(data) < 64:
                if not selector.select(10):
                    raise CommandError('activation advisory lock timed out')
                chunk = os.read(self.process.stdout.fileno(), 64)
                if not chunk:
                    if not data:
                        self.closed()
                    break
                data += chunk
        if data.count(b'\n') != 1 or not data.endswith(b'\n'):
            raise CommandError('activation advisory lock failed')
        return data[:-1]

    def __exit__(self, *args):
        self.db.holder, self.db.backend = None, None
        try:
            self.process.stdin.close()
        except BrokenPipeError:
            pass
        try:
            self.process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            os.killpg(self.process.pid, signal.SIGKILL)
            self.process.wait()
        self.process.stdout.close()
