import base64
import copy
from datetime import datetime, timezone
import json
from pathlib import Path
import re
import time
from urllib.parse import quote

from .deploy_io import CommandError, Result
from .deploy_secrets_state import SecretsState


def consumers(directory, secrets):
    found = set()
    required = ('postgres', 'migrate', 'queryembed', 'server', 'web', 'caddy')
    if any(not (directory / ('trachuan-' + name + '.container')).is_file() for name in required):
        raise ValueError('rendered Quadlet unit missing')
    for path in sorted(directory.glob('trachuan-*.container')):
        section = None
        for line in path.read_text().splitlines():
            line = line.strip()
            if line.startswith('['):
                section = line
            if section == '[Container]' and line.startswith('Secret='):
                name = line.split('=', 1)[1].split(',', 1)[0].strip()
                if name in secrets:
                    service = path.name[len('trachuan-'):-len('.container')]
                    if service not in ('postgres', 'migrate', 'queryembed', 'server', 'web', 'caddy'):
                        raise ValueError('unsupported secret consumer')
                    found.add(service)
    return found


def derived(name, value):
    dependency = {'auth_db_password': ('auth_owner', 'auth', 'server_auth_database_url'),
                  'corpus_reader_password': ('corpus_reader', '{database}', 'server_corpus_database_url')}
    if name not in dependency:
        return {}
    role, database, target = dependency[name]
    password = quote(value.decode('utf-8'), safe='')
    return {target: ('postgres://' + role + ':' + password + '@127.0.0.1:5432/' + database + '?sslmode=disable').encode()}


class Secrets:
    def __init__(self, state, host, rules, clock=time.time):
        self.state, self.host, self.rules, self.clock = state, host, rules, clock
        try:
            self.storage = SecretsState(host.paths.state / 'secrets-state.json')
        except (OSError, ValueError):
            self.storage = None
        self.data = self.storage.data if self.storage else None
        self.failed = False
        self.name = 'vault'

    def due(self, force):
        last = self.data['last_success_at']
        return (force or bool(self.data['pending']) or last is None or
                self.clock() - datetime.fromisoformat(last.replace('Z', '+00:00')).timestamp() >= 3600)

    def fetch(self, name):
        self.name = name
        raw = self.host.runner(['python3', '-I', str(Path(__file__).with_name('deploy_vault.py')),
                                self.rules['region'], self.rules['vault_id'], name],
                               timeout=60, stdout_limit=700000, error_prefix='OCI_ERROR:')
        fields = raw.split(b'\0')
        if len(fields) != 4 or fields[0] != name.encode() or fields[3] or not re.fullmatch(rb'[1-9][0-9]{0,19}', fields[1]):
            raise ValueError('invalid Vault record')
        value = base64.b64decode(fields[2], validate=True)
        if not value or len(value) > 512000:
            raise ValueError('invalid Vault value size')
        return int(fields[1]), value

    def exists(self, name):
        try:
            self.host.user('podman', 'secret', 'exists', name, timeout=30)
            return True
        except CommandError as error:
            if error.returncode != 1:
                raise
            return False

    def equal(self, name, value):
        raw = self.host.user('podman', 'secret', 'inspect', '--showsecret', name,
                             timeout=30, stdout_limit=4 * 1024 * 1024)
        records = json.loads(raw)
        if (not isinstance(records, list) or len(records) != 1 or not isinstance(records[0], dict)
                or not isinstance(records[0].get('SecretData'), str)):
            raise ValueError('invalid Podman secret inspection')
        return records[0]['SecretData'].encode('utf-8') == value

    def plan(self, bundles):
        work = {}
        for name, (version, value) in bundles.items():
            self.name = name
            dependencies = derived(name, value)
            missing = not self.exists(name)
            missing_derived = any(not self.exists(target) for target in dependencies)
            recorded = self.data['versions'].get(name)
            pending = name in self.data['pending']
            if not pending and not missing and not missing_derived:
                if recorded == version:
                    continue
                if recorded is None and self.equal(name, value):
                    self.data['versions'][name] = version
                    continue
            work[name] = dict({name: value}, **dependencies)
        return work

    def recreate(self, affected):
        if affected:
            self.host.systemctl('stop', 'caddy', 'web', 'server', 'migrate', 'queryembed')
            if 'postgres' in affected:
                self.host.postgres()
            self.host.migrate()
            self.host.systemctl('start', 'server', 'web')
        elif not self.host.paths.hold.exists():
            return
        self.host.probe()
        journal = self.state.data['journal'] or {}
        if not self.data['held'] and journal.get('status') != 'stopped':
            self.host.clear_hold()
            self.host.systemctl('start', 'caddy')
            self.host.probe(edge=True)

    def reconcile(self, bundles):
        work = self.plan(bundles)
        affected = consumers(self.host.paths.quadlet, {name for values in work.values() for name in values})
        if work:
            if not self.data['pending']:
                self.data['held'] = self.host.paths.hold.exists()
            self.data['pending'] = {name: bundles[name][0] for name in work}
            self.storage.save()
            if affected:
                self.host.hold()
                self.host.systemctl('stop', 'caddy')
            for name, values in work.items():
                self.name = name
                for target, value in values.items():
                    self.host.user('podman', 'secret', 'create', '--replace', target, '-',
                                   input=value, timeout=60)
            self.recreate(affected)
        previous = copy.deepcopy(self.data)
        self.data['versions'].update(self.data['pending'])
        self.data['pending'] = {}
        self.data['held'] = False
        self.data['last_success_at'] = datetime.fromtimestamp(self.clock(), timezone.utc).isoformat(timespec='seconds').replace('+00:00', 'Z')
        try:
            self.storage.save()
        except BaseException:
            self.data = previous
            self.storage.data = previous
            raise
        return bool(work)

    def run(self, force=False):
        result = Result(self.host.paths.result.with_name('secrets.json'), self.host.paths.log)
        try:
            if self.storage is None:
                self.name = 'state'
                raise ValueError('invalid secret state')
            if not self.due(force):
                return True
            if 'REPLACE' in self.rules['vault_id'].upper():
                raise ValueError('placeholder vault id')
            bundles = {name: self.fetch(name) for name in self.rules['secrets']}
            changed = self.reconcile(bundles)
            result.write('ok' if changed else 'current', 'secrets', 'Vault secrets reconciled')
            return True
        except (OSError, ValueError, TypeError, CommandError, InterruptedError) as error:
            self.failed = True
            code = error.code if isinstance(error, CommandError) and error.code else 'ReconcileError'
            if self.storage is None:
                code = 'InvalidSecretsState'
            elif isinstance(error, ValueError) and 'REPLACE' in self.rules['vault_id'].upper():
                code = 'PlaceholderVault'
            result.write('failed', 'secrets', self.name + ': ' + code)
            if self.data and self.data['pending']:
                self.host.hold()
                try:
                    self.host.systemctl('stop', 'caddy')
                except (OSError, CommandError, InterruptedError):
                    pass
                return False
            return True
        except BaseException:
            if self.data and self.data['pending']:
                self.host.hold()
            raise
