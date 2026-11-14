from datetime import datetime, timezone
import fcntl
import json
import os
import re
from pathlib import Path
import subprocess
import signal
import selectors
import time
import tempfile


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(',', ':'), ensure_ascii=False).encode()


def atomic(path, data, mode=0o600):
    path = Path(path)
    descriptor, name = tempfile.mkstemp(prefix='.' + path.name + '.', dir=path.parent)
    try:
        with os.fdopen(descriptor, 'wb') as output:
            os.fchmod(output.fileno(), mode)
            output.write(data)
            output.flush()
            os.fsync(output.fileno())
        os.replace(name, path)
        directory = os.open(path.parent, os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(directory)
        finally:
            os.close(directory)
    finally:
        Path(name).unlink(missing_ok=True)


def now():
    return datetime.now(timezone.utc).isoformat(timespec='seconds').replace('+00:00', 'Z')


def remove(path):
    path = Path(path)
    try:
        path.unlink()
    except FileNotFoundError:
        return
    descriptor = os.open(path.parent, os.O_RDONLY | os.O_DIRECTORY)
    try:
        os.fsync(descriptor)
    finally:
        os.close(descriptor)


class Busy(Exception):
    pass


class Lock:
    def __init__(self, path, create=True):
        self.path, self.create = path, create

    def __enter__(self):
        descriptor = os.open(self.path, (os.O_CREAT if self.create else 0) | os.O_RDWR | os.O_NOFOLLOW, 0o600)
        self.file = os.fdopen(descriptor, 'r+')
        try:
            fcntl.flock(self.file, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError as error:
            self.file.close()
            raise Busy('lock busy') from error
        return self

    def __exit__(self, *args):
        self.file.close()


class CommandError(Exception):
    def __init__(self, message, reason='failed'):
        super().__init__(message)
        self.reason = reason


def failure_reason(args, stderr):
    text = stderr.decode('utf-8', errors='replace').lower()
    if Path(args[0]).name == 'oras' and args[1:3] == ['repo', 'tags']:
        if re.search(r'(?:^|[:\n]\s*)(?:name unknown|repository not found)(?::|$)', text):
            return 'not_found'
        if re.search(r'get "[^"\n]+/v2/[^"\n]+/tags/list(?:\?[^"\n]*)?": response status code 404:', text):
            return 'not_found'
    if Path(args[0]).name == 'cosign' and 'verify-attestation' in args:
        if 'none of the expected identities matched what was in the certificate' in text:
            return 'identity'
        if 'no matching certificateidentity found' in text and ('expected san value' in text or 'expected san value to match regex' in text):
            return 'identity'
    return 'failed'


def bounded_output(process, limit, timeout):
    deadline = time.monotonic() + timeout
    output, errors = bytearray(), bytearray()
    with selectors.DefaultSelector() as selector:
        selector.register(process.stdout, selectors.EVENT_READ, output)
        selector.register(process.stderr, selectors.EVENT_READ, errors)
        while selector.get_map():
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise subprocess.TimeoutExpired('host command', timeout)
            ready = selector.select(remaining)
            if not ready:
                raise subprocess.TimeoutExpired('host command', timeout)
            for key, _ in ready:
                size = min(65536, limit + 1 - len(output)) if key.data is output else 65536
                chunk = os.read(key.fd, size)
                if not chunk:
                    selector.unregister(key.fileobj)
                elif key.data is output:
                    output.extend(chunk)
                    if len(output) > limit:
                        raise CommandError('command output exceeds byte limit')
                else:
                    errors.extend(chunk[:max(0, 65536 - len(errors))])
        process.wait(timeout=max(0, deadline - time.monotonic()))
    return bytes(output), bytes(errors)


def command(args, input=None, timeout=300, stdout_limit=None):
    try:
        with subprocess.Popen(args, stdin=subprocess.PIPE if input is not None else subprocess.DEVNULL,
                              stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True) as process:
            try:
                if stdout_limit is None:
                    output, errors = process.communicate(input=input, timeout=timeout)
                else:
                    if input is not None or type(stdout_limit) is not int or stdout_limit < 0:
                        raise ValueError('invalid streaming command limits')
                    output, errors = bounded_output(process, stdout_limit, timeout)
                if process.returncode:
                    raise CommandError('command failed', reason=failure_reason(args, errors))
            except BaseException:
                try:
                    os.killpg(process.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
                process.communicate()
                raise
    except (OSError, subprocess.SubprocessError) as error:
        raise CommandError('command failed') from error
    if len(output) > 16 * 1024 * 1024:
        raise CommandError('command output too large')
    return output


class Result:
    def __init__(self, path, log, clock=now):
        self.path, self.log, self.clock = Path(path), Path(log), clock
        self.started = clock()
        self.previous = {}
        try:
            self.previous = json.loads(self.path.read_bytes())
            if not isinstance(self.previous, dict):
                self.previous = {}
        except (OSError, ValueError):
            pass

    def write(self, status, step, message, digest=None, manifest=None):
        manifest = manifest or {}
        finished = self.clock()
        success = finished if status in ('ok', 'verified', 'bootstrapped') else self.previous.get('last_success_at')
        value = dict(status=status, manifest_hash=digest, release=manifest.get('release'),
                     sequence=manifest.get('sequence'), step=step, message=message,
                     started_at=self.started, finished_at=finished, last_success_at=success)
        atomic(self.path, canonical(value) + b'\n', 0o640)
        self.append(value)

    def append(self, value):
        descriptor = os.open(self.log, os.O_WRONLY | os.O_APPEND | os.O_CREAT | os.O_NOFOLLOW, 0o640)
        with os.fdopen(descriptor, 'ab') as output:
            output.write(canonical(dict(timestamp=self.clock(), **value)) + b'\n')
            output.flush()
            os.fsync(output.fileno())
