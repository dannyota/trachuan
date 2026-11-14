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


class CancellationUnconfirmed(RuntimeError):
    pass


def command(args, timeout, diagnostics=False, on_cancel=None, deadline=None):
    if timeout <= 0:
        raise subprocess.TimeoutExpired(args, timeout)
    with subprocess.Popen(args, stdout=subprocess.PIPE,
                          stderr=subprocess.PIPE if diagnostics else subprocess.DEVNULL,
                          text=True, start_new_session=True) as child:
        try:
            output, error = child.communicate(timeout=timeout)
        except BaseException as failure:
            cleanup_error = None
            try:
                if on_cancel:
                    on_cancel()
            except BaseException as error:
                cleanup_error = error
            try:
                os.killpg(child.pid, signal.SIGTERM)
            except ProcessLookupError:
                pass
            try:
                grace = 5 if on_cancel else 15
                remaining = grace if deadline is None else max(0, min(grace, deadline-time.monotonic()))
                child.wait(timeout=remaining)
            except subprocess.TimeoutExpired:
                try:
                    os.killpg(child.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
                child.wait()
            if cleanup_error is not None:
                raise cleanup_error from failure
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
    def __init__(self, state, deadline):
        self.deadline = deadline
        self.marker = Path(state) / 'active-job'
        self.prefix = ['podman', 'exec', 'trachuan-postgres', '/usr/local/bin/backup-job.sh']

    def recover(self):
        if not self.marker.exists():
            return
        token = self.marker.read_text()
        if not re.fullmatch('[0-9a-f]{32}', token):
            raise CancellationUnconfirmed('backup cancellation could not be confirmed: invalid active-job marker')
        deadline = min(self.deadline-1, time.monotonic()+20)
        while time.monotonic() < deadline:
            try:
                output = command(self.prefix+['cancel', token], timeout=min(10, deadline-time.monotonic()), deadline=deadline)
                if output.strip() != 'stopped':
                    raise CommandFailed()
                self.marker.unlink()
                return
            except InterruptedError as error:
                raise CancellationUnconfirmed('backup cancellation could not be confirmed: interrupted; active-job retained') from error
            except (CommandFailed, OSError, subprocess.TimeoutExpired):
                pass
            remaining = deadline-time.monotonic()
            if remaining > 0:
                time.sleep(min(0.1, remaining))
        raise CancellationUnconfirmed('backup cancellation could not be confirmed before the deadline; active-job retained')

    def execute(self, args, timeout):
        self.recover()
        remaining = min(timeout, self.deadline-time.monotonic()-1)
        if remaining <= 20:
            raise subprocess.TimeoutExpired(args, remaining)
        token = uuid.uuid4().hex
        staging = self.marker.with_suffix('.tmp')
        staging.write_text(token)
        staging.chmod(0o600)
        staging.replace(self.marker)
        cancelled = False
        def cancel():
            nonlocal cancelled
            cancelled = True
            self.recover()
        try:
            command(self.prefix+['prepare', token], timeout=min(10, remaining-20), deadline=self.deadline-1)
            remaining = min(timeout, self.deadline-time.monotonic()-1)
            if remaining <= 20:
                raise subprocess.TimeoutExpired(args, remaining)
            return command(self.prefix+['run', token, f'{remaining-20:.3f}', *args],
                           timeout=remaining-10, on_cancel=cancel, deadline=self.deadline-1)
        finally:
            if not cancelled:
                self.recover()
