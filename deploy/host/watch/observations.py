import os
from pathlib import Path
import subprocess

from . import network, storage


INTEGRITY = ('pointer_changed', 'serving_database_gone', 'active_cleared', 'adoption_failed', 'record_rebuilt',
             'journal_row_refused', 'lock_unavailable')


def result(path, kind):
    value = storage.read(path)
    if value is None:
        return None
    statuses = {
        'deploy': ('ok', 'current', 'failed', 'rolled_back', 'stopped', 'verified', 'skipped', 'bootstrapped', 'running'),
        'release': ('ok', 'current', 'verified', 'failed', 'waiting', 'skipped', 'running'),
        'secrets': ('ok', 'current', 'failed', 'skipped', 'running'),
        'backup': ('ok', 'failed', 'unknown', 'running', 'skipped'),
    }
    if value.get('status') not in statuses[kind]:
        raise ValueError('invalid watch job status')
    fields = ('started_at', 'finished_at', 'last_success_at', 'waiting_since')
    for field in fields:
        value[field] = storage.timestamp(value.get(field))
    if value['status'] in ('failed', 'rolled_back', 'unknown') and value['started_at'] is None:
        raise ValueError('missing watch attempt timestamp')
    if value['status'] == 'waiting' and value['waiting_since'] is None:
        raise ValueError('missing watch waiting timestamp')
    if value.get('release') is not None and (not isinstance(value['release'], str) or
                                            not 0 < len(value['release']) <= 256):
        raise ValueError('invalid watch release identity')
    if value.get('sequence') is not None and (type(value['sequence']) is not int or value['sequence'] < 1):
        raise ValueError('invalid watch sequence')
    if value.get('message') is not None and not isinstance(value['message'], str):
        raise ValueError('invalid watch job message')
    integrity = value.get('integrity', [])
    if not isinstance(integrity, list) or not all(code in INTEGRITY for code in integrity):
        raise ValueError('invalid watch integrity list')
    return value


def filesystem(path):
    value = os.statvfs(path)
    used = value.f_blocks-value.f_bfree
    return used/(used+value.f_bavail)*100


def memory(path=Path('/proc/meminfo')):
    values = {}
    for line in path.read_text().splitlines():
        name, _, value = line.partition(':')
        if name in ('MemTotal', 'MemAvailable'):
            fields = value.split()
            if len(fields) != 2 or fields[1] != 'kB':
                raise ValueError('invalid watch memory input')
            values[name] = int(fields[0])
    total, available = values.get('MemTotal', 0), values.get('MemAvailable', -1)
    if total <= 0 or not 0 <= available <= total:
        raise ValueError('invalid watch memory input')
    return available/total*100


def deploy_state():
    result = subprocess.run(['/usr/bin/systemctl', 'is-active', 'trachuan-deploy.service'],
                            capture_output=True, timeout=5, check=False)
    if result.returncode not in (0, 3):
        raise ValueError('deploy activity read failed')
    return result.stdout.decode().strip()


def collect(root=Path('/srv/trachuan/state'), activity=deploy_state,
            get=network.local, disk=filesystem, mem=memory):
    data = {name: result(root / 'deploy' / file, kind) for name, file, kind in
            (('deploy', 'result.json', 'deploy'), ('release', 'release.json', 'release'),
             ('frameworks', 'release-frameworks.json', 'release'), ('secrets', 'secrets.json', 'secrets'))}
    data['backups'] = {job: result(root / 'backup' / (job+'.json'), 'backup')
                       for job in ('full', 'diff', 'verify', 'health', 'restore-check')}
    data['ready'] = None
    if activity() not in ('active', 'activating'):
        try:
            data['ready'] = get('/readyz', 5)[0]
        except Exception:
            data['ready'] = 0
    data['corpus'] = None
    try:
        status, body = get('/api/v1/public/status', 5)
        release = body.get('release') if isinstance(body, dict) else None
        if status == 200 and isinstance(release, str) and 0 < len(release) <= 256:
            data['corpus'] = release
    except Exception:
        pass
    data['disk'] = {mount: disk(mount) for mount in ('/', '/srv/trachuan/data')}
    data['memory'] = mem()
    return data
