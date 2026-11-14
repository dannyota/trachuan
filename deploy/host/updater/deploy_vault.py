import base64
import contextlib
import io
import logging
import re
import sys


def names():
    return ('google_client_secret', 'smtp_username', 'smtp_password', 'account_keys',
            'postgres_password', 'auth_db_password', 'corpus_reader_password',
            'activator_db_password', 'backup_s3_key', 'backup_s3_key_secret',
            'server_signup_emails')


def fetch(sdk, region, vault, name):
    signer = sdk.auth.signers.InstancePrincipalsSecurityTokenSigner(
        retry_strategy=sdk.retry.NoneRetryStrategy())
    client = sdk.secrets.SecretsClient(
        config={'region': region}, signer=signer, timeout=(10, 20),
        retry_strategy=sdk.retry.NoneRetryStrategy())
    data = client.get_secret_bundle_by_name(
        secret_name=name, vault_id=vault,
        retry_strategy=sdk.retry.NoneRetryStrategy()).data
    if type(data.version_number) is not int or data.version_number < 1:
        raise ValueError('invalid bundle version')
    content = data.secret_bundle_content
    if content.content_type != 'BASE64':
        raise ValueError('invalid bundle content type')
    value = base64.b64decode(content.content, validate=True)
    if not value or len(value) > 512000:
        raise ValueError('invalid bundle size')
    return name.encode() + b'\0' + str(data.version_number).encode() + b'\0' + base64.b64encode(value) + b'\0'


def main():
    logging.disable(logging.CRITICAL)
    try:
        with contextlib.redirect_stdout(io.StringIO()):
            import oci
            region, vault, name = sys.argv[1:]
            if name not in names():
                raise ValueError('invalid secret name')
            record = fetch(oci, region, vault, name)
    except Exception as error:
        code = getattr(error, 'code', 'SDKError')
        if not isinstance(code, str) or not re.fullmatch('[A-Za-z][A-Za-z0-9_]{0,79}', code):
            code = 'SDKError'
        sys.stderr.write('OCI_ERROR:' + code + '\n')
        return 1
    sys.stdout.buffer.write(record)
    return 0


if __name__ == '__main__':
    sys.exit(main())
