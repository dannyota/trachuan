from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import shutil
import time

from .deploy_corpus_activate import Activation, COMPLETE
from .deploy_corpus_database import Database
from .deploy_corpus_kind import LAW
from .deploy_corpus_result import ReleaseResult
from .deploy_corpus_sql import SUPERUSER_REFUSED
from .deploy_release_target import Leftover
from .deploy_io import atomic, canonical, CommandError, Busy, interrupted


class ActivatorNotProvisioned(Exception):
    pass


class Corpus:
    def __init__(self, state, store, host, rules, registry, database=None, prepare=lambda: True, clock=time.time,
                 kind=LAW):
        self.state, self.store, self.host, self.rules, self.registry = state, store, host, rules, registry
        self.kind, self.label = kind, kind.label
        self.db = database or Database(host, rules, kind)
        self.engine = Activation(self.db, kind)
        self.prepare, self.clock = prepare, clock
        self.result = ReleaseResult(host.paths, clock, kind.result)
        self.hash, self.manifest, self.mode, self.step = None, None, None, kind.label
        self.artifacts = host.paths.state / kind.artifacts
        self.interrupted = False

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

    def load(self, row):
        self.hash, self.manifest = self.kind.verify(self.artifacts / row['manifest_hash'], self.rules, self.host.runner)
        if (self.hash != row['manifest_hash'] or self.manifest != row['manifest']
                or row['database_name'] != self.manifest['database'] or row['rel'] != self.manifest['rel']
                or row['country'] != self.kind.country(self.manifest)
                or row['dump_sha256'] != self.manifest['dump']['sha256']):
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

    def finish(self, row):
        self.mode = row['mode']
        if row['step'] == 'aborted':
            self.state.fail(self.hash, self.kind.key)
            self.result.write('failed', 'aborted', self.label + ' activation guard changed',
                              self.hash, self.manifest, self.mode)
            return False
        active = self.db.current(self.kind.country(self.manifest)) == row['database_name']
        self.state.settle(self.kind.key, self.hash, self.manifest['sequence'], active)
        self.dump_path().unlink(missing_ok=True)
        self.result.write('ok', 'complete', self.label + ' release activated', self.hash, self.manifest, self.mode)
        return True

    def check(self, image):
        self.step = 'superuser'
        if self.db.other_superuser():
            self.result.write('waiting', self.step, SUPERUSER_REFUSED, self.hash, self.manifest, self.mode)
            return False
        self.step = 'compatibility'
        if not self.kind.compatible(self.manifest, self.db.describe(image)):
            self.result.write('waiting', self.step, self.kind.waiting, self.hash, self.manifest, self.mode)
            return False
        self.db.check_extensions(self.manifest)
        self.step = 'resources'
        self.resources()
        return True

    def recover(self, mode):
        with self.db.lock():
            rows = self.db.operations()
        pending = self.state.records(self.kind.key)
        for row in rows:
            if row['mode'] == 'prune':
                continue
            bound = next((r for r in pending.values() if r['hash'] == row['manifest_hash']), None)
            if row['step'] in ('activated', 'reverted') and bound and bound['status'] == 'pending':
                self.load(row)
                with self.db.lock():
                    return self.finish(row)
            if row['step'] in COMPLETE:
                continue
            self.load(row)
            self.mode = row['mode']
            if mode == 'verify':
                self.result.write('verified', 'recovery', 'interrupted ' + self.label + ' activation; activation disabled',
                                  self.hash, self.manifest, self.mode)
                return True
            self.prepare_activation()
            with self.db.lock():
                if bound and bound['status'] == 'failed':
                    self.engine.abort(row)
                    return False
                if not self.engine.guarded(row):
                    self.engine.abort(row)
                    return self.finish(dict(row, step='aborted'))
                image = self.image()
                if not self.check(image):
                    return True
                self.step = 'recovery'
                dump = self.download() if self.mode == 'new' and row['step'] in ('started','creating','created','restoring') else None
                return self.finish(self.engine.resume(row, dump, image, recheck=True))
        return None

    def prune(self, mode):
        if mode != 'activate':
            return
        marker = self.host.paths.state / self.kind.marker
        day = datetime.fromtimestamp(self.clock(), timezone.utc).date().isoformat()
        previous = json.loads(marker.read_bytes()) if marker.exists() else None
        with self.db.lock():
            interrupted = any(r['step'] == 'pruning' or (r['step'] == 'aborted'
                and self.db.owned(r['database_name']) and not self.db.protected(r['database_name']))
                for r in self.db.operations())
            if previous != day or interrupted:
                self.step = 'prune'
                self.engine.prune()
                atomic(marker, canonical(day))

    def run(self, directory, mode):
        try:
            if not self.kind.ready(self.state, self.rules):
                self.result.write('waiting', 'floor', 'release floor')
                return True
            self.step = 'provision'
            self.provision()
            self.db.discard_lists()
            recovered = self.recover(mode)
            if recovered is not None:
                return recovered
            self.prune(mode)
            self.step = 'fetch'
            repository = self.kind.settings(self.rules)[0]
            try:
                path = self.registry.fetch(repository, self.kind.tag, Path(directory)/self.label, 'corpus-release')
            except CommandError as error:
                if error.reason in ('not_found', 'denied') and not self.state.records(self.kind.key):
                    self.result.write('current', 'fetch', self.kind.unpublished)
                    return True
                raise
            self.step = 'manifest'
            self.hash, self.manifest = self.kind.verify(path, self.rules, self.host.runner)
            decision = self.state.decision(self.kind.key, self.hash, self.manifest['sequence'])
            if decision == 'noop':
                failed = self.hash in self.state.failed(self.kind.key)
                with self.db.lock():
                    self.mode = next((r['mode'] for r in self.db.operations()
                                      if r['manifest_hash'] == self.hash and r['mode'] != 'prune'), None)
                self.result.write('failed' if failed else 'current', 'noop',
                                  self.label + ' manifest failed; re-sign the release to retry' if failed
                                  else self.label + ' manifest already seen',
                                  self.hash, self.manifest, self.mode)
                return not failed
            self.state.accept(self.kind.key, self.hash, self.manifest['sequence'])
            self.remember(path)
            if mode == 'activate':
                self.prepare_activation()
            with self.db.lock():
                if mode == 'activate':
                    self.step = 'leftovers'
                    self.db.clear_leftovers()
                    self.step = 'manifest'
                self.mode, _ = self.engine.choose(self.manifest)
                image = self.image()
                if not self.check(image):
                    return True
                if mode == 'verify':
                    self.result.write('verified', 'complete', self.label + ' verified; activation disabled',
                                      self.hash, self.manifest, self.mode)
                    return True
                self.step = 'download'
                dump = self.download() if self.mode == 'new' else None
                self.step = 'activation'
                return self.finish(self.engine.run(self.hash, self.manifest, dump, image))
        except ActivatorNotProvisioned:
            self.result.write('waiting', 'provision', 'activator not provisioned', self.hash, self.manifest, self.mode)
            return True
        except Leftover as error:
            self.result.write('waiting', self.step, str(error), self.hash, self.manifest, self.mode)
            return True
        except Busy:
            self.result.write('skipped', self.step, 'activation advisory lock busy; retry later', self.hash, self.manifest, self.mode)
            return True
        except (OSError, ValueError, KeyError, TypeError, CommandError, InterruptedError, RecursionError) as error:
            self.interrupted = interrupted(error)
            reason = str(error) if isinstance(error, ValueError) else self.label + ' command failed or run interrupted'
            if isinstance(error, RecursionError):
                reason = self.label + ' input nested too deeply'
            cause = self.kind.cause(error) if isinstance(error, CommandError) else None
            if cause:
                reason += ': ' + cause
            if self.hash:
                try:
                    with self.db.lock():
                        rows = [r for r in self.db.operations() if r['manifest_hash'] == self.hash]
                        if rows and rows[-1]['step'] in ('activated','reverted'):
                            return self.finish(rows[-1])
                        if rows and rows[-1]['step'] == 'aborted':
                            self.state.fail(self.hash, self.kind.key)
                except (OSError, ValueError, CommandError, Busy) as inner:
                    self.interrupted = self.interrupted or interrupted(inner)
            self.result.write('failed', self.step, reason, self.hash, self.manifest, self.mode)
            return False
