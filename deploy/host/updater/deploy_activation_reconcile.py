import hashlib
import json
import time

from .deploy_activation_record import RETENTION_SECONDS, Record, blank, moment
from .deploy_corpus_kind import FRAMEWORKS, LAW


OWNER = 'activator'


def record_path(directory, kind):
    return directory / (kind.key + '-activation.json')


def known_oid(record, name):
    data = record.data
    if data['active'] and data['active']['database'] == name:
        return data['active']['oid']
    operation = data['operation']
    if operation and operation['database'] == name and operation['oid'] is not None:
        return operation['oid']
    entry = record.entry(name)
    return entry['oid'] if entry else None


def evaluate(record, pointer, catalog):
    active, operation = record.active, record.operation
    explained = (operation is not None and operation['step'] == 'swapping'
                 and pointer in (operation['database'], operation['expected']))
    if active is not None and pointer != active['database'] and not explained:
        record.alert('pointer_changed')
    if pointer is None:
        return
    info, known = catalog.get(pointer), known_oid(record, pointer)
    if info is None or (known is not None and info['oid'] != known):
        record.alert('serving_database_gone')


class Records:
    def __init__(self, directory, kind, state, clock=time.time, note=lambda message: None):
        self.directory, self.kind, self.state, self.clock, self.note = directory, kind, state, clock, note
        self.other_kind = FRAMEWORKS if kind is LAW else LAW
        self.own = Record(record_path(directory, kind), kind.sql.database, clock)
        self.other = Record(record_path(directory, self.other_kind), self.other_kind.sql.database, clock)

    def statuses(self, kind):
        items = self.state.records(kind.key).items()
        return [record for _, record in sorted(items, key=lambda item: int(item[0]))]

    def active_hash(self, kind):
        found = [record['hash'] for record in self.statuses(kind) if record['status'] == 'active']
        return found[-1] if found else None

    def pending(self, kind, digest):
        return any(record['hash'] == digest and record['status'] == 'pending' for record in self.statuses(kind))

    def manifest(self, kind, digest):
        try:
            raw = (self.directory / kind.artifacts / digest / 'manifest.json').read_bytes()
        except OSError:
            return None
        if hashlib.sha256(raw).hexdigest() != digest:
            return None
        try:
            value = json.loads(raw)
        except (ValueError, RecursionError):
            return None
        return value if isinstance(value, dict) else None

    def described(self, kind, digest, catalog):
        manifest = self.manifest(kind, digest)
        if manifest is None:
            return None
        name, dump = manifest.get('database'), manifest.get('dump')
        sha256 = dump.get('sha256') if isinstance(dump, dict) else None
        try:
            kind.sql.database(name)
        except ValueError:
            return None
        info = catalog.get(name)
        if info is None or info['owner'] != OWNER or not isinstance(sha256, str):
            return None
        return dict(database=name, oid=info['oid'], dump_sha256=sha256)

    def active_entry(self, kind, digest, catalog):
        found = self.described(kind, digest, catalog) if digest else None
        return dict(found, manifest_hash=digest) if found else None

    def rebuilt(self, kind, record, catalog):
        record.data = blank()
        record.data['active'] = self.active_entry(kind, self.active_hash(kind), catalog)
        for status in self.statuses(kind):
            if status['status'] != 'retained':
                continue
            found = self.described(kind, status['hash'], catalog)
            if found:
                record.retire(found['database'], found['oid'], found['dump_sha256'])
        if record.active:
            record.data['retired'] = [item for item in record.retired
                                      if item['database'] != record.active['database']]

    def recover_file(self, kind, record, catalog):
        status = record.load()
        if status == 'ok':
            return
        self.rebuilt(kind, record, catalog)
        if status == 'unreadable':
            record.alert('record_rebuilt')
            self.note('unreadable ' + kind.label + ' activation record rebuilt')
        record.save()

    def reconcile(self, catalog):
        record, kind = self.own, self.kind
        self.recover_file(kind, record, catalog)
        self.follow_state(record, kind, catalog)
        operation = record.operation
        if operation is not None and not self.pending(kind, operation['manifest_hash']):
            record.data['operation'] = None
        self.prune_entries(record, catalog)
        self.check_active(record, catalog)
        record.save()

    def follow_state(self, record, kind, catalog):
        active, operation = record.active, record.operation
        named = self.active_hash(kind)
        if active is None or named is None or named == active['manifest_hash']:
            return
        if operation is not None and operation['step'] == 'activated':
            return
        info = catalog.get(active['database'])
        if info is not None and info['oid'] == active['oid']:
            record.retire(active['database'], active['oid'], active['dump_sha256'])
        record.data['active'] = self.active_entry(kind, named, catalog)

    def prune_entries(self, record, catalog):
        kept, now = [], self.clock()
        active = record.active['database'] if record.active else None
        for entry in record.retired:
            if entry['database'] == active:
                continue
            info = catalog.get(entry['database'])
            young = now - moment(entry['retired_at']) < RETENTION_SECONDS
            if young and (info is None or info['oid'] != entry['oid'] or info['owner'] != OWNER):
                continue
            kept.append(entry)
        record.data['retired'] = kept

    def check_active(self, record, catalog):
        active = record.active
        if active is None:
            return
        info = catalog.get(active['database'])
        if info is not None and info['oid'] == active['oid'] and info['owner'] == OWNER:
            return
        record.data['active'] = None
        record.alert('active_cleared')
        if info is None or info['oid'] != active['oid']:
            record.alert('serving_database_gone')

    def observe_pointer(self, pointer, catalog):
        evaluate(self.own, pointer, catalog)
        self.own.save()

    def other_data(self, catalog):
        status = self.other.load()
        if status == 'missing':
            return blank()
        if status == 'unreadable':
            self.recover_file(self.other_kind, self.other, catalog)
        return self.other.data

    def names(self, catalog, own=None, pruning=()):
        names = set()
        for data, mine in ((self.own.data or blank(), True), (self.other_data(catalog), False)):
            if data['active']:
                names.add(data['active']['database'])
            names.update(entry['database'] for entry in data['retired'] if entry['database'] not in pruning)
            operation = data['operation']
            if operation and not (mine and operation['id'] == own):
                names.add(operation['database'])
        return names
