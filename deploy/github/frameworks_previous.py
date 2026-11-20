import hashlib

from frameworks_registry import production, release
from frameworks_release import BUCKET, LIMITS, object_key
from object_store import fetch_checked


def require_active(report, directory, client, repository):
    named = report['previous']
    current = production(repository, directory / 'manifest')
    if current is None:
        if named is not None:
            raise ValueError('report names a previous release but none is published')
        return
    if named is None:
        raise ValueError('report must name the active frameworks release')
    if named['rel'] != current['rel'] or named['report_sha256'] != current['report']['sha256']:
        raise ValueError('report previous differs from the active frameworks release')
    directory.mkdir(parents=True, exist_ok=True)
    data = fetch_checked(client, BUCKET, object_key(current['rel'], 'report.json'),
                         directory / 'previous-report.json', LIMITS['report.json'], 'previous report')
    if hashlib.sha256(data).hexdigest() != named['report_sha256']:
        raise ValueError('previous report SHA-256 differs from report')


def require_signed(report, directory, repository):
    named = report['previous']
    if named is None:
        return
    if release(repository, named['rel'], directory)['report']['sha256'] != named['report_sha256']:
        raise ValueError('previous report SHA-256 differs from its signed manifest')
