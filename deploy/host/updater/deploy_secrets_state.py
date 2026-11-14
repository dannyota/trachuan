from datetime import datetime, timezone
import json
from pathlib import Path

from .deploy_io import atomic, canonical
from .deploy_vault import names


class SecretsState:
    def __init__(self, path):
        self.path = Path(path)
        try:
            self.data = json.loads(self.path.read_bytes())
        except FileNotFoundError:
            self.data = dict(versions={}, pending={}, last_success_at=None, held=False)
        self.validate()

    def validate(self):
        try:
            value = self.data
            if not isinstance(value, dict) or set(value) != {'versions', 'pending', 'last_success_at', 'held'}:
                raise ValueError('secret state fields')
            for key in ('versions', 'pending'):
                if not isinstance(value[key], dict) or not set(value[key]).issubset(names()):
                    raise ValueError('secret state names')
                if any(type(version) is not int or version < 1 for version in value[key].values()):
                    raise ValueError('secret state version')
            if type(value['held']) is not bool:
                raise ValueError('secret state hold')
            if value['last_success_at'] is not None:
                timestamp = datetime.fromisoformat(value['last_success_at'].replace('Z', '+00:00'))
                if timestamp.utcoffset() != timezone.utc.utcoffset(timestamp):
                    raise ValueError('secret state timestamp')
        except (ValueError, KeyError, TypeError, AttributeError) as error:
            raise ValueError('invalid secret state') from error

    def save(self):
        self.validate()
        atomic(self.path, canonical(self.data) + b'\n', 0o600)
