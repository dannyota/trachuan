import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

from corpus_manifest import identity
from manifest import positive


def download(country, rel, directory):
    identity(country, rel)
    endpoint = os.environ['OCI_S3_ENDPOINT']
    if not endpoint.startswith('https://') or '@' in endpoint:
        raise ValueError('OCI_S3_ENDPOINT must be HTTPS without credentials')
    version = subprocess.run(['aws', '--version'], check=True, capture_output=True, text=True).stdout
    if not version.startswith('aws-cli/2.'):
        raise ValueError('AWS CLI v2 is required')
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
                  'report.json': 16 * 1024**2, 'release.json': 16 * 1024**2}
        sizes = {}
        for name, limit in limits.items():
            key = f'{country}/{rel}/{name}'
            head = [*command, 's3api', 'head-object', '--bucket', 'trachuan-releases',
                    '--key', key, '--output', 'json']
            result = subprocess.run(head, check=True, capture_output=True, text=True, env=env)
            expected = json.loads(result.stdout)['ContentLength']
            positive(expected, 'object size')
            if expected > limit:
                raise ValueError(f'object exceeds download limit: {name}')
            sizes[name] = expected
        if sum(sizes.values()) >= shutil.disk_usage(directory).free - 2 * 1024**3:
            raise ValueError('corpus objects exceed free disk minus 2 GiB')
        for name, expected in sizes.items():
            key = f'{country}/{rel}/{name}'
            path = directory / name
            subprocess.run([*command, 's3api', 'get-object', '--bucket', 'trachuan-releases',
                            '--key', key, '--range', f'bytes=0-{expected - 1}', str(path)],
                           check=True, stdout=subprocess.PIPE, env=env)
            if path.stat().st_size != expected:
                raise ValueError(f'downloaded size differs: {name}')


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('country')
    parser.add_argument('rel')
    parser.add_argument('directory', type=Path)
    args = parser.parse_args()
    download(args.country, args.rel, args.directory)


if __name__ == '__main__':
    main()
