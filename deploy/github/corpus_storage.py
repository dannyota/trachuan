import argparse
import os
from pathlib import Path
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
    directory.mkdir(parents=True, exist_ok=False)
    with tempfile.TemporaryDirectory() as temporary:
        config = Path(temporary) / 'config'
        config.write_text('[default]\nregion = ap-singapore-1\ns3 =\n'
                          '    addressing_style = path\n    signature_version = s3v4\n')
        env = dict(os.environ, AWS_CONFIG_FILE=str(config), AWS_REQUEST_CHECKSUM_CALCULATION='when_required',
                   AWS_DEFAULT_REGION='ap-singapore-1', AWS_REGION='ap-singapore-1')
        command = ['aws', '--endpoint-url', endpoint, '--no-cli-pager']
        for name in ('corpus.dump', 'MANIFEST.sha256', 'report.json', 'release.json'):
            key = f'{country}/{rel}/{name}'
            head = [*command, 's3api', 'head-object', '--bucket', 'trachuan-releases',
                    '--key', key, '--query', 'ContentLength', '--output', 'text']
            expected = int(subprocess.run(head, check=True, capture_output=True, text=True, env=env).stdout.strip())
            positive(expected, 'object size')
            path = directory / name
            subprocess.run([*command, 's3', 'cp', 's3://trachuan-releases/' + key,
                            str(path), '--only-show-errors'], check=True, env=env)
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
