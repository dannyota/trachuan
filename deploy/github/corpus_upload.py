import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile

from corpus_manifest import identity, release_files, validate_inputs
from corpus_previous import verified_previous
from corpus_production import require_settled
from manifest import positive


def etag(result):
    value = result.get('ETag')
    if not isinstance(value, str) or not value:
        raise ValueError('upload did not return an ETag')
    return value


def require_validated(digest, size, expected, key):
    if digest != expected[0] or size != expected[1]:
        raise ValueError('local file changed after validation: ' + key.rsplit('/', 1)[1])


def upload_file(path, key, client, part_size=64 * 1024**2, bucket='trachuan-releases', expected=None):
    size = path.stat().st_size
    positive(size, 'upload size')
    common = ['--bucket', bucket, '--key', key]
    if expected is not None and size != expected[1]:
        raise ValueError('local file changed after validation: ' + key.rsplit('/', 1)[1])

    def put(body):
        return dict(ContentLength=size, ETag=etag(client(['put-object', *common, '--body', str(body),
                                                          '--content-length', str(size)])))

    if size <= part_size:
        if expected is None:
            return put(path)
        data = path.read_bytes()
        require_validated(hashlib.sha256(data).hexdigest(), len(data), expected, key)
        with tempfile.TemporaryDirectory() as temporary:
            body = Path(temporary) / 'body'
            body.write_bytes(data)
            return put(body)
    upload_id = client(['create-multipart-upload', *common]).get('UploadId')
    if not isinstance(upload_id, str) or not upload_id:
        raise ValueError('multipart upload did not return an upload id')
    multipart = [*common, '--upload-id', upload_id]
    parts, transferred, stream = [], 0, hashlib.sha256()
    try:
        with tempfile.TemporaryDirectory() as temporary, path.open('rb') as source:
            body = Path(temporary) / 'part'
            while chunk := source.read(part_size):
                stream.update(chunk)
                body.write_bytes(chunk)
                number = len(parts) + 1
                result = client(['upload-part', *multipart, '--part-number', str(number),
                                 '--body', str(body), '--content-length', str(len(chunk))])
                parts.append(dict(PartNumber=number, ETag=etag(result)))
                transferred += len(chunk)
        if transferred != size:
            raise ValueError('local upload size changed')
        if expected is not None:
            require_validated(stream.hexdigest(), transferred, expected, key)
        result = client(['complete-multipart-upload', *multipart,
                         '--multipart-upload', json.dumps(dict(Parts=parts))])
        return dict(ContentLength=size, ETag=etag(result))
    except BaseException:
        try:
            client(['abort-multipart-upload', *multipart])
        except (subprocess.CalledProcessError, ValueError):
            pass
        raise


def verify_receipts(receipts, client, bucket='trachuan-releases'):
    for key, receipt in receipts.items():
        current = client(['head-object', '--bucket', bucket, '--key', key])
        if type(current.get('ContentLength')) is not int or current['ContentLength'] != receipt['ContentLength'] or current.get('ETag') != receipt['ETag']:
            raise ValueError('uploaded object size or ETag changed: ' + key.rsplit('/', 1)[1])


def upload(country, rel, directory, report):
    identity(country, rel)
    command = ['aws', '--profile', 'trachuan-release-upload', '--endpoint-url',
               os.environ['OCI_S3_ENDPOINT'], '--no-cli-pager', 's3api']

    def client(args):
        result = subprocess.run([*command, '--output', 'json', *args],
                                check=True, capture_output=True, text=True)
        return json.loads(result.stdout)

    row, candidate = validate_inputs(country, rel, directory, report)
    with tempfile.TemporaryDirectory() as temporary:
        require_settled(candidate, Path(temporary))
        verified_previous(candidate, Path(temporary), client, release_row=row)

    receipts = {}
    for name in release_files(row['schema_version']):
        source = report if name == 'report.json' else directory / name
        key = f'{country}/{rel}/{name}'
        receipts[key] = upload_file(source, key, client)
    verify_receipts(receipts, client)


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
