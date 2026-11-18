import copy
from contextlib import nullcontext
import hashlib
import json
import re
from unittest import mock

from updater import activation_test as fixture
from updater.corpus_verify_test import description as law_description, manifest as law_manifest
from updater.deploy_activation_reconcile import record_path
from updater.deploy_activation_record import Record, blank
from updater.deploy_corpus import Corpus
from updater.deploy_corpus_database import Database, NoRecords
from updater.deploy_corpus_kind import FRAMEWORKS, LAW
from updater.deploy_corpus_sql import JournalRefused
from updater.deploy_io import canonical
from updater.deploy_release_tables import PointerRefused, TableFailure
from updater.deploy_release_target import DATABASE_LEFT, Leftover, Waiting
from updater.frameworks_verify_test import BLOCK, description as frameworks_description, manifest as frameworks_manifest


NOW = 1800000000
CLOSED = ('activated', 'reverted', 'aborted', 'pruned')
RETIRE = re.compile(r"INSERT INTO public\.retired_release .*?VALUES \('vn','([^']+)',now\(\),now\(\)\+interval "
                    r"'7 days',NULL,(?:'([0-9a-f]{64})'|NULL)", re.S)


class Crash(BaseException):
    pass


class World:
    protected_names = Database.protected_names
    droppable = Database.droppable
    drop_target = Database.drop_target
    target = Database.target

    def __init__(self, kind):
        self.kind, self.tables, self.sql = kind, self, kind.sql
        self.guard, self.remember = NoRecords(), lambda operation, oid: None
        self.pointer_value, self.pointer_bad, self.unavailable = None, False, False
        self.databases, self.next_oid = {}, 1000
        self.rows, self.table_retired, self.foreign_protected = [], {}, set()
        self.connected, self.dropped, self.sweeps, self.log = set(), [], [], []
        self.created, self.tidied, self.lists_discarded = 0, 0, 0
        self.lists_removed, self.failures, self.crashes = [], {}, set()
        self.superuser, self.leftover, self.tamper, self.swap_error = False, None, False, None
        self.swaps, self.locks, self.lock_error, self.before_drop = [], 0, None, None
        self.describe = mock.Mock()
        self.check_extensions = mock.Mock()

    def check_lock(self):
        self.locks += 0

    def crash(self, point):
        if point in self.crashes:
            self.crashes.discard(point)
            raise Crash(point)

    def table(self):
        if self.unavailable:
            raise TableFailure('auth tables unavailable')

    def add(self, name, oid=None, owner='activator'):
        if oid is None:
            self.next_oid += 1
            oid = self.next_oid
        self.databases[name] = dict(oid=oid, owner=owner)
        return oid

    def lock(self):
        self.locks += 1
        if self.lock_error is not None:
            raise self.lock_error
        return nullcontext()

    def discard_lists(self):
        self.lists_discarded += 1

    def remove_list(self, operation):
        self.lists_removed.append(operation)

    def catalog(self):
        return copy.deepcopy(self.databases)

    def exists(self, name):
        return name in self.databases

    def owned(self, name):
        return self.databases.get(name, {}).get('owner') == 'activator'

    def connections(self, name):
        return name in self.connected

    def other_superuser(self):
        return self.superuser

    def clear_leftovers(self):
        self.sweeps.append(len(self.rows))
        if self.leftover:
            raise Leftover(self.leftover)
        protected = self.protected_names()
        for name in [n for n, info in self.databases.items() if info['owner'] == 'activator']:
            if name not in protected:
                self.drop_database(name)

    def tidy(self):
        self.tidied += 1
        try:
            self.clear_leftovers()
        except Waiting:
            pass

    def drop_database(self, name, oid=None):
        self.crash('drop:' + name)
        if self.before_drop is not None:
            self.before_drop(name)
        info = self.databases.get(name)
        if info is not None and info['owner'] == 'activator' and (oid is None or info['oid'] == oid):
            del self.databases[name]
            self.dropped.append(name)
        if name in self.databases:
            raise Waiting(DATABASE_LEFT)

    def step(self, name, row):
        self.log.append(name)
        self.crash(name)
        if name in self.failures:
            raise self.failures[name]

    def inspect(self, row, dump=None, image=None):
        self.step('inspect', row)

    def create(self, row):
        self.step('create', row)
        name = row['database_name']
        if name in self.databases and self.databases[name]['owner'] != 'activator':
            raise ValueError('activation target not owned by activator')
        self.clear_leftovers()
        if name in self.databases:
            self.drop_target(row)
        row['oid'] = self.add(name)
        self.created += 1
        self.remember(row['operation_id'], row['oid'])

    def restore(self, row, dump=None, image=None):
        self.step('restore', row)

    def validate(self, row):
        self.step('validate', row)

    def smoke(self, row, image):
        self.step('smoke', row)

    def verify_target(self, row):
        if self.databases.get(row['database_name'], {}).get('oid') != row.get('oid'):
            raise ValueError('activation target database identity changed')

    def projection(self, row):
        return 0

    def pointer(self):
        self.table()
        if self.pointer_bad:
            raise PointerRefused('active pointer is not a release database')
        return self.pointer_value

    def latest(self):
        found = {}
        for row in self.rows:
            found[row['operation_id']] = row
        return found

    def protected(self, operation=None):
        self.table()
        names = {self.pointer_value} if self.pointer_value else set()
        names |= {name for name, row in self.table_retired.items() if row['live']}
        names |= set(self.foreign_protected)
        names |= {row['database_name'] for row in self.latest().values()
                  if row['mode'] != 'prune' and row['step'] not in CLOSED and row['operation_id'] != operation}
        return names

    def operations(self, own=None):
        self.table()
        rows = [row for row in self.latest().values() if row['step'] not in CLOSED or row['operation_id'] == own]
        return [self.kind.sql.journal_row(row) for row in rows]

    def append(self, row, step):
        self.table()
        self.crash('append:' + step)
        if self.tamper:
            raise JournalRefused('invalid release journal row')
        value = dict(row, step=step)
        self.rows.append(value)
        return value

    def close_open(self, keep=None):
        self.table()
        for row in list(self.latest().values()):
            if row['mode'] != 'prune' and row['step'] not in CLOSED and row['operation_id'] != keep:
                self.rows.append(dict(row, step='aborted'))

    def swap(self, statement, row):
        self.table()
        self.crash('swap:before')
        if self.swap_error:
            raise self.swap_error
        target, expected = row['database_name'], row['expected_active_db']
        if self.pointer_value not in (target, expected):
            raise TableFailure('active pointer changed')
        self.pointer_value = target
        retired = RETIRE.search(statement)
        if retired:
            self.table_retired[retired[1]] = dict(dump_sha256=retired[2], live=True)
        if row['mode'] == 'retained':
            self.table_retired.pop(target, None)
        self.swaps.append(dict(target=target, retired=retired[1] if retired else None,
                               digest=retired[2] if retired else None))
        value = dict(row, step='activated')
        self.rows.append(value)
        self.crash('swap:after')
        return value

    def forget(self, name):
        self.table()
        self.crash('forget:' + name)
        self.table_retired.pop(name, None)


