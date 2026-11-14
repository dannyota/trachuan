#!/usr/bin/env python3
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import uuid


def utc_now():
    return datetime.now(timezone.utc).isoformat(timespec='seconds').replace('+00:00', 'Z')


class Result:
    def __init__(self, path, now=utc_now):
        self.path = Path(path)
        if not self.path.parent.is_dir():
            raise FileNotFoundError(f'backup result directory is not provisioned: {self.path.parent}')
        self.now = now
        self.started = now()
        self.success = None
        try:
            self.success = json.loads(self.path.read_text())['last_success_at']
        except (OSError, ValueError, KeyError):
            pass

    def write(self, status, reason):
        if status not in {'running', 'ok', 'failed', 'unknown', 'skipped'}:
            raise ValueError('invalid backup status')
        finished = None if status == 'running' else self.now()
        success = finished if status == 'ok' else self.success
        payload = {'started_at': self.started, 'finished_at': finished,
                   'status': status, 'reason': reason, 'last_success_at': success}
        staging = self.path.with_name('.'+self.path.name+'.'+uuid.uuid4().hex)
        descriptor = os.open(staging, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o640)
        try:
            with os.fdopen(descriptor, 'w') as output:
                json.dump(payload, output, separators=(',', ':'))
                output.write('\n')
                output.flush()
                os.fsync(output.fileno())
            os.replace(staging, self.path)
            self.success = success
        finally:
            Path(staging).unlink(missing_ok=True)
        print(f'backup unit={self.path.stem} status={status} reason={reason}', flush=True)
