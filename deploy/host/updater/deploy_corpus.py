from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import shutil
import time

from .deploy_activation_reconcile import Records
from .deploy_activation_record import BEFORE_SWAPPING
from .deploy_corpus_activate import Activation, Refused
from .deploy_corpus_database import Database
from .deploy_corpus_kind import LAW
from .deploy_corpus_result import ReleaseResult
from .deploy_corpus_sql import JournalRefused, REFUSED, SUPERUSER_REFUSED
from .deploy_io import atomic, canonical, CommandError, interrupted
from .deploy_release_tables import PointerRefused
from .deploy_release_target import Held, LockUnavailable, Waiting
from .deploy_verify import match


class ActivatorNotProvisioned(Exception):
    pass


class Corpus:
    def __init__(self, state, store, host, rules, registry, database=None, prepare=lambda: True, clock=time.time,
                 kind=LAW):
        self.state, self.store, self.host, self.rules, self.registry = state, store, host, rules, registry
        self.kind, self.label = kind, kind.label
        self.db = database or Database(host, rules, kind)
        self.tables = self.db.tables
        self.records = Records(host.paths.state, kind, state, clock, self.warn)
        self.record = self.records.own
        self.engine = Activation(self.db, kind, self.records, lambda digest: state.fail(digest, kind.key))
        self.prepare, self.clock = prepare, clock
        self.result = ReleaseResult(host.paths, clock, kind.result, self.record.integrity)
        self.hash, self.manifest, self.mode, self.step = None, None, None, kind.label
        self.artifacts = host.paths.state / kind.artifacts
        self.pointer, self.catalog = None, {}
        self.interrupted = False

    def warn(self, message):
        self.host.note('activation-record', message)

    def image(self):
        return self.store.load(self.state.data['active']).manifest['images']['server']

    def prepare_activation(self):
        if not self.prepare():
            raise ValueError('secret replacement pending before ' + self.label + ' activation')
        self.provision()

    def provision(self):
        if not self.host.provision_activator():
            raise ActivatorNotProvisioned()

    def remember(self, directory):
        target = self.artifacts / self.hash
        target.mkdir(parents=True, mode=0o700, exist_ok=True)
        for name in ('manifest.json', 'manifest.sigstore.json'):
            atomic(target / name, (directory / name).read_bytes())

    def load(self, operation):
        match('[0-9a-f]{64}', operation['manifest_hash'], 'recovery manifest hash')
        self.hash, self.manifest = self.kind.verify(self.artifacts / operation['manifest_hash'], self.rules, self.host.runner)
        if (self.hash != operation['manifest_hash'] or operation['database'] != self.manifest['database']
                or operation['dump_sha256'] != self.manifest['dump']['sha256']):
            raise ValueError(self.label + ' recovery manifest mismatch')
        binding = self.state.records(self.kind.key).get(str(self.manifest['sequence']))
        if not binding or binding['hash'] != self.hash:
            raise ValueError(self.label + ' recovery sequence binding missing')

    def resources(self):
        free = next((int(line.split()[1])*1024 for line in Path('/proc/meminfo').read_text().splitlines()
                     if line.startswith('MemAvailable:')), None)
        if free is None or free < 2500000000:
            raise ValueError(self.label + ' activation needs 2.5 GB free memory')
        disk = shutil.disk_usage(self.host.paths.root / 'srv/trachuan/data')[2]
        if disk < 10000000000:
            raise ValueError(self.label + ' activation needs 10 GB free disk')

    def dump_path(self):
        return self.host.paths.releases / self.kind.staging(self.manifest) / self.kind.dump

    def download(self):
        target = self.dump_path()
        target.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
        os.chmod(target.parent, 0o700)
        metadata = self.manifest['dump']
        if target.exists():
            digest = hashlib.sha256()
            with target.open('rb') as source:
                while chunk := source.read(1024*1024):
                    digest.update(chunk)
            if target.stat().st_size == metadata['size'] and digest.hexdigest() == metadata['sha256']:
                return target
            target.unlink()
        target.with_suffix('.partial').unlink(missing_ok=True)
        _, namespace, bucket = self.kind.settings(self.rules)
        self.host.runner(['python3', '-I', str(Path(__file__).with_name('deploy_corpus_download.py')),
            self.rules['region'], namespace, bucket, str(target)],
            input=canonical(metadata), timeout=3600, error_prefix='OCI_ERROR:')
        return target

    def check(self, image, resources=True):
        self.step = 'superuser'
        if self.db.other_superuser():
            self.result.write('waiting', self.step, SUPERUSER_REFUSED, self.hash, self.manifest, self.mode)
            return False
        self.step = 'compatibility'
        if not self.kind.compatible(self.manifest, self.db.describe(image)):
            self.result.write('waiting', self.step, self.kind.waiting, self.hash, self.manifest, self.mode)
            return False
        self.db.check_extensions(self.manifest)
        if resources:
            self.step = 'resources'
            self.resources()
        return True

    def discard_targets(self):
        path = self.host.paths.state / (self.kind.key + '-targets')
        if path.is_symlink() or path.is_file():
            path.unlink()
        elif path.is_dir():
            shutil.rmtree(path)

    def enter(self, mode):
        self.catalog = self.db.catalog()
        self.records.reconcile(self.catalog)
        try:
            self.pointer = self.tables.pointer()
        except PointerRefused:
            self.record.raise_alert('pointer_changed')
            raise
        self.records.observe_pointer(self.pointer, self.catalog)
        operation = self.record.operation
        own = operation['id'] if operation else None
        self.tables.operations(own)
        if mode == 'activate':
            self.discard_targets()
            self.tables.close_open(own)

    def conclude(self):
        operation = self.record.operation
        self.mode = operation['mode']
        self.state.settle(self.kind.key, self.hash, self.manifest['sequence'], True)
        self.dump_path().unlink(missing_ok=True)
        self.record.clear_operation()
        self.db.tidy()
        self.result.write('ok', 'complete', self.label + ' release activated', self.hash, self.manifest, self.mode)
        return True

    def restart(self):
        self.record.clear_operation()
        self.tables.close_open()

    def finish_operation(self):
        self.engine.catch_up(self.manifest)
        self.engine.finish(self.manifest)
        return self.conclude()

    def resume_operation(self):
        operation = self.record.operation
        image = self.image()
        if not self.check(image):
            return True
        self.step = 'recovery'
        self.db.clear_leftovers()
        early = operation['step'] in ('started', 'creating', 'created', 'restoring')
        dump = self.download() if operation['mode'] == 'new' and early else None
        self.engine.resume(self.manifest, dump, image, recheck=True)
        return self.conclude()

    def adopt(self):
        image = self.image()
        if not self.check(image, resources=False):
            return True
        self.step = 'adoption'
        self.engine.adopt(self.hash, self.manifest, image, self.pointer, self.catalog)
        return self.conclude()

    def recover(self, mode):
        operation = self.record.operation
        if operation is None:
            return None
        self.load(operation)
        self.mode = operation['mode']
        if mode == 'verify':
            self.result.write('verified', 'recovery', 'interrupted ' + self.label + ' activation; activation disabled',
                              self.hash, self.manifest, self.mode)
            return True
        step, target = operation['step'], operation['database']
        recorded = operation['oid'] is None or self.catalog.get(target, {}).get('oid') == operation['oid']
        if step == 'activated':
            return self.conclude()
        if step == 'aborted':
            self.engine.abort(self.manifest)
            self.result.write('failed', 'aborted', self.label + ' activation aborted', self.hash, self.manifest, self.mode)
            return False
        if step in BEFORE_SWAPPING:
            if self.pointer == target:
                return self.adopt()
            if recorded:
                return self.resume_operation()
            self.restart()
            return None
        if self.pointer == target:
            if recorded:
                return self.finish_operation()
            self.record.raise_alert('serving_database_gone')
            raise Held('serving database gone')
        if self.pointer == operation['expected']:
            if recorded:
                return self.finish_operation()
            self.restart()
            return None
        self.record.raise_alert('pointer_changed')
        raise Held('pointer changed')

    def recovery(self, mode):
        with self.db.lock():
            self.enter(mode)
            if self.record.operation is None or mode == 'verify':
                return self.recover(mode)
        self.prepare_activation()
        with self.db.lock():
            self.enter(mode)
            return self.recover(mode)

    def prune(self, mode):
        if mode != 'activate':
            return
        marker = self.host.paths.state / self.kind.marker
        day = datetime.fromtimestamp(self.clock(), timezone.utc).date().isoformat()
        previous = json.loads(marker.read_bytes()) if marker.exists() else None
        if previous == day:
            return
        with self.db.lock():
            self.enter(mode)
            self.step = 'prune'
            self.engine.prune(self.catalog)
            atomic(marker, canonical(day))

    def fetch(self, directory):
        repository = self.kind.settings(self.rules)[0]
        return self.registry.fetch(repository, self.kind.tag, Path(directory)/self.label, 'corpus-release')

    def sequence_of(self, digest):
        return next((int(number) for number, record in self.state.records(self.kind.key).items()
                     if record['hash'] == digest), 0)

    def newest(self):
        return self.manifest['sequence'] >= max((int(n) for n in self.state.records(self.kind.key)), default=0)

    def start(self, mode):
        adopting = self.pointer == self.manifest['database']
        oid = None
        if adopting:
            self.mode = 'retained'
        else:
            self.mode, oid = self.engine.choose(self.manifest, self.pointer, self.catalog)
        if not self.newest():
            raise Waiting('a newer ' + self.label + ' manifest was accepted')
        image = self.image()
        if not self.check(image, resources=not adopting):
            return True
        if mode == 'verify':
            self.result.write('verified', 'complete', self.label + ' verified; activation disabled',
                              self.hash, self.manifest, self.mode)
            return True
        if adopting:
            self.step = 'adoption'
            self.engine.adopt(self.hash, self.manifest, image, self.pointer, self.catalog)
            return self.conclude()
        self.step = 'download'
        dump = self.download() if self.mode == 'new' else None
        self.step = 'activation'
        self.engine.begin(self.hash, self.manifest, self.mode, self.pointer, oid)
        self.engine.resume(self.manifest, dump, image)
        return self.conclude()

    def noop(self):
        failed = self.hash in self.state.failed(self.kind.key)
        previous = self.result.previous
        self.mode = previous.get('mode') if previous.get('manifest_hash') == self.hash else None
        self.result.write('failed' if failed else 'current', 'noop',
                          self.label + ' manifest failed; re-sign the release to retry' if failed
                          else self.label + ' manifest already seen',
                          self.hash, self.manifest, self.mode)
        return not failed

    def advance(self, directory, mode, held):
        self.step = 'fetch'
        operation = self.record.operation if held else None
        waiting_on = self.hash, self.manifest
        try:
            path = self.fetch(directory)
            self.step = 'manifest'
            self.hash, self.manifest = self.kind.verify(path, self.rules, self.host.runner)
            decision = self.state.decision(self.kind.key, self.hash, self.manifest['sequence'])
        except CommandError as error:
            if held:
                self.hash, self.manifest = waiting_on
                raise held from error
            if error.reason in ('not_found', 'denied') and not self.state.records(self.kind.key):
                self.result.write('current', 'fetch', self.kind.unpublished)
                return True
            raise
        except ValueError as error:
            if held:
                self.hash, self.manifest = waiting_on
                raise held from error
            raise
        if held and (decision == 'noop' or self.manifest['sequence'] <= self.sequence_of(operation['manifest_hash'])):
            self.hash, self.manifest = waiting_on
            raise held
        if decision == 'noop':
            return self.noop()
        self.state.accept(self.kind.key, self.hash, self.manifest['sequence'])
        self.remember(path)
        if mode == 'activate':
            self.prepare_activation()
        with self.db.lock():
            self.enter(mode)
            if held:
                self.restart()
            if mode == 'activate':
                self.step = 'leftovers'
                self.db.clear_leftovers()
            self.step = 'manifest'
            return self.start(mode)

    def run(self, directory, mode):
        try:
            if not self.kind.ready(self.state, self.rules):
                self.result.write('waiting', 'floor', 'release floor')
                return True
            self.step = 'provision'
            self.provision()
            self.db.discard_lists()
            self.step = 'reconcile'
            self.records.reconcile(self.db.catalog(locked=False))
            self.step = 'recovery'
            held = None
            try:
                recovered = self.recovery(mode)
                if recovered is not None:
                    return recovered
                self.prune(mode)
            except Held as error:
                held = error
            return self.advance(directory, mode, held)
        except ActivatorNotProvisioned:
            self.result.write('waiting', 'provision', 'activator not provisioned', self.hash, self.manifest, self.mode)
            return True
        except JournalRefused as error:
            self.interrupted = interrupted(error)
            if self.record.data is not None:
                self.record.raise_alert('journal_row_refused')
            self.result.write('waiting', self.step, REFUSED, self.hash, self.manifest, self.mode)
            return True
        except Refused as error:
            self.state.fail(self.hash, self.kind.key)
            self.result.write('failed', self.step, str(error), self.hash, self.manifest, self.mode)
            return False
        except Waiting as error:
            self.interrupted = interrupted(error)
            if isinstance(error, LockUnavailable):
                self.record.raise_alert('lock_unavailable')
            self.result.write('waiting', self.step, str(error), self.hash, self.manifest, self.mode)
            return True
        except (OSError, ValueError, KeyError, TypeError, CommandError, InterruptedError, RecursionError) as error:
            self.interrupted = interrupted(error)
            reason = str(error) if isinstance(error, ValueError) else self.label + ' command failed or run interrupted'
            if isinstance(error, RecursionError):
                reason = self.label + ' input nested too deeply'
            cause = self.kind.cause(error) if isinstance(error, CommandError) else None
            if cause:
                reason += ': ' + cause
            self.result.write('failed', self.step, reason, self.hash, self.manifest, self.mode)
            return False
