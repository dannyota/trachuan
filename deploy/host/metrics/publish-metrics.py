import json
import os
from pathlib import Path
import signal
import stat
import socket
import sys
from datetime import datetime, timezone
from urllib.request import Request, urlopen


BACKUP_DIR = Path('/srv/trachuan/state/backup')
DEPLOY_RESULT = Path('/srv/trachuan/state/deploy/result.json')
SECRETS_RESULT = Path('/srv/trachuan/state/deploy/secrets.json')
RELEASE_RESULT = Path('/srv/trachuan/state/deploy/release.json')
FRAMEWORKS_RESULT = Path('/srv/trachuan/state/deploy/release-frameworks.json')
WATCH_RESULT = Path('/srv/trachuan/state/watch/result.json')
JOBS = ('full', 'diff', 'verify', 'health', 'restore-check')
MOUNTS = ('/', '/srv/trachuan/data')
MAX_RESULT_BYTES = 65536


def filesystem(path):
    stat = os.statvfs(path)
    used = stat.f_blocks - stat.f_bfree
    return used / (used + stat.f_bavail) * 100, stat.f_bavail * stat.f_frsize


def read_result(path):
    try:
        fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    except FileNotFoundError:
        return None
    with os.fdopen(fd, 'rb') as source:
        if not stat.S_ISREG(os.fstat(source.fileno()).st_mode):
            raise ValueError('job result is not a regular file')
        raw = source.read(MAX_RESULT_BYTES + 1)
    if len(raw) > MAX_RESULT_BYTES:
        raise ValueError('job result too large')
    return json.loads(raw.decode('utf-8'))


def job_result(path, now, prefix, statuses, failures):
    result = read_result(path)
    return job_metrics(result, now, prefix, statuses, failures)


def job_metrics(result, now, prefix, statuses, failures):
    if result is None:
        return []
    status = result['status']
    if status not in statuses:
        raise ValueError('invalid job status')
    metrics = [(prefix + '_failed', int(status in failures))]
    timestamp = result.get('last_success_at')
    if timestamp:
        last_success = datetime.fromisoformat(timestamp.replace('Z', '+00:00'))
        if last_success.utcoffset() != timezone.utc.utcoffset(last_success):
            raise ValueError('invalid job timestamp')
        metrics.append((prefix + '_last_success_age_seconds', (now - last_success).total_seconds()))
    return metrics


def backup(path, now):
    return job_result(path, now, 'backup', ('ok', 'failed', 'unknown', 'skipped', 'running'),
                      ('failed', 'unknown'))


def deploy(path, now):
    result = read_result(path)
    metrics = job_metrics(result, now, 'deploy',
                      ('ok', 'failed', 'rolled_back', 'stopped', 'verified', 'skipped', 'bootstrapped', 'current'),
                      ('failed', 'rolled_back', 'stopped'))
    if result is not None:
        floor = result.get('release_floor')
        if floor is not None and (type(floor) is not int or floor < 1):
            raise ValueError('invalid release floor')
        metrics.append(('deploy_release_floor', floor or 0))
    return metrics


def release(path, now):
    result = read_result(path)
    if result is None:
        return []
    status = result['status']
    if status not in ('ok', 'failed', 'waiting', 'current', 'verified', 'skipped'):
        raise ValueError('invalid corpus release status')
    age = 0
    if status == 'waiting':
        value = result.get('waiting_since')
        if not isinstance(value, str):
            raise ValueError('missing corpus waiting timestamp')
        timestamp = datetime.fromisoformat(value.replace('Z', '+00:00'))
        if timestamp.utcoffset() != timezone.utc.utcoffset(timestamp):
            raise ValueError('invalid corpus waiting timestamp')
        age = max(0, (now-timestamp).total_seconds())
    integrity = result.get('integrity', [])
    if not isinstance(integrity, list) or not all(isinstance(code, str) for code in integrity):
        raise ValueError('invalid release integrity list')
    return [('release_failed', int(status == 'failed')), ('release_waiting_age_seconds', age),
            ('release_integrity', int(bool(integrity)))]


def watch_stale(path, now):
    try:
        value = read_result(path)
        if not isinstance(value, dict) or value.get('status') not in ('ok', 'failed'):
            return 1
        timestamp = value.get('last_success_at')
        if not isinstance(timestamp, str):
            return 1
        success = datetime.fromisoformat(timestamp.replace('Z', '+00:00'))
        if success.utcoffset() != timezone.utc.utcoffset(success):
            return 1
        return int((now-success).total_seconds() >= 3600)
    except (OSError, ValueError, TypeError, OverflowError, RecursionError):
        return 1


def collect(host, now):
    metrics = [('heartbeat', 1, {'host': host}),
               ('watch_stale', watch_stale(WATCH_RESULT, now), {'host': host})]
    for path in MOUNTS:
        used, free = filesystem(path)
        dimensions = {'host': host, 'mount': path}
        metrics.extend((('filesystem_used_percent', used, dimensions),
                        ('filesystem_free_bytes', free, dimensions)))
    metrics.append(('data_mounted', int(os.path.ismount(MOUNTS[1])), {'host': host}))
    for job in JOBS:
        for name, value in backup(BACKUP_DIR / (job + '.json'), now):
            metrics.append((name, value, {'host': host, 'job': job}))
    for name, value in deploy(DEPLOY_RESULT, now):
        metrics.append((name, value, {'host': host}))
    for kind, path in (('law', RELEASE_RESULT), ('frameworks', FRAMEWORKS_RESULT)):
        for name, value in release(path, now):
            metrics.append((name, value, {'host': host, 'kind': kind}))
    for name, value in job_result(SECRETS_RESULT, now, 'secrets',
                                  ('ok', 'current', 'failed', 'skipped'), ('failed',)):
        metrics.append((name, value, {'host': host}))
    return metrics


def post(client, compartment, metrics, now):
    models = oci.monitoring.models
    data = [models.MetricDataDetails(
        namespace='trachuan_host', compartment_id=compartment, name=name,
        dimensions=dimensions, datapoints=[models.Datapoint(timestamp=now, value=value)])
        for name, value, dimensions in metrics]
    response = client.post_metric_data(models.PostMetricDataDetails(metric_data=data),
                                       retry_strategy=oci.retry.NoneRetryStrategy())
    if response.data.failed_metrics_count:
        raise RuntimeError('metric ingestion failed')


def timeout_handler(_signum, _frame):
    raise TimeoutError('metrics timeout')


def main():
    signal.signal(signal.SIGALRM, timeout_handler)
    signal.alarm(20)
    request = Request('http://169.254.169.254/opc/v2/instance/',
                      headers={'Authorization': 'Bearer Oracle'})
    with urlopen(request, timeout=20) as response:
        metadata = json.load(response)
    signer = oci.auth.signers.InstancePrincipalsSecurityTokenSigner(
        retry_strategy=oci.retry.NoneRetryStrategy())
    client = oci.monitoring.MonitoringClient(
        config={}, signer=signer, timeout=(20, 20),
        service_endpoint='https://telemetry-ingestion.' + metadata['region'] + '.oraclecloud.com',
        retry_strategy=oci.retry.NoneRetryStrategy())
    now = datetime.now(timezone.utc)
    post(client, metadata['compartmentId'], collect(socket.gethostname(), now), now)


if __name__ == '__main__':
    try:
        import oci
        main()
    except Exception:
        print('metrics publication failed', file=sys.stderr)
        sys.exit(1)
