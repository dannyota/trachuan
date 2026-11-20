import argparse
import os
from pathlib import Path
import subprocess
import sys
import tempfile

from corpus_dispatch import dispatch_command
from corpus_manifest import decode, identity, manifest_fields, validate_manifest
from corpus_registry import existing_release, production
from corpus_rules import first_signing_allowed, require_status_country
from manifest import canonical
from repositories import workflow

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


def require_settled(report, directory, repository):
    country = report['previous_diff']['previous']['country']
    require_status_country(country)
    current = production(repository, country, directory / 'production')
    require_compared_with(report, current)
    if served_release() != current['rel']:
        raise ValueError('served release differs from the production manifest')
    return current


def changed_fields(signed, rebuilt):
    return sorted(field for field in signed.keys() | rebuilt.keys() if field not in RESIGN_MAY_CHANGE
                  and (field not in signed or field not in rebuilt or canonical(rebuilt[field]) != canonical(signed[field])))


def require_signable(manifest, report, run, directory, repository, resign=False):
    country, rel = manifest['country'], manifest['rel']
    current = production(repository, country, directory)
    if current is None:
        raise ValueError('no verified production manifest')
    existing = existing_release(repository, country, rel, directory)
    if existing is None:
        if resign:
            raise ValueError(f'an empty release SHA-256 signs only an already signed {country}-{rel}')
        if not first_signing_allowed(country, rel, run):
            raise ValueError('a release without a decision below the cutoff needs a signed manifest')
    else:
        changed = changed_fields(existing, manifest)
        if changed:
            raise ValueError('signing again would change signed fields: ' + ', '.join(changed))
    if existing is None or existing['sequence'] > current['sequence']:
        require_compared_with(report, current)
    return current


def signed_release(repository, country, rel, temporary):
    signed = existing_release(repository, country, rel, temporary)
    if signed is None:
        raise ValueError(f'no signed manifest for {country}-{rel}')
    return signed


def shown(manifest, field):
    return canonical(manifest[field]).decode() if field in manifest else 'absent'


def resign_differences(country, rel, directory, temporary, repository):
    signed = signed_release(repository, country, rel, temporary)
    rebuilt = validate_manifest(manifest_fields(repository, signed['commit'], signed['sequence'], country, rel,
                                                directory, directory / 'report.json'))
    return signed, [(field, shown(signed, field), shown(rebuilt, field)) for field in changed_fields(signed, rebuilt)]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('command', choices=['resign-check'])
    parser.add_argument('country')
    parser.add_argument('rel')
    parser.add_argument('directory', type=Path, nargs='?')
    parser.add_argument('--repository')
    args = parser.parse_args()
    identity(args.country, args.rel)
    repository = workflow(args.repository)
    with tempfile.TemporaryDirectory() as temporary:
        if args.directory is None:
            signed, differences = signed_release(repository, args.country, args.rel, Path(temporary)), []
        else:
            signed, differences = resign_differences(args.country, args.rel, args.directory, Path(temporary),
                                                     repository)
    for field, old, new in differences:
        print(f'{field}: signed {old} rebuilt {new}')
    if args.directory is not None and not differences:
        print('no signed field would change')
    print(' '.join(dispatch_command(repository, args.country, args.rel, signed['report']['sha256'], '')))
    if differences:
        sys.exit(RESIGN_DRIFT_EXIT)


if __name__ == '__main__':
    main()
