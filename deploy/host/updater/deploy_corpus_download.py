import contextlib
import hashlib
import io
import json
import logging
import os
from pathlib import Path
import re
import sys


def stream(source, target, metadata):
    target = Path(target)
    pending = target.with_suffix('.partial')
    digest, size = hashlib.sha256(), 0
    try:
        with pending.open('xb') as output:
            os.chmod(pending, 0o600)
            while chunk := source.read(min(1024 * 1024, metadata['size'] + 1 - size)):
                size += len(chunk)
                if size > metadata['size']:
                    raise ValueError('dump size overflow')
                digest.update(chunk)
                output.write(chunk)
            if size != metadata['size'] or digest.hexdigest() != metadata['sha256']:
                raise ValueError('dump size or hash mismatch')
            output.flush()
            os.fsync(output.fileno())
        os.replace(pending, target)
    finally:
        pending.unlink(missing_ok=True)


def fetch(sdk, region, namespace, bucket, metadata, target):
    signer = sdk.auth.signers.InstancePrincipalsSecurityTokenSigner(
        retry_strategy=sdk.retry.NoneRetryStrategy(),
        federation_client_retry_strategy=sdk.retry.NoneRetryStrategy())
    client = sdk.object_storage.ObjectStorageClient(
        config={'region': region}, signer=signer, timeout=(10, 60),
        retry_strategy=sdk.retry.NoneRetryStrategy())
    response = client.get_object(namespace, bucket, metadata['object'],
                                 retry_strategy=sdk.retry.NoneRetryStrategy())
    with contextlib.closing(response.data.raw) as source:
        stream(source, target, metadata)


def main():
    logging.disable(logging.CRITICAL)
    try:
        with contextlib.redirect_stdout(io.StringIO()):
            import oci
            region, namespace, bucket, target = sys.argv[1:]
            metadata = json.load(sys.stdin)
            fetch(oci, region, namespace, bucket, metadata, target)
    except Exception as error:
        code = getattr(error, 'code', 'DownloadError')
        if not isinstance(code, str) or not re.fullmatch('[A-Za-z][A-Za-z0-9_]{0,79}', code):
            code = 'DownloadError'
        sys.stderr.write('OCI_ERROR:' + code + '\n')
        return 1
    return 0


if __name__ == '__main__':
    sys.exit(main())