class PhaseCase(fixture.ActivationFixture):
    kind = LAW

    def setUp(self):
        super().setUp()
        self.now = NOW
        self.m = self.new_manifest()
        self.name = self.m['database']
        self.active_name = self.prefix + self.m['previous']
        self.directory = self.paths.state / (self.kind.key + '-fixture')
        self.directory.mkdir()
        self.write_manifest()
        self.db = World(self.kind)
        self.db.describe.return_value = self.description(self.m)
        self.registry = mock.Mock()
        self.registry.fetch.return_value = self.directory
        self.prepare = mock.Mock(return_value=True)
        self.phase = self.make_phase()

    def make_phase(self):
        phase = Corpus(self.state, self.store, self.host, self.rules, self.registry, database=self.db,
                       prepare=self.prepare, clock=lambda: self.now, kind=self.kind)
        phase.resources = mock.Mock()
        phase.engine.steps = self.db
        self.dumps = []
        def download():
            path = self.paths.root / 'dump' / self.kind.dump
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(b'synthetic dump')
            self.dumps.append(path)
            return path
        phase.download = mock.Mock(side_effect=download)
        phase.dump_path = lambda: self.paths.root / 'dump' / self.kind.dump
        return phase

    def retarget(self, rel, previous=None, sequence=None):
        self.m = json.loads(json.dumps(self.m).replace(self.m['rel'], rel))
        if previous is not None:
            self.m['previous'] = previous
        if sequence is not None:
            self.m['sequence'] = sequence
        self.name = self.m['database']
        self.write_manifest()

    def write_manifest(self):
        self.directory.joinpath('manifest.json').write_bytes(canonical(self.m))
        self.directory.joinpath('manifest.sigstore.json').write_bytes(b'{}')
        self.digest = hashlib.sha256(canonical(self.m)).hexdigest()

    @property
    def dump(self):
        return self.m['dump']['sha256']

    def records_of_kind(self):
        return self.state.records(self.kind.key)

    def stored_manifest(self, name, dump=None, salt='', kind=None):
        kind = kind or self.kind
        raw = canonical(dict(database=name, dump=dict(sha256=dump or self.dump), salt=salt))
        digest = hashlib.sha256(raw).hexdigest()
        directory = self.paths.state / kind.artifacts / digest
        directory.mkdir(parents=True, exist_ok=True)
        (directory / 'manifest.json').write_bytes(raw)
        return digest

    def bind(self, sequence, digest, status):
        section = (self.state.section()['accepted'] if self.kind is FRAMEWORKS else self.state.data['accepted']['corpus'])
        section[str(sequence)] = dict(hash=digest, status=status)
        self.state.save()

    def baseline(self, oid=100):
        self.active_hash = self.stored_manifest(self.active_name)
        self.bind(1, self.active_hash, 'active')
        self.db.add(self.active_name, oid)
        self.db.pointer_value = self.active_name
        return self.active_hash

    def accept(self):
        self.state.accept(self.kind.key, self.digest, self.m['sequence'])
        self.phase.hash = self.digest
        self.phase.remember(self.directory)

    @property
    def path(self):
        return record_path(self.paths.state, self.kind)

    def write_record(self, **values):
        record = Record(self.path, self.kind.sql.database, lambda: self.now)
        record.data = dict(blank(), **values)
        record.save()

    def stored(self):
        return json.loads(self.path.read_bytes())

    def active_entry(self, oid=100, name=None, digest=None):
        return dict(database=name or self.active_name, oid=oid, manifest_hash=digest or self.active_hash,
                    dump_sha256=self.dump)

    def operation(self, step, oid=None, mode='new', expected='active', name=None, digest=None):
        return dict(id='c' * 32, mode=mode, database=name or self.name, manifest_hash=digest or self.digest,
                    dump_sha256=self.dump, expected=self.active_name if expected == 'active' else expected,
                    oid=oid, step=step)

    def open_operation(self, step, oid=None, **changes):
        self.baseline()
        self.accept()
        op = self.operation(step, oid, **changes)
        self.write_record(active=self.active_entry(), operation=op)
        return op

    def run_phase(self, mode='activate'):
        return self.phase.run(self.paths.state / 'run', mode)

    def again(self, mode='activate'):
        self.phase = self.make_phase()
        return self.run_phase(mode)

    def result(self):
        return json.loads(self.paths.result.with_name(self.kind.result + '.json').read_bytes())

    def status(self):
        return self.state.records(self.kind.key)[str(self.m['sequence'])]['status']


class LawCase(PhaseCase):
    kind, prefix = LAW, 'corpus_vn_'

    def new_manifest(self):
        return law_manifest()

    def description(self, m):
        return law_description(m)


class FrameworksCase(PhaseCase):
    kind, prefix = FRAMEWORKS, 'frameworks_'

    def setUp(self):
        fixture.ActivationFixture.setUp(self)
        self.rules = dict(self.rules, frameworks=dict(BLOCK))
        self.state.data['release_floor'] = BLOCK['min_release_floor']
        self.state.save()
        calls = fixture.ActivationFixture.setUp
        fixture.ActivationFixture.setUp = lambda case: None
        try:
            PhaseCase.setUp(self)
        finally:
            fixture.ActivationFixture.setUp = calls

    def new_manifest(self):
        return frameworks_manifest()

    def description(self, m):
        return frameworks_description(m)


KINDS = (LawCase, FrameworksCase)
