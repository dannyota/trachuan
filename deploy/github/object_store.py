import json
import os
from pathlib import Path
import shutil
import subprocess

from manifest import positive

CONFIG = ('[default]\nregion = ap-singapore-1\ns3 =\n'
          '    addressing_style = path\n    signature_version = s3v4\n')


def s3_endpoint():
    endpoint = os.environ['OCI_S3_ENDPOINT']
    if not endpoint.startswith('https://') or '@' in endpoint:
        raise ValueError('OCI_S3_ENDPOINT must be HTTPS without credentials')
    return endpoint


def require_pinned_cli():
    version = subprocess.run(['aws', '--version'], check=True, capture_output=True, text=True).stdout
    if version.split(maxsplit=1)[0:1] != ['aws-cli/2.40.0']:
        raise ValueError('AWS CLI 2.40.0 is required')
    print(version.strip(), flush=True)


class ObjectReader:
    def __init__(self, bucket, endpoint, directory):
        config = Path(directory) / 'config'
        config.write_text(CONFIG)
        self.bucket = bucket
        self.env = dict(os.environ, AWS_CONFIG_FILE=str(config), AWS_REQUEST_CHECKSUM_CALCULATION='when_required',
                        AWS_DEFAULT_REGION='ap-singapore-1', AWS_REGION='ap-singapore-1')
        self.command = ['aws', '--endpoint-url', endpoint, '--no-cli-pager']

    def head(self, key, name, limit):
        args = [*self.command, 's3api', 'head-object', '--bucket', self.bucket, '--key', key, '--output', 'json']
        result = subprocess.run(args, check=True, capture_output=True, text=True, env=self.env)
        metadata = json.loads(result.stdout)
        expected = metadata['ContentLength']
        positive(expected, 'object size')
        if expected > limit:
            raise ValueError(f'object exceeds download limit: {name}')
        etag = metadata.get('ETag')
        if not isinstance(etag, str) or not etag:
            raise ValueError(f'HEAD did not return an ETag: {name}')
        return expected, etag

    def get(self, key, name, path, expected, etag):
        result = subprocess.run([*self.command, 's3api', 'get-object', '--bucket', self.bucket,
                                 '--key', key, '--range', f'bytes=0-{expected - 1}',
                                 '--if-match', etag, '--output', 'json', str(path)],
                                check=True, stdout=subprocess.PIPE, text=True, env=self.env)
        if path.stat().st_size != expected:
            raise ValueError(f'downloaded size differs: {name}')
        metadata = json.loads(result.stdout)
        if metadata.get('ETag') != etag or metadata.get('ContentRange') != f'bytes 0-{expected - 1}/{expected}':
            raise ValueError(f'GET object identity or full size differs from HEAD: {name}')


def require_free_space(objects, directory, label):
    if sum(size for size, _ in objects.values()) >= shutil.disk_usage(directory).free - 2 * 1024**3:
        raise ValueError(f'{label} objects exceed free disk minus 2 GiB')


def fetch_checked(client, bucket, key, path, limit, label):
    common = ['--bucket', bucket, '--key', key]
    metadata = client(['head-object', *common])
    size, etag = metadata.get('ContentLength'), metadata.get('ETag')
    positive(size, label + ' size')
    if size > limit or not isinstance(etag, str) or not etag:
        raise ValueError(f'invalid {label} size or ETag')
    result = client(['get-object', *common, '--range', f'bytes=0-{size - 1}',
                     '--if-match', etag, str(path)])
    data = path.read_bytes()
    if len(data) != size or result.get('ETag') != etag or result.get('ContentRange') != f'bytes 0-{size - 1}/{size}':
        raise ValueError(f'{label} download identity or size differs')
    return data
