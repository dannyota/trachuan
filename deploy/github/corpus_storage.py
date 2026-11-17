import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

from corpus_manifest import decode, identity
from manifest import positive


def download(country, rel, directory):
    identity(country, rel)
    endpoint = os.environ['OCI_S3_ENDPOINT']
    if not endpoint.startswith('https://') or '@' in endpoint:
        raise ValueError('OCI_S3_ENDPOINT must be HTTPS without credentials')
    version = subprocess.run(['aws', '--version'], check=True, capture_output=True, text=True).stdout
    if version.split(maxsplit=1)[0:1] != ['aws-cli/2.40.0']:
        raise ValueError('AWS CLI 2.40.0 is required')
    print(version.strip(), flush=True)
    directory.mkdir(parents=True, exist_ok=False)
    with tempfile.TemporaryDirectory() as temporary:
        config = Path(temporary) / 'config'
        config.write_text('[default]\nregion = ap-singapore-1\ns3 =\n'
                          '    addressing_style = path\n    signature_version = s3v4\n')
        env = dict(os.environ, AWS_CONFIG_FILE=str(config), AWS_REQUEST_CHECKSUM_CALCULATION='when_required',
                   AWS_DEFAULT_REGION='ap-singapore-1', AWS_REGION='ap-singapore-1')
        command = ['aws', '--endpoint-url', endpoint, '--no-cli-pager']
        limits = {'corpus.dump': 16 * 1024**3, 'MANIFEST.sha256': 1024**2,
                  'report.json': 16 * 1024**2, 'release.json': 16 * 1024**2,
                  'build.json': 16 * 1024**2}
        objects = {}
        for name, limit in limits.items():
            key = f'{country}/{rel}/{name}'
            head = [*command, 's3api', 'head-object', '--bucket', 'trachuan-releases',
                    '--key', key, '--output', 'json']
            try:
                result = subprocess.run(head, check=True, capture_output=True, text=True, env=env)
            except subprocess.CalledProcessError:
                if name == 'build.json':
                    continue
                raise
            metadata = json.loads(result.stdout)
            expected = metadata['ContentLength']
            positive(expected, 'object size')
            if expected > limit:
                raise ValueError(f'object exceeds download limit: {name}')
            etag = metadata.get('ETag')
            if not isinstance(etag, str) or not etag:
                raise ValueError(f'HEAD did not return an ETag: {name}')
            objects[name] = (expected, etag)
        if sum(size for size, _ in objects.values()) >= shutil.disk_usage(directory).free - 2 * 1024**3:
            raise ValueError('corpus objects exceed free disk minus 2 GiB')

        def fetch(name):
            expected, etag = objects[name]
            key = f'{country}/{rel}/{name}'
            path = directory / name
            result = subprocess.run([*command, 's3api', 'get-object', '--bucket', 'trachuan-releases',
                                     '--key', key, '--range', f'bytes=0-{expected - 1}',
                                     '--if-match', etag, '--output', 'json', str(path)],
                                    check=True, stdout=subprocess.PIPE, text=True, env=env)
            if path.stat().st_size != expected:
                raise ValueError(f'downloaded size differs: {name}')
            metadata = json.loads(result.stdout)
            if metadata.get('ETag') != etag or metadata.get('ContentRange') != f'bytes 0-{expected - 1}/{expected}':
                raise ValueError(f'GET object identity or full size differs from HEAD: {name}')

        fetch('release.json')
        row = decode((directory / 'release.json').read_bytes())
        if not isinstance(row, dict) or row.get('country') != country or row.get('rel') != rel:
            raise ValueError('downloaded release identity differs from request')
        positive(row.get('schema_version'), 'schema version')
        if row['schema_version'] >= 8 and 'build.json' not in objects:
            raise ValueError('schema 8 requires build.json HEAD metadata')
        for name in objects:
            if name == 'release.json' or (name == 'build.json' and row['schema_version'] < 8):
                continue
            fetch(name)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('country')
    parser.add_argument('rel')
    parser.add_argument('directory', type=Path)
    args = parser.parse_args()
    download(args.country, args.rel, args.directory)


if __name__ == '__main__':
    main()
