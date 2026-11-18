import hashlib
import uuid

from .deploy_activation_reconcile import OWNER
from .deploy_activation_record import RETENTION_SECONDS, moment
from .deploy_corpus_sql import JournalRefused
from .deploy_io import CommandError, interrupted
from .deploy_release_target import Held, Waiting


ORDER = dict(new=('started', 'creating', 'created', 'restoring', 'restored', 'validated', 'checked'),
             retained=('started', 'restored', 'validated', 'checked'))
POINTER_CHANGED = 'pointer changed'
ADOPTION_FAILED = 'adoption check failed'


class Activation:
    def __init__(self, database, kind, records, spend=lambda digest: None):
        self.db, self.kind, self.records, self.spend = database, kind, records, spend
        self.tables = database.tables
        self.steps = kind.steps(database)
        database.guard = records
        database.remember = self.remember

    @property
    def record(self):
        return self.records.own

    def remember(self, operation, oid):
        current = self.record.operation
        if current is not None and current['id'] == operation:
            self.record.set_oid(oid)

    def row(self, operation, manifest):
        return dict(operation_id=operation['id'], country=self.kind.country(manifest), rel=manifest['rel'],
                    mode=operation['mode'], database_name=operation['database'],
                    manifest_hash=operation['manifest_hash'], dump_sha256=operation['dump_sha256'],
                    expected_active_db=operation['expected'], manifest=manifest, step=operation['step'],
                    oid=operation['oid'])

    def choose(self, manifest, pointer, catalog):
        name, label = manifest['database'], self.kind.label
        record, entry = self.record, self.record.entry(name)
        info = catalog.get(name)
        fresh = entry is not None and self.records.clock() - moment(entry['retired_at']) < RETENTION_SECONDS
        if (fresh and entry['dump_sha256'] == manifest['dump']['sha256'] and info is not None
                and info['oid'] == entry['oid'] and info['owner'] == OWNER):
            if pointer is None:
                raise Waiting(label + ' pointer missing')
            return 'retained', entry['oid']
        previous, active = self.kind.previous(manifest), record.active
        if pointer != previous or (active is not None and active['database'] != previous):
            record.raise_alert('pointer_changed')
            raise Waiting('new ' + label + ' previous does not equal active release')
        if entry is not None or (active is not None and active['database'] == name):
            raise Waiting('new ' + label + ' target is the active or a retired database')
        if info is not None:
            if info['owner'] != OWNER:
                raise ValueError('new ' + label + ' target already exists')
            if self.db.connections(name):
                raise Waiting('new ' + label + ' target has connections')
        return 'new', None

    def operation(self, digest, manifest, mode, expected, oid, step):
        return dict(id=uuid.uuid4().hex, mode=mode, database=manifest['database'], manifest_hash=digest,
                    dump_sha256=manifest['dump']['sha256'], expected=expected, oid=oid, step=step)

    def begin(self, digest, manifest, mode, expected, oid=None):
        operation = self.record.begin(self.operation(digest, manifest, mode, expected, oid, 'started'))
        self.tables.append(self.row(operation, manifest), 'started')
        return operation

    def advance(self, manifest, step):
        self.db.check_lock()
        self.record.step(step)
        self.tables.append(self.row(self.record.operation, manifest), step)

    def catch_up(self, manifest):
        operation = self.record.operation
        wanted = 'checked' if operation['step'] == 'swapping' else operation['step']
        order = ORDER[operation['mode']]
        if wanted not in order:
            return
        end = order.index(wanted)
        latest = next((row['step'] for row in self.tables.operations(operation['id'])
                       if row['operation_id'] == operation['id']), None)
        if latest is None:
            start = 0
        elif latest in order:
            start = order.index(latest) + 1
        else:
            start = end
        row = self.row(operation, manifest)
        for step in order[start:end + 1]:
            self.tables.append(row, step)

    def attempt(self, manifest, action, *args):
        try:
            return action(self.row(self.record.operation, manifest), *args)
        except (Waiting, JournalRefused):
            raise
        except Exception as error:
            if interrupted(error):
                raise
            self.abort(manifest)
            raise

    def resume(self, manifest, dump, image, recheck=False):
        self.catch_up(manifest)
        operation, steps = self.record.operation, self.steps
        if recheck and operation['step'] == 'checked':
            self.attempt(manifest, steps.validate)
            self.attempt(manifest, steps.smoke, image)
        if operation['mode'] == 'new':
            if operation['step'] == 'started':
                self.attempt(manifest, steps.inspect, dump, image)
                self.advance(manifest, 'creating')
            if operation['step'] == 'creating':
                self.attempt(manifest, steps.inspect, dump, image)
                self.attempt(manifest, steps.create)
                self.advance(manifest, 'created')
            if operation['step'] == 'created':
                self.advance(manifest, 'restoring')
            if operation['step'] == 'restoring':
                self.attempt(manifest, steps.restore, dump, image)
                self.advance(manifest, 'restored')
        elif operation['step'] == 'started':
            self.advance(manifest, 'restored')
        if operation['step'] == 'restored':
            self.attempt(manifest, steps.validate)
            self.advance(manifest, 'validated')
        if operation['step'] == 'validated':
            self.attempt(manifest, steps.smoke, image)
            self.advance(manifest, 'checked')
        if operation['step'] != 'checked':
            raise ValueError('unknown ' + self.kind.label + ' activation step')
        if self.tables.pointer() != operation['expected']:
            self.record.raise_alert('pointer_changed')
            raise Held(POINTER_CHANGED)
        self.attempt(manifest, self.db.verify_target)
        self.record.step('swapping')
        self.finish(manifest)

    def finish(self, manifest):
        operation, active = self.record.operation, self.record.active
        target = operation['database']
        retire = None
        if active is not None:
            if active['database'] != target:
                retire = (active['database'], active['dump_sha256'])
        elif operation['expected'] not in (None, target):
            retire = (operation['expected'], None)
        row = self.row(operation, manifest)
        self.db.check_lock()
        try:
            statement = self.kind.activation(self.db, row, retire)
        except (ValueError, CommandError) as error:
            if interrupted(error):
                raise
            raise Waiting(self.kind.label + ' swap not ready') from error
        self.tables.swap(statement, row)
        self.record.activate()

    def known_oid(self, operation, name):
        if operation is not None:
            return operation['oid']
        active, entry = self.record.active, self.record.entry(name)
        if active is not None and active['database'] == name:
            return active['oid']
        return entry['oid'] if entry else None

    def refuse_adoption(self, cause=None):
        self.record.raise_alert('adoption_failed')
        raise Held(ADOPTION_FAILED) from cause

    def adopt(self, digest, manifest, image, pointer, catalog):
        operation = self.record.operation
        name = manifest['database']
        info, known = catalog.get(name), self.known_oid(operation, name)
        if info is None or info['owner'] != OWNER or (known is not None and info['oid'] != known):
            self.refuse_adoption()
        candidate = operation or self.operation(digest, manifest, 'retained', pointer, info['oid'], 'checked')
        row = dict(self.row(candidate, manifest), mode='retained', oid=info['oid'])
        try:
            self.steps.validate(row)
            self.steps.smoke(row, image)
        except Waiting:
            raise
        except Exception as error:
            if interrupted(error):
                raise
            self.refuse_adoption(error)
        if operation is None:
            self.record.begin(candidate)
        else:
            self.record.adopt(info['oid'])
        self.record.step('swapping')
        self.catch_up(manifest)
        self.finish(manifest)

    def abort(self, manifest):
        operation = self.record.operation
        self.record.step('aborted')
        row = self.row(operation, manifest)
        self.tables.append(row, 'aborted')
        name = row['database_name']
        if (row['mode'] == 'new' and self.db.exists(name) and self.db.owned(name)
                and self.db.droppable(name, row['operation_id'])):
            self.db.drop_database(name, row['oid'])
        self.db.remove_list(row['operation_id'])
        self.spend(operation['manifest_hash'])
        self.record.clear_operation()
        self.db.tidy()

    def prune_row(self, entry):
        name = entry['database']
        country, rel = self.kind.parse(name)
        identity = hashlib.sha256(('prune:' + name + ':' + str(entry['oid'])).encode()).hexdigest()[:32]
        return dict(operation_id=identity, country=country, rel=rel, mode='prune', database_name=name,
                    manifest_hash=None, dump_sha256=None, expected_active_db=None, manifest=None)

    def prune(self, catalog):
        now = self.records.clock()
        self.db.check_lock()
        for entry in list(self.record.retired):
            if now - moment(entry['retired_at']) < RETENTION_SECONDS:
                continue
            name = entry['database']
            info = catalog.get(name)
            if info is not None and (info['oid'] != entry['oid'] or info['owner'] != OWNER):
                self.record.forget(name)
                continue
            if info is not None and not self.db.droppable(name, pruning={name}):
                continue
            row = self.prune_row(entry)
            self.tables.append(row, 'pruning')
            if info is not None:
                self.db.drop_database(name, entry['oid'])
            self.tables.forget(name)
            self.tables.append(row, 'pruned')
            self.record.forget(name)
