from datetime import datetime, timezone
import fcntl
import json
import os
from pathlib import Path
import subprocess
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
    pass


def command(args, input=None, timeout=300):
    try:
        result = subprocess.run(args, input=input, stdout=subprocess.PIPE,
                                stderr=subprocess.PIPE, timeout=timeout, check=True)
    except (OSError, subprocess.SubprocessError) as error:
        raise CommandError('command failed') from error
    if len(result.stdout) > 16 * 1024 * 1024:
        raise CommandError('command output too large')
    return result.stdout


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
