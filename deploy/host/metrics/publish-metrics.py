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
JOBS = ('full', 'diff', 'verify', 'health', 'restore-check')
MOUNTS = ('/', '/srv/trachuan/data')
MAX_RESULT_BYTES = 65536


def filesystem(path):
    stat = os.statvfs(path)
    used = stat.f_blocks - stat.f_bfree
    return used / (used + stat.f_bavail) * 100, stat.f_bavail * stat.f_frsize


def backup(path, now):
    try:
        fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    except FileNotFoundError:
        return []
    with os.fdopen(fd, 'rb') as source:
        if not stat.S_ISREG(os.fstat(source.fileno()).st_mode):
            raise ValueError('backup result is not a regular file')
        raw = source.read(MAX_RESULT_BYTES + 1)
    if len(raw) > MAX_RESULT_BYTES:
        raise ValueError('backup result too large')
    result = json.loads(raw.decode('utf-8'))
    status = result['status']
    if status not in ('ok', 'failed', 'unknown', 'skipped', 'running'):
        raise ValueError('invalid backup status')
    metrics = [('backup_failed', int(status in ('failed', 'unknown')))]
    timestamp = result.get('last_success_at')
    if timestamp:
        last_success = datetime.fromisoformat(timestamp.replace('Z', '+00:00'))
        if last_success.utcoffset() != timezone.utc.utcoffset(last_success):
            raise ValueError('invalid backup timestamp')
        metrics.append(('backup_last_success_age_seconds', (now - last_success).total_seconds()))
    return metrics


def collect(host, now):
    metrics = [('heartbeat', 1, {'host': host})]
    for path in MOUNTS:
        used, free = filesystem(path)
        dimensions = {'host': host, 'mount': path}
        metrics.extend((('filesystem_used_percent', used, dimensions),
                        ('filesystem_free_bytes', free, dimensions)))
    metrics.append(('data_mounted', int(os.path.ismount(MOUNTS[1])), {'host': host}))
    for job in JOBS:
        for name, value in backup(BACKUP_DIR / (job + '.json'), now):
            metrics.append((name, value, {'host': host, 'job': job}))
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
