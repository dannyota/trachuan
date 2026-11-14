import argparse
import json
import os
from pathlib import Path
import subprocess
import tempfile

from corpus_manifest import identity
from manifest import positive


def etag(result):
    value = result.get('ETag')
    if not isinstance(value, str) or not value:
        raise ValueError('upload did not return an ETag')
    return value


def upload_file(path, key, client, part_size=64 * 1024**2):
    size = path.stat().st_size
    positive(size, 'upload size')
    common = ['--bucket', 'trachuan-releases', '--key', key]
    if size <= part_size:
        result = client(['put-object', *common, '--body', str(path), '--content-length', str(size)])
        return dict(ContentLength=size, ETag=etag(result))
    upload_id = client(['create-multipart-upload', *common]).get('UploadId')
    if not isinstance(upload_id, str) or not upload_id:
        raise ValueError('multipart upload did not return an upload id')
    multipart = [*common, '--upload-id', upload_id]
    parts, transferred = [], 0
    try:
        with tempfile.TemporaryDirectory() as temporary, path.open('rb') as source:
            body = Path(temporary) / 'part'
            while chunk := source.read(part_size):
                body.write_bytes(chunk)
                number = len(parts) + 1
                result = client(['upload-part', *multipart, '--part-number', str(number),
                                 '--body', str(body), '--content-length', str(len(chunk))])
                parts.append(dict(PartNumber=number, ETag=etag(result)))
                transferred += len(chunk)
        if transferred != size:
            raise ValueError('local upload size changed')
        result = client(['complete-multipart-upload', *multipart,
                         '--multipart-upload', json.dumps(dict(Parts=parts))])
        return dict(ContentLength=size, ETag=etag(result))
    except BaseException:
        try:
            client(['abort-multipart-upload', *multipart])
        except (subprocess.CalledProcessError, ValueError):
            pass
        raise


def upload(country, rel, directory, report):
    identity(country, rel)
    command = ['aws', '--profile', 'trachuan-release-upload', '--endpoint-url',
               os.environ['OCI_S3_ENDPOINT'], '--no-cli-pager', 's3api']

    def client(args):
        result = subprocess.run([*command, *args, '--output', 'json'],
                                check=True, capture_output=True, text=True)
        return json.loads(result.stdout)

    receipts = {}
    for name in ('corpus.dump', 'MANIFEST.sha256', 'report.json', 'release.json'):
        source = report if name == 'report.json' else directory / name
        key = f'{country}/{rel}/{name}'
        receipts[key] = upload_file(source, key, client)
    for key, receipt in receipts.items():
        current = client(['head-object', '--bucket', 'trachuan-releases', '--key', key])
        if type(current.get('ContentLength')) is not int or current['ContentLength'] != receipt['ContentLength'] or current.get('ETag') != receipt['ETag']:
            raise ValueError('uploaded object size or ETag changed: ' + key.rsplit('/', 1)[1])


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('country')
    parser.add_argument('rel')
    parser.add_argument('directory', type=Path)
    parser.add_argument('report', type=Path)
    args = parser.parse_args()
    upload(args.country, args.rel, args.directory, args.report)


if __name__ == '__main__':
    main()
