from datetime import datetime, timezone
import json
import math
import os
from pathlib import Path
import stat
import tempfile

from .signals import fresh


def read(path):
    try:
        descriptor = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    except FileNotFoundError:
        return None
    with os.fdopen(descriptor, 'rb') as source:
        if not stat.S_ISREG(os.fstat(source.fileno()).st_mode):
            raise ValueError('invalid watch input')
        raw = source.read(65537)
    if len(raw) > 65536:
        raise ValueError('invalid watch input')
    value = json.loads(raw)
    if not isinstance(value, dict):
        raise ValueError('invalid watch input')
    return value


def number(value):
    return type(value) is int or (type(value) is float and math.isfinite(value))


def timestamp(value):
    if value is None:
        return None
    if not isinstance(value, str):
        raise ValueError('invalid watch timestamp')
    parsed = datetime.fromisoformat(value.replace('Z', '+00:00'))
    if parsed.utcoffset() != timezone.utc.utcoffset(parsed):
        raise ValueError('invalid watch timestamp')
    return parsed.timestamp()


def utc(now):
    return datetime.fromtimestamp(now, timezone.utc).isoformat(timespec='seconds').replace('+00:00', 'Z')


def atomic(path, value, mode=0o600):
    path = Path(path)
    descriptor, name = tempfile.mkstemp(prefix='.'+path.name+'.', dir=path.parent)
    try:
        with os.fdopen(descriptor, 'w') as output:
            os.fchmod(output.fileno(), mode)
            json.dump(value, output, separators=(',', ':'), allow_nan=False)
            output.write('\n')
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


def save(path, state):
    atomic(path, state)


def valid_state(value):
    if not isinstance(value, dict) or set(value) != set(fresh()) or type(value['version']) is not int or value['version'] != 1:
        return False
    if not number(value['retry_at']) or not isinstance(value['sends'], list):
        return False
    if not all(number(sent) for sent in value['sends']):
        return False
    if not isinstance(value['signals'], dict) or not isinstance(value['events'], dict):
        return False
    keys = {'site', 'deploy_stopped', 'deploy_failed', 'release_failed', 'release_waiting',
            'secrets', 'backup_old', 'memory', 'disk:/', 'disk:/srv/trachuan/data',
            *('backup_'+job for job in ('full', 'diff', 'verify', 'health', 'restore-check'))}
    for key, item in value['signals'].items():
        if key not in keys or not isinstance(item, dict):
            return False
        required = {'firing', 'told', 'told_at', 'fired_at', 'since', 'elapsed', 'count',
                    'attempts', 'checked_at', 'previous', 'cleared_at', 'facts'}
        if not required <= set(item) or set(item)-required-{'failure_at'}:
            return False
        if any(type(item.get(field)) is not bool for field in ('firing', 'told')):
            return False
        if any(not number(item.get(field)) for field in ('elapsed', 'count', 'checked_at')):
            return False
        if any(item.get(field) is not None and not number(item[field])
               for field in ('told_at', 'fired_at', 'since', 'failure_at', 'cleared_at')):
            return False
        if item['told'] and item.get('told_at') is None:
            return False
        if item['firing'] and item.get('fired_at') is None:
            return False
        if item.get('previous') not in (None, 'bad', 'good', 'hold'):
            return False
        if not isinstance(item.get('facts'), str) or len(item['facts']) > 1024:
            return False
        if not isinstance(item.get('attempts'), list) or len(item['attempts']) > 3:
            return False
        if not all(number(attempt) for attempt in item['attempts']):
            return False
    for key, item in value['events'].items():
        if key not in ('release_live', 'corpus_live') or not isinstance(item, dict):
            return False
        if set(item) != {'observed', 'told'}:
            return False
        for identity in item.values():
            if key == 'corpus_live':
                if not isinstance(identity, str) or not 0 < len(identity) <= 256:
                    return False
            elif (not isinstance(identity, list) or len(identity) != 2 or
                  not isinstance(identity[0], str) or not 0 < len(identity[0]) <= 256 or
                  type(identity[1]) is not int or identity[1] < 1):
                return False
    return True


def load_state(path):
    try:
        value = read(path)
        if valid_state(value):
            return value
    except (OSError, ValueError, TypeError, RecursionError, OverflowError):
        pass
    return fresh()


def result(directory, success, now):
    path = Path(directory) / 'result.json'
    previous = None
    try:
        value = read(path)
        previous = (value or {}).get('last_success_at')
        timestamp(previous)
    except (OSError, ValueError, TypeError, RecursionError, OverflowError):
        previous = None
    atomic(path, dict(status='ok' if success else 'failed',
                      last_success_at=utc(now) if success else previous,
                      message='watch completed' if success else 'watch failed'), 0o640)


def log(path, value):
    descriptor = os.open(path, os.O_WRONLY | os.O_APPEND | os.O_CREAT | os.O_NOFOLLOW, 0o640)
    with os.fdopen(descriptor, 'w') as output:
        output.write(json.dumps(value, separators=(',', ':'))+'\n')
