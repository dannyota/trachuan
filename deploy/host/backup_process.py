#!/usr/bin/env python3
import os
from pathlib import Path
import re
import signal
import subprocess
import time
import uuid


class CommandFailed(RuntimeError):
    pass


def command(args, timeout, diagnostics=False, on_cancel=None):
    if timeout <= 0:
        raise subprocess.TimeoutExpired(args, timeout)
    with subprocess.Popen(args, stdout=subprocess.PIPE,
                          stderr=subprocess.PIPE if diagnostics else subprocess.DEVNULL,
                          text=True, start_new_session=True) as child:
        try:
            output, error = child.communicate(timeout=timeout)
        except BaseException as failure:
            if on_cancel:
                on_cancel()
            try:
                os.killpg(child.pid, signal.SIGTERM)
            except ProcessLookupError:
                pass
            try:
                child.wait(timeout=15)
            except subprocess.TimeoutExpired:
                try:
                    os.killpg(child.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
                child.wait()
            if diagnostics:
                _, error = child.communicate()
                detail = ' '.join((error or '').split())[-4096:]
                if isinstance(failure, subprocess.TimeoutExpired):
                    failure.stderr = detail
                elif detail:
                    raise CommandFailed('interrupted: '+detail) from failure
            raise
        if child.returncode in {124, 137}:
            raise subprocess.TimeoutExpired(args, timeout)
        if child.returncode:
            raise CommandFailed(' '.join((error or '').split())[-4096:] if diagnostics else '')
        return output


class Executor:
    def __init__(self, state):
        self.marker = Path(state) / 'active-job'
        self.prefix = ['podman', 'exec', 'trachuan-postgres', '/usr/local/bin/backup-job.sh']

    def recover(self):
        if not self.marker.exists():
            return
        token = self.marker.read_text()
        if not re.fullmatch('[0-9a-f]{32}', token):
            raise ValueError('invalid active backup job')
        while True:
            try:
                output = command(self.prefix+['cancel', token], timeout=10)
                if output.strip() != 'stopped':
                    raise CommandFailed()
                self.marker.unlink()
                return
            except (CommandFailed, OSError, subprocess.TimeoutExpired, InterruptedError):
                time.sleep(0.1)

    def execute(self, args, timeout):
        if timeout <= 20:
            raise subprocess.TimeoutExpired(args, timeout)
        self.recover()
        token = uuid.uuid4().hex
        staging = self.marker.with_suffix('.tmp')
        staging.write_text(token)
        staging.chmod(0o600)
        staging.replace(self.marker)
        try:
            command(self.prefix+['prepare', token], timeout=min(10, timeout-20))
            return command(self.prefix+['run', token, f'{timeout-20:.3f}', *args],
                           timeout=timeout, on_cancel=self.recover)
        finally:
            self.recover()
