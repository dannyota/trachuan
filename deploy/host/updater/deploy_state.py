import json
from pathlib import Path
import re

from .deploy_io import atomic, canonical


def digest(value):
    return isinstance(value, str) and re.fullmatch('[0-9a-f]{64}', value) is not None


class State:
    def __init__(self, path, data=None):
        self.path = Path(path)
        try:
            self.data = data if data is not None else json.loads(self.path.read_bytes())
            self.validate()
        except (OSError, ValueError, KeyError, TypeError, AttributeError) as error:
            raise ValueError('state missing or invalid; bootstrap required') from error

    def validate(self):
        data = self.data
        if set(data) != {'version', 'active', 'accepted', 'retained', 'failed', 'consumed',
                         'schemas', 'installed_bundle', 'journal'} or data['version'] != 1:
            raise ValueError('state fields')
        if not digest(data['active']) or set(data['accepted']) != {'image', 'rollback', 'corpus'}:
            raise ValueError('state active')
        for records in data['accepted'].values():
            if not isinstance(records, dict):
                raise ValueError('state records')
            for sequence, record in records.items():
                if not sequence.isdigit() or int(sequence) < 1 or str(int(sequence)) != sequence:
                    raise ValueError('state sequence')
                if set(record) != {'hash', 'status'} or not digest(record['hash']):
                    raise ValueError('state binding')
                if record['status'] not in ('pending', 'active', 'retained', 'failed', 'consumed'):
                    raise ValueError('state status')
        for key in ('retained', 'failed', 'consumed'):
            if not isinstance(data[key], list) or not all(digest(h) for h in data[key]):
                raise ValueError('state hashes')
        if not isinstance(data['schemas'], dict) or data['active'] not in data['schemas']:
            raise ValueError('state schema')
        for key, version in data['schemas'].items():
            if not digest(key) or type(version) is not int or version < 0:
                raise ValueError('state schema')
        if data['installed_bundle'] is not None and not digest(data['installed_bundle']):
            raise ValueError('state bundle')
        active = [r for r in data['accepted']['image'].values() if r['hash'] == data['active']]
        if len(active) != 1 or active[0]['status'] != 'active':
            raise ValueError('state active binding')
        journal = data['journal']
        if journal is not None:
            if set(journal) != {'target', 'previous', 'baseline', 'restart_postgres', 'status', 'step', 'authorization'}:
                raise ValueError('journal fields')
            if not digest(journal['target']) or not digest(journal['previous']):
                raise ValueError('journal hashes')
            if type(journal['baseline']) is not int or journal['baseline'] < 0:
                raise ValueError('journal baseline')
            if type(journal['restart_postgres']) is not bool or journal['status'] not in (
                    'started', 'recovering', 'activated', 'rolled_back', 'stopped'):
                raise ValueError('journal status')
            if journal['authorization'] is not None and not digest(journal['authorization']):
                raise ValueError('journal authorization')

    @classmethod
    def bootstrap(cls, path, active, sequence, schema, bundle):
        data = dict(version=1, active=active,
                    accepted={'image': {str(sequence): {'hash': active, 'status': 'active'}},
                              'rollback': {}, 'corpus': {}},
                    retained=[], failed=[], consumed=[], schemas={active: schema},
                    installed_bundle=bundle, journal=None)
        state = cls(path, data)
        state.save()
        return state

    def save(self):
        self.validate()
        atomic(self.path, canonical(self.data) + b'\n')

    def decision(self, kind, hash_value, sequence):
        if type(sequence) is not int or sequence < 1:
            raise ValueError('older or invalid sequence')
        records = self.data['accepted'][kind]
        bound = records.get(str(sequence))
        if bound and bound['hash'] != hash_value:
            raise ValueError('sequence reuse')
        if hash_value in [self.data['active'], *self.data['retained'], *self.data['failed']]:
            return 'noop'
        if hash_value in self.data['consumed']:
            raise ValueError('rollback consumed')
        if bound:
            return 'pending' if bound['status'] == 'pending' else 'noop'
        if sequence <= max((int(n) for n in records), default=0):
            raise ValueError('older sequence')
        if not digest(hash_value):
            raise ValueError('invalid manifest hash')
        return 'new'

    def accept(self, kind, hash_value, sequence):
        decision = self.decision(kind, hash_value, sequence)
        if decision == 'new':
            self.data['accepted'][kind][str(sequence)] = {'hash': hash_value, 'status': 'pending'}
            self.save()
        return decision

    def fail(self, hash_value):
        self.mark_failed(hash_value)
        self.save()

    def mark_failed(self, hash_value):
        if hash_value not in self.data['failed']:
            self.data['failed'].append(hash_value)
        self.data['retained'] = [h for h in self.data['retained'] if h != hash_value]
        for records in self.data['accepted'].values():
            for record in records.values():
                if record['hash'] == hash_value:
                    record['status'] = 'failed'

    def activate(self, hash_value, schema, authorization=None):
        previous = self.data['active']
        if previous != hash_value:
            self.data['retained'] = [previous] + [h for h in self.data['retained'] if h not in (previous, hash_value)]
        else:
            self.data['retained'] = [h for h in self.data['retained'] if h != hash_value]
        self.data['retained'] = self.data['retained'][:4]
        self.data['active'] = hash_value
        self.data['schemas'][hash_value] = schema
        records = self.data['accepted']['image']
        sequence = next(int(n) for n, row in records.items() if row['hash'] == hash_value)
        for n, row in records.items():
            if row['hash'] == hash_value:
                row['status'] = 'active'
            elif row['status'] == 'active':
                row['status'] = 'retained'
            elif row['status'] == 'pending' and int(n) < sequence:
                self.mark_failed(row['hash'])
        if authorization:
            records = self.data['accepted']['rollback']
            sequence = next(int(n) for n, row in records.items() if row['hash'] == authorization)
            for n, row in records.items():
                if row['status'] == 'pending' and int(n) < sequence:
                    self.mark_failed(row['hash'])
        self.save()

    def check_rollback(self, hash_value, auth, schema):
        if hash_value in self.data['consumed']:
            raise ValueError('rollback consumed')
        decision = self.decision('rollback', hash_value, auth['sequence'])
        if decision == 'noop' or auth['current'] != self.data['active']:
            raise ValueError('rollback current mismatch or replay')
        if auth['target'] not in self.data['retained']:
            raise ValueError('rollback target not retained')
        if self.data['schemas'].get(auth['target']) != schema:
            raise ValueError('rollback schema changed')
        return decision

    def authorize(self, hash_value, auth, schema):
        self.check_rollback(hash_value, auth, schema)
        journal = self.data['journal']
        if not journal or journal['authorization'] != hash_value or journal['target'] != auth['target'] or journal['previous'] != auth['current'] or journal['baseline'] != schema or journal['status'] != 'started':
            raise ValueError('rollback activation journal required')
        self.data['consumed'].append(hash_value)
        self.data['accepted']['rollback'][str(auth['sequence'])] = {'hash': hash_value, 'status': 'consumed'}
        self.save()
