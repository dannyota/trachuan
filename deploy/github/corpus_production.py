import argparse
import os
from pathlib import Path
import subprocess
import sys
import tempfile

from corpus_manifest import decode, manifest_fields, validate_manifest
from corpus_registry import existing_release, production
from corpus_rules import first_signing_allowed, refuse_until_sidecar_gate, require_status_country
from manifest import canonical

STATUS_URL = 'https://trachuan.vn/api/v1/public/status'
STATUS_LIMIT = 1024**2
RESIGN_MAY_CHANGE = {'commit', 'sequence', 'heading_review'}
RESIGN_DRIFT_EXIT = 3


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


def changed_fields(signed, rebuilt):
    return sorted(field for field, value in signed.items() if field not in RESIGN_MAY_CHANGE
                  and (field not in rebuilt or canonical(rebuilt[field]) != canonical(value)))


def require_signable(manifest, report, run, directory):
    country, rel = manifest['country'], manifest['rel']
    current = production(country, directory)
    if current is None:
        raise ValueError('no verified production manifest')
    existing = existing_release(country, rel, directory)
    if existing is None:
        if not first_signing_allowed(country, rel, run):
            raise ValueError('a release without a decision below the cutoff needs a signed manifest')
    else:
        changed = changed_fields(existing, manifest)
        if changed:
            raise ValueError('signing again would change signed fields: ' + ', '.join(changed))
    if existing is None or existing['sequence'] > current['sequence']:
        require_compared_with(report, current)
    return current


def resign_differences(country, rel, directory, temporary):
    signed = existing_release(country, rel, temporary)
    if signed is None:
        raise ValueError(f'no signed manifest for {country}-{rel}')
    rebuilt = validate_manifest(manifest_fields(signed['commit'], signed['sequence'], country, rel,
                                                directory, directory / 'report.json'))
    refuse_until_sidecar_gate(decode((directory / 'release.json').read_bytes()))
    return [(field, canonical(signed[field]).decode(),
             canonical(rebuilt[field]).decode() if field in rebuilt else 'absent')
            for field in changed_fields(signed, rebuilt)]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('command', choices=['resign-check'])
    parser.add_argument('country')
    parser.add_argument('rel')
    parser.add_argument('directory', type=Path)
    args = parser.parse_args()
    with tempfile.TemporaryDirectory() as temporary:
        differences = resign_differences(args.country, args.rel, args.directory, Path(temporary))
    for field, signed, rebuilt in differences:
        print(f'{field}: signed {signed} rebuilt {rebuilt}')
    if differences:
        sys.exit(RESIGN_DRIFT_EXIT)
    print('no signed field would change')


if __name__ == '__main__':
    main()
