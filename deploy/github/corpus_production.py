import os
import subprocess

from corpus_manifest import decode
from corpus_registry import existing_release, production
from corpus_rules import require_status_country

STATUS_URL = 'https://trachuan.vn/api/v1/public/status'
STATUS_LIMIT = 1024**2


def require_compared_with(report, current):
    if current is None:
        raise ValueError('no verified production manifest')
    previous = report['previous_diff']['previous']
    if previous['rel'] != current['rel'] or previous['report_sha256'] != current['report']['sha256']:
        raise ValueError('report was not compared with the production release')


def status_response():
    args = ['curl', '-q', '--silent', '--show-error', '--noproxy', '*', '--proto', '=https',
            '--max-redirs', '0', '--max-time', '30', '--max-filesize', str(STATUS_LIMIT),
            '--header', 'Cache-Control: no-cache', '--output', '-',
            '--write-out', '%{http_code}', STATUS_URL]
    limit = STATUS_LIMIT + len('200')
    data = bytearray()
    with subprocess.Popen(args, stdout=subprocess.PIPE, env={'PATH': os.environ['PATH']}) as process:
        try:
            while chunk := process.stdout.read(min(65536, limit + 1 - len(data))):
                data.extend(chunk)
                if len(data) > limit:
                    raise ValueError('public status exceeds its size limit')
            if process.wait() != 0:
                raise subprocess.CalledProcessError(process.returncode, args)
        finally:
            if process.poll() is None:
                process.kill()
                process.wait()
    return bytes(data[-3:]), bytes(data[:-3])


def served_release():
    code, body = status_response()
    if code != b'200':
        raise ValueError('public status did not answer 200')
    status = decode(body)
    if not isinstance(status, dict) or not isinstance(status.get('release'), str) or not status['release']:
        raise ValueError('invalid public status')
    return status['release']


def require_settled(report, directory):
    country = report['previous_diff']['previous']['country']
    require_status_country(country)
    current = production(country, directory / 'production')
    require_compared_with(report, current)
    if served_release() != current['rel']:
        raise ValueError('served release differs from the production manifest')
    return current


def require_signable(country, rel, report, directory):
    current = production(country, directory)
    if current is None:
        raise ValueError('no verified production manifest')
    existing = existing_release(country, rel, directory)
    if existing is None or existing['sequence'] > current['sequence']:
        require_compared_with(report, current)
    return current
