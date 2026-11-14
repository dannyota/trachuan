import json
import os
from pathlib import Path
import tempfile

from .deploy_activate import Activator
from .deploy_bundle import bundle
from .deploy_host import Host
from .deploy_io import atomic, Busy, command, CommandError, Lock, Result
from .deploy_registry import Registry
from .deploy_secrets import Secrets
from .deploy_state import State
from .deploy_store import Candidate, Paths, Store
from .deploy_verify import match, policy, Verifier, verify_manifest


class Updater:
    def __init__(self, paths=None, runner=command, user_prefix=None):
        self.paths, self.runner = paths or Paths(), runner
        self.rules = policy(self.paths.policy)
        self.host = Host(self.paths, runner, user_prefix)
        self.verifier = Verifier(self.rules, runner, user_prefix)
        self.registry = Registry(self.rules, runner)
        self.store = Store(self.paths, self.rules, runner)
        self.phases = ('secrets', 'images')
        self.secrets_failed = False
        self.step, self.hash, self.manifest = 'starting', None, None

    def mode(self):
        mode = self.paths.mode.read_text().strip()
        if mode not in ('verify', 'activate'):
            raise ValueError('invalid deploy mode')
        return mode

    def backup_pending(self, result):
        if os.path.lexists(self.paths.backup_lock.with_name('active-job')):
            result.write('skipped', 'backup', 'backup cancellation unconfirmed; retry later')
            return True
        return False

    def noop(self, result, state):
        journal = state.data['journal'] or {}
        status = 'current' if self.hash == state.data['active'] else 'skipped'
        if self.hash in state.data['failed'] or journal.get('status') == 'stopped':
            status = journal.get('status') if journal.get('status') in ('rolled_back', 'stopped') else 'failed'
        result.write(status, 'noop', 'manifest already seen', self.hash, self.manifest)

    def secrets(self, state, directory, mode, result, force=False):
        self.step = 'secrets'
        phase = Secrets(state, self.host, self.rules)
        proceed = phase.run(force=True) if force else phase.run()
        self.secrets_failed = self.secrets_failed or phase.failed
        if not proceed:
            result.write('failed', 'secrets', 'secret replacement pending')
        return proceed

    def verified(self, state, candidate, mode, result):
        self.step = 'images'
        self.verifier.images(candidate.manifest)
        self.step = 'bundle'
        candidate.files = bundle(candidate.directory / 'host-bundle.tar.gz', candidate.manifest)
        if mode == 'verify':
            result.write('verified', 'complete', 'release verified; activation disabled', candidate.hash, candidate.manifest)
            return
        if not self.secrets(state, candidate.directory, mode, result, force=True):
            return
        self.step = 'activate'
        Activator(state, self.store, self.host).activate(candidate)
        result.write('ok', 'complete', 'release activated', candidate.hash, candidate.manifest)

    def images(self, state, directory, mode, result):
        self.step = 'fetch'
        production = self.registry.fetch(self.rules['release_repository'], 'production', directory / 'production', 'release')
        tag = self.registry.latest_rollback(bool(state.data['accepted'].get('rollback')))
        if getattr(self.registry, 'rollback_unreadable', False) is True:
            result.append(dict(status='warning', step='fetch',
                               message='rollback channel unreadable; rollback authorizations withheld'))
        authorization = None
        if tag:
            rollback_path = self.registry.fetch(self.rules['rollback_repository'], tag, directory / 'rollback', 'rollback')
            authorization = verify_manifest(rollback_path, self.rules, self.runner, 'rollback')
        self.step = 'manifest'
        self.hash, self.manifest = verify_manifest(production, self.rules, self.runner)
        if authorization and authorization[1]['current'] == state.data['active'] and authorization[0] not in state.data['consumed']:
            self.apply_rollback(state, authorization, mode, result)
            return
        decision = state.decision('image', self.hash, self.manifest['sequence'])
        if decision != 'noop':
            state.accept('image', self.hash, self.manifest['sequence'])
            self.verified(state, Candidate(self.hash, self.manifest, production, {}), mode, result)
        elif authorization:
            self.apply_rollback(state, authorization, mode, result)
        else:
            self.noop(result, state)

    def apply_rollback(self, state, authorization, mode, result):
        hash_value, auth = authorization
        record = state.data['accepted']['rollback'].get(str(auth['sequence']))
        if record and record['hash'] == hash_value and record['status'] == 'consumed':
            self.noop(result, state)
            return
        self.step, self.hash, self.manifest = 'rollback', hash_value, auth
        schema = self.host.schema()
        state.check_rollback(hash_value, auth, schema)
        candidate = self.store.load(auth['target'])
        current = self.store.load(state.data['active'])
        if candidate.manifest['sequence'] >= current.manifest['sequence']:
            raise ValueError('rollback target must have a lower sequence')
        state.accept('rollback', hash_value, auth['sequence'])
        self.verifier.images(candidate.manifest)
        if mode == 'verify':
            result.write('verified', 'rollback', 'rollback verified; activation disabled', hash_value, auth)
            return
        if not self.secrets(state, candidate.directory, mode, result, force=True):
            return
        self.step = 'rollback'
        Activator(state, self.store, self.host).activate(candidate, expected_schema=schema,
                                                       authorization=hash_value, rollback=auth)
        result.write('ok', 'rollback', 'authorized rollback activated', hash_value, auth)

    def failure(self, result, state, error):
        status = None
        if state and self.step != 'recovery' and not state.data['secrets']['pending']:
            try:
                status = Activator(state, self.store, self.host).recover()
            except (OSError, ValueError, CommandError, InterruptedError):
                status = 'failed'
        if isinstance(error, ValueError):
            reason = str(error)
        elif isinstance(error, CommandError) and error.detail:
            reason = str(error) + ': ' + error.detail
        else:
            reason = 'command failed or run interrupted'
        result.write(status or 'failed', self.step, reason, self.hash, self.manifest)
        return 1

    def run(self):
        state = None
        self.secrets_failed = False
        try:
            with Lock(self.paths.lock), Lock(self.paths.backup_lock, create=False):
                result = Result(self.paths.result, self.paths.log)
                try:
                    mode = self.mode()
                    state = State(self.paths.state / 'state.json')
                    self.step = 'recovery'
                    activator = Activator(state, self.store, self.host)
                    stopped = activator.enforce_stopped()
                    if self.backup_pending(result):
                        return 0
                    recovered = None if stopped or state.data['secrets']['pending'] else activator.recover()
                    if recovered:
                        journal = state.data['journal']
                        result.write(recovered, 'recovery', 'interrupted activation recovered', journal['target'])
                        return 1
                    with tempfile.TemporaryDirectory(prefix='run-', dir=self.paths.state) as directory:
                        for phase in self.phases:
                            if getattr(self, phase)(state, Path(directory), mode, result) is False:
                                return 1
                    return int(self.secrets_failed)
                except (OSError, ValueError, KeyError, TypeError, CommandError, InterruptedError) as error:
                    return self.failure(result, state, error)
        except Busy:
            return 0

    def bootstrap(self, hash_value):
        match('[0-9a-f]{64}', hash_value, 'bootstrap manifest hash')
        try:
            with Lock(self.paths.lock), Lock(self.paths.backup_lock, create=False):
                result = Result(self.paths.result, self.paths.log)
                try:
                    if self.backup_pending(result):
                        return 1
                    with tempfile.TemporaryDirectory(prefix='bootstrap-', dir=self.paths.state) as directory:
                        path = self.registry.find(hash_value, Path(directory))
                        candidate = Candidate.read(path, self.rules, self.runner)
                        if candidate.hash != hash_value:
                            raise ValueError('bootstrap hash mismatch')
                        self.verifier.images(candidate.manifest)
                        self.host.confirm(candidate)
                        schema = self.host.schema()
                        self.store.retain(candidate)
                        atomic(self.paths.mode, b'verify\n', 0o644)
                        state = State.bootstrap(self.paths.state / 'state.json', hash_value,
                                        candidate.manifest['sequence'], schema,
                                        candidate.manifest['host_bundle']['sha256'])
                        self.store.prune(state)
                        self.runner(['systemctl', 'enable', '--now', 'trachuan-deploy.timer'])
                        result.write('bootstrapped', 'complete', 'bootstrap verified; mode verify', hash_value, candidate.manifest)
                        return 0
                except (OSError, ValueError, KeyError, TypeError, CommandError, InterruptedError) as error:
                    return self.failure(result, None, error)
        except Busy:
            return 1

    def status(self):
        state = State(self.paths.state / 'state.json')
        result = json.loads(self.paths.result.read_bytes()) if self.paths.result.exists() else None
        return dict(mode=self.mode(), active=state.data['active'], journal=state.data['journal'], result=result)
