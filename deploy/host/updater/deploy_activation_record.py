from datetime import datetime, timezone
import json
from pathlib import Path
import re
import time

from .deploy_io import atomic, canonical


VERSION = 1
STEPS = ('started', 'creating', 'created', 'restoring', 'restored', 'validated', 'checked', 'swapping',
         'activated', 'aborted')
BEFORE_SWAPPING = STEPS[:STEPS.index('swapping')]
MODES = ('new', 'retained')
ALERTS = ('pointer_changed', 'serving_database_gone', 'active_cleared', 'adoption_failed', 'record_rebuilt',
          'journal_row_refused', 'lock_unavailable')
RETENTION_SECONDS = 7 * 86400
ALERT_SECONDS = 86400
TOP = {'version', 'active', 'retired', 'operation', 'alerts'}
ACTIVE = {'database', 'oid', 'manifest_hash', 'dump_sha256'}
RETIRED = {'database', 'oid', 'dump_sha256', 'retired_at'}
OPERATION = {'id', 'mode', 'database', 'manifest_hash', 'dump_sha256', 'expected', 'oid', 'step'}
TIME = re.compile(r'[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z')
FORMAT = '%Y-%m-%dT%H:%M:%SZ'


class Unreadable(ValueError):
    pass


def stamp(now):
    return datetime.fromtimestamp(now, timezone.utc).strftime(FORMAT)


def moment(text):
    return datetime.strptime(text, FORMAT).replace(tzinfo=timezone.utc).timestamp()


def require(condition):
    if not condition:
        raise Unreadable('invalid activation record')


def hexadecimal(value, size):
    require(isinstance(value, str) and re.fullmatch('[0-9a-f]{' + str(size) + '}', value) is not None)


def positive(value):
    require(type(value) is int and value > 0)


def named(value, check):
    require(isinstance(value, str))
    try:
        check(value)
    except ValueError:
        raise Unreadable('invalid activation record') from None


def timestamp(value):
    require(isinstance(value, str) and TIME.fullmatch(value) is not None)
    try:
        moment(value)
    except ValueError:
        raise Unreadable('invalid activation record') from None


def section(value, keys):
    require(isinstance(value, dict) and set(value) == keys)


def validate(value, check):
    section(value, TOP)
    require(type(value['version']) is int and value['version'] == VERSION)
    active = value['active']
    if active is not None:
        section(active, ACTIVE)
        named(active['database'], check)
        positive(active['oid'])
        hexadecimal(active['manifest_hash'], 64)
        hexadecimal(active['dump_sha256'], 64)
    require(isinstance(value['retired'], list))
    seen = set()
    for entry in value['retired']:
        section(entry, RETIRED)
        named(entry['database'], check)
        require(entry['database'] not in seen)
        seen.add(entry['database'])
        positive(entry['oid'])
        hexadecimal(entry['dump_sha256'], 64)
        timestamp(entry['retired_at'])
    operation = value['operation']
    if operation is not None:
        section(operation, OPERATION)
        hexadecimal(operation['id'], 32)
        require(operation['mode'] in MODES and operation['step'] in STEPS)
        named(operation['database'], check)
        hexadecimal(operation['manifest_hash'], 64)
        hexadecimal(operation['dump_sha256'], 64)
        if operation['expected'] is not None:
            named(operation['expected'], check)
        if operation['oid'] is not None:
            positive(operation['oid'])
    require(isinstance(value['alerts'], dict))
    for code, seen_at in value['alerts'].items():
        require(code in ALERTS)
        timestamp(seen_at)


def unique(pairs):
    value = {}
    for key, item in pairs:
        require(key not in value)
        value[key] = item
    return value


def parse(raw, check):
    try:
        value = json.loads(raw, object_pairs_hook=unique)
    except (ValueError, RecursionError):
        raise Unreadable('invalid activation record') from None
    validate(value, check)
    return value


def blank():
    return dict(version=VERSION, active=None, retired=[], operation=None, alerts={})


class Record:
    def __init__(self, path, check, clock=time.time):
        self.path, self.check, self.clock = Path(path), check, clock
        self.data, self.written = None, None

    def load(self):
        self.data, self.written = None, None
        try:
            raw = self.path.read_bytes()
        except FileNotFoundError:
            return 'missing'
        try:
            self.data = parse(raw, self.check)
        except Unreadable:
            return 'unreadable'
        self.written = raw
        return 'ok'

    def recent(self, alerts):
        now = self.clock()
        return {code: seen for code, seen in alerts.items() if now - moment(seen) < ALERT_SECONDS}

    def save(self):
        self.data['alerts'] = self.recent(self.data['alerts'])
        validate(self.data, self.check)
        raw = canonical(self.data) + b'\n'
        if raw != self.written:
            atomic(self.path, raw)
            self.written = raw

    def integrity(self):
        data = self.data
        if data is None:
            try:
                data = parse(self.path.read_bytes(), self.check)
            except (OSError, Unreadable):
                return []
        return sorted(self.recent(data['alerts']))

    def alert(self, code):
        if code not in ALERTS:
            raise ValueError('unknown activation alert')
        self.data['alerts'][code] = stamp(self.clock())

    def raise_alert(self, code):
        self.alert(code)
        self.save()

    @property
    def active(self):
        return self.data['active']

    @property
    def retired(self):
        return self.data['retired']

    @property
    def operation(self):
        return self.data['operation']

    def entry(self, name):
        return next((item for item in self.data['retired'] if item['database'] == name), None)

    def retire(self, database, oid, digest):
        self.data['retired'] = [item for item in self.data['retired'] if item['database'] != database]
        self.data['retired'].append(dict(database=database, oid=oid, dump_sha256=digest,
                                         retired_at=stamp(self.clock())))

    def forget(self, name):
        self.data['retired'] = [item for item in self.data['retired'] if item['database'] != name]
        self.save()

    def begin(self, operation):
        if self.data['operation'] is not None:
            raise ValueError('activation operation already open')
        self.data['operation'] = operation
        self.save()
        return operation

    def step(self, step):
        if step not in STEPS:
            raise ValueError('unknown activation step')
        self.data['operation']['step'] = step
        self.save()

    def set_oid(self, oid):
        self.data['operation']['oid'] = oid
        self.save()

    def adopt(self, oid):
        self.data['operation'].update(mode='retained', oid=oid)
        self.save()

    def activate(self):
        operation, previous = self.data['operation'], self.data['active']
        target = operation['database']
        self.data['retired'] = [item for item in self.data['retired'] if item['database'] != target]
        if previous is not None and previous['database'] != target:
            self.retire(previous['database'], previous['oid'], previous['dump_sha256'])
        self.data['active'] = dict(database=target, oid=operation['oid'], manifest_hash=operation['manifest_hash'],
                                   dump_sha256=operation['dump_sha256'])
        operation['step'] = 'activated'
        self.save()

    def clear_operation(self):
        self.data['operation'] = None
        self.save()
