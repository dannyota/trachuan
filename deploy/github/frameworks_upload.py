import argparse
import json
import os
import subprocess
import tempfile

from corpus_upload import upload_file, verify_receipts
from frameworks_cli import guarded
from frameworks_manifest import document
from frameworks_paths import private_directory, release_directory, source_file
from frameworks_previous import require_active
from frameworks_release import BUCKET, object_key, release_id, validate_release
from object_store import s3_endpoint

SOURCES = {'frameworks.dump': 'build/frameworks.dump', 'build.json': 'build/build.json',
           'evaluate.json': 'evaluate/manifest.json', 'report.json': 'evaluate/report.json',
           'release.json': 'release/release.json'}
PROFILE = 'trachuan-release-upload'
CONFIG = (f'[profile {PROFILE}]\nregion = ap-singapore-1\ns3 =\n'
          '    addressing_style = path\n    signature_version = s3v4\n')


def print_plan(rel):
    prefix = f's3://{BUCKET}/frameworks/{rel}/'
    print('Verify frameworks-production signed manifest and previous report SHA-256 before upload')
    print(f'Check empty prefix {prefix} with profile {PROFILE}')
    for name in SOURCES:
        print(f'Upload and verify size and ETag: {prefix}{name}')
    print(f'gh workflow run frameworks-release.yml --ref main -f rel={rel}')


def confine(directory):
    os.environ['TMPDIR'] = str(directory)
    tempfile.tempdir = str(directory)
    config = directory / 'aws-config'
    config.write_text(CONFIG)
    os.environ['AWS_CONFIG_FILE'] = str(config)
    os.environ['AWS_REQUEST_CHECKSUM_CALCULATION'] = 'when_required'


def require_cli():
    version = subprocess.run(['aws', '--version'], check=True, capture_output=True, text=True).stdout.strip()
    if not version.startswith('aws-cli/2.'):
        raise ValueError('AWS CLI v2 is required')
    print(version, flush=True)


def s3_command(endpoint):
    return ['aws', '--profile', PROFILE, '--endpoint-url', endpoint, '--no-cli-pager', 's3api']


def require_empty_prefix(command, rel):
    result = subprocess.run([*command, 'list-objects-v2', '--bucket', BUCKET, '--prefix', f'frameworks/{rel}/',
                             '--max-keys', '1', '--query', 'KeyCount', '--output', 'text'],
                            check=True, stdout=subprocess.PIPE, text=True)
    if result.stdout.strip() != '0':
        raise ValueError('Release prefix already holds objects; choose a new rel')


def upload(rel, base, paths, facts, endpoint):
    os.umask(0o077)
    with private_directory(base) as directory:
        confine(directory)
        require_cli()
        command = s3_command(endpoint)

        def client(args):
            result = subprocess.run([*command, '--output', 'json', *args], check=True, capture_output=True, text=True)
            return json.loads(result.stdout)

        require_empty_prefix(command, rel)
        require_active(facts.report, directory / 'previous', client)
        receipts = {object_key(rel, name): upload_file(path, object_key(rel, name), client, bucket=BUCKET)
                    for name, path in paths.items()}
        verify_receipts(receipts, client, bucket=BUCKET)
        subprocess.run(['gh', 'workflow', 'run', 'frameworks-release.yml', '--ref', 'main', '-f', f'rel={rel}'], check=True)


def run(rel, dry_run):
    release_id(rel)
    endpoint = s3_endpoint()
    base = release_directory(rel)
    paths = {name: source_file(base, relative) for name, relative in SOURCES.items()}
    facts = validate_release(rel, paths)
    document('0' * 40, 1, rel, facts)
    if dry_run:
        print_plan(rel)
    else:
        upload(rel, base, paths, facts, endpoint)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--dry-run', action='store_true')
    parser.add_argument('rel')
    args = parser.parse_args()
    guarded(lambda: run(args.rel, args.dry_run))


if __name__ == '__main__':
    main()
