import uuid

from .deploy_corpus_kind import LAW


COMPLETE = frozenset(('activated', 'reverted', 'aborted', 'pruned'))


class Activation:
    def __init__(self, database, kind=LAW):
        self.db, self.kind = database, kind
        self.steps = kind.steps(database)

    def choose(self, manifest):
        name, country = manifest['database'], self.kind.country(manifest)
        active = self.db.current(country)
        if active is None and not self.kind.bootstrap:
            raise ValueError('active corpus pointer missing')
        retired = self.db.retirement(name)
        if retired is not None:
            if not retired['dump_sha256'] or retired['dump_sha256'] != manifest['dump']['sha256']:
                raise ValueError('retained database dump digest differs or is unknown')
            if active is None or not retired['live'] or not self.db.exists(name) or name == active:
                raise ValueError('retained database unavailable or retention expired')
            return 'retained', active
        if active != self.kind.previous(manifest):
            raise ValueError('new ' + self.kind.label + ' previous does not equal active release')
        if self.db.exists(name):
            raise ValueError('new ' + self.kind.label + ' target already exists')
        return 'new', active

    def begin(self, hash_value, manifest, mode, expected):
        row = dict(operation_id=uuid.uuid4().hex, country=self.kind.country(manifest), rel=manifest['rel'],
                   mode=mode, database_name=manifest['database'], manifest_hash=hash_value,
                   dump_sha256=manifest['dump']['sha256'], expected_active_db=expected,
                   manifest=manifest)
        return self.db.append(row, 'started')

    def abort(self, row):
        latest = next((r for r in self.db.operations() if r['operation_id'] == row['operation_id']), row)
        if latest['step'] in ('activated', 'reverted'):
            return
        self.db.append(row, 'aborted')
        name = row['database_name']
        if (row['mode'] == 'new' and not self.db.protected(name) and self.db.exists(name)
                and not self.db.connections(name)):
            self.db.drop(name)

    def guarded(self, row):
        name = row['database_name']
        active = self.db.current(row['country'])
        retired = self.db.retirement(name)
        if active != row['expected_active_db'] or active == name:
            return False
        if active is None and (not self.kind.bootstrap or row['mode'] != 'new'):
            return False
        if row['mode'] == 'new':
            return retired is None
        return (retired is not None and retired['live'] and retired['dump_sha256'] is not None
                and retired['dump_sha256'] == row['dump_sha256'] and self.db.exists(name))

    def run(self, hash_value, manifest, dump, image):
        rows = [r for r in self.db.operations() if r['manifest_hash'] == hash_value and r['mode'] != 'prune']
        if rows:
            return self.resume(rows[-1], dump, image)
        mode, expected = self.choose(manifest)
        return self.resume(self.begin(hash_value, manifest, mode, expected), dump, image)

    def resume(self, row, dump, image, recheck=False):
        if row['step'] in ('activated', 'reverted'):
            return row
        if row['step'] == 'aborted':
            self.abort_cleanup(row)
            raise ValueError(self.kind.label + ' activation was aborted')
        if not self.guarded(row):
            self.abort(row)
            return dict(row, step='aborted')
        try:
            if recheck and row['step'] == 'checked':
                self.steps.validate(row)
                self.steps.smoke(row, image)
            if row['mode'] == 'new':
                if row['step'] == 'started':
                    self.steps.inspect(row, dump, image)
                    row = self.db.append(row, 'creating')
                if row['step'] == 'creating':
                    self.steps.inspect(row, dump, image)
                    self.steps.create(row)
                    row = self.db.append(row, 'created')
                if row['step'] == 'created':
                    row = self.db.append(row, 'restoring')
                if row['step'] == 'restoring':
                    self.steps.restore(row, dump, image)
                    row = self.db.append(row, 'restored')
            elif row['step'] == 'started':
                row = self.db.append(row, 'restored')
            if row['step'] == 'restored':
                self.steps.validate(row)
                row = self.db.append(row, 'validated')
            if row['step'] == 'validated':
                self.steps.smoke(row, image)
                row = self.db.append(row, 'checked')
            if row['step'] == 'checked':
                if not self.guarded(row):
                    raise ValueError(self.kind.label + ' activation guard changed')
                row = self.db.activate(row)
            if row['step'] != 'activated':
                raise ValueError('unknown ' + self.kind.label + ' activation step')
            return row
        except Exception:
            self.abort(row)
            raise

    def abort_cleanup(self, row):
        name = row['database_name']
        if (row['mode'] == 'new' and not self.db.protected(name) and self.db.exists(name)
                and not self.db.connections(name)):
            self.db.drop(name)

    def prune_one(self, row):
        name = row['database_name']
        if self.db.protected(name) or self.db.connections(name):
            return
        if self.db.exists(name):
            self.db.drop(name)
        self.db.forget(name)
        self.db.append(row, 'pruned')

    def prune(self):
        for row in self.db.operations():
            if row['step'] == 'pruning':
                self.prune_one(row)
            elif row['step'] == 'aborted':
                self.abort_cleanup(row)
        open_prunes = {r['database_name'] for r in self.db.operations() if r['step'] == 'pruning'}
        for name in self.db.expired():
            if name in open_prunes or self.db.protected(name) or self.db.connections(name):
                continue
            country, rel = self.kind.parse(name)
            row = dict(operation_id=uuid.uuid4().hex, country=country, rel=rel, mode='prune',
                       database_name=name, manifest_hash=None, dump_sha256=None,
                       expected_active_db=None, manifest=None)
            self.prune_one(self.db.append(row, 'pruning'))
