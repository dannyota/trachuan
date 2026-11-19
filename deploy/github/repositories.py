import json
import os
from pathlib import Path
import re

POLICY = Path(__file__).resolve().parents[1] / 'host/deploy-policy.json'


def checked(value):
    if not isinstance(value, str) or re.fullmatch(r'[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+', value) is None:
        raise ValueError('invalid repository')
    return value


def workflow(argument=None):
    environment = os.environ.get('GITHUB_REPOSITORY')
    if argument is None and not environment:
        raise ValueError('pass --repository outside GitHub Actions')
    if argument is not None and environment and argument != environment:
        raise ValueError('--repository differs from GITHUB_REPOSITORY')
    return checked(environment if argument is None else argument)


def listed():
    value = json.loads(POLICY.read_bytes())['repositories']
    if not isinstance(value, list) or not 1 <= len(value) <= 2 or len({checked(name) for name in value}) != len(value):
        raise ValueError('invalid deploy policy repositories')
    return value


def require_listed(value):
    if not isinstance(value, str) or value not in listed():
        raise ValueError('repository is not in the deploy policy')
    return value


def package(repository, name):
    return 'ghcr.io/' + checked(repository).split('/')[0] + '/trachuan-' + name
