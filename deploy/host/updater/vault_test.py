import base64
import contextlib
import io
from types import SimpleNamespace
import unittest
from unittest import mock

from updater import deploy_vault as vault


class VaultTest(unittest.TestCase):
    def sdk(self):
        sdk = mock.Mock()
        sdk.secrets.SecretsClient.return_value.get_secret_bundle_by_name.return_value.data = SimpleNamespace(
            version_number=7, secret_bundle_content=SimpleNamespace(
                content_type='BASE64', content=base64.b64encode(b'synthetic-vault-value').decode()))
        return sdk

    def test_current_bundle_uses_instance_principal_region_and_named_vault(self):
        sdk = self.sdk()
        self.assertEqual(vault.fetch(sdk, 'ap-singapore-1', 'ocid1.vault.synthetic', 'smtp_password'),
                         b'smtp_password\x007\x00' + base64.b64encode(b'synthetic-vault-value') + b'\x00')
        sdk.auth.signers.InstancePrincipalsSecurityTokenSigner.assert_called_once_with(
            retry_strategy=sdk.retry.NoneRetryStrategy.return_value)
        sdk.secrets.SecretsClient.assert_called_once_with(
            config={'region': 'ap-singapore-1'}, signer=sdk.auth.signers.InstancePrincipalsSecurityTokenSigner.return_value,
            timeout=(10, 20), retry_strategy=sdk.retry.NoneRetryStrategy.return_value)
        sdk.secrets.SecretsClient.return_value.get_secret_bundle_by_name.assert_called_once_with(
            secret_name='smtp_password', vault_id='ocid1.vault.synthetic',
            retry_strategy=sdk.retry.NoneRetryStrategy.return_value)

    def test_current_version_bytes_are_exact_and_no_older_version_is_read(self):
        for value in (b'synthetic-exact-value', b'synthetic-account-key\n'):
            sdk = self.sdk()
            data = sdk.secrets.SecretsClient.return_value.get_secret_bundle_by_name.return_value.data
            data.version_number = 2
            data.secret_bundle_content.content = base64.b64encode(value).decode()
            with self.subTest(newline=value.endswith(b'\n')):
                record = vault.fetch(sdk, 'ap-singapore-1', 'ocid1.vault.synthetic', 'account_keys')
                name, version, content, ending = record.split(b'\0')
                self.assertEqual((name, version, ending), (b'account_keys', b'2', b''))
                self.assertEqual(base64.b64decode(content), value)
                sdk.secrets.SecretsClient.return_value.get_secret_bundle_by_name.assert_called_once_with(
                    secret_name='account_keys', vault_id='ocid1.vault.synthetic',
                    retry_strategy=sdk.retry.NoneRetryStrategy.return_value)
                sdk.secrets.SecretsClient.return_value.get_secret_bundle.assert_not_called()

    def test_current_bundle_error_never_falls_back_to_an_older_version(self):
        sdk = self.sdk()
        client = sdk.secrets.SecretsClient.return_value
        client.get_secret_bundle_by_name.side_effect = RuntimeError('synthetic SDK error')
        with self.assertRaises(RuntimeError):
            vault.fetch(sdk, 'ap-singapore-1', 'ocid1.vault.synthetic', 'account_keys')
        client.get_secret_bundle_by_name.assert_called_once_with(
            secret_name='account_keys', vault_id='ocid1.vault.synthetic',
            retry_strategy=sdk.retry.NoneRetryStrategy.return_value)
        client.get_secret_bundle.assert_not_called()

    def test_sdk_error_only_emits_code_and_never_traceback_or_values(self):
        sdk = self.sdk()
        error = RuntimeError('synthetic-vault-value')
        error.code = 'NotAuthorizedOrNotFound'
        sdk.auth.signers.InstancePrincipalsSecurityTokenSigner.side_effect = error
        output, errors = io.StringIO(), io.StringIO()
        with mock.patch.dict('sys.modules', oci=sdk), \
             mock.patch('sys.argv', ['deploy_vault.py', 'ap-singapore-1', 'ocid1.vault.synthetic', 'smtp_password']), \
             contextlib.redirect_stdout(output), contextlib.redirect_stderr(errors):
            self.assertEqual(vault.main(), 1)
        self.assertEqual(output.getvalue(), '')
        self.assertEqual(errors.getvalue(), 'OCI_ERROR:NotAuthorizedOrNotFound\n')

    def test_invalid_sdk_error_code_is_withheld(self):
        sdk = self.sdk()
        error = RuntimeError('synthetic-vault-value')
        error.code = 'synthetic-vault-value\n'
        sdk.auth.signers.InstancePrincipalsSecurityTokenSigner.side_effect = error
        with mock.patch.dict('sys.modules', oci=sdk), \
             mock.patch('sys.argv', ['deploy_vault.py', 'ap-singapore-1', 'ocid1.vault.synthetic', 'smtp_password']), \
             contextlib.redirect_stderr(io.StringIO()) as errors:
            self.assertEqual(vault.main(), 1)
        self.assertEqual(errors.getvalue(), 'OCI_ERROR:SDKError\n')

    def test_invalid_bundle_content_and_version_are_rejected(self):
        for field, value in (('version_number', 0), ('content_type', 'UNKNOWN'), ('content', 'bad base64')):
            sdk = self.sdk()
            data = sdk.secrets.SecretsClient.return_value.get_secret_bundle_by_name.return_value.data
            target = data if field == 'version_number' else data.secret_bundle_content
            setattr(target, field, value)
            with self.subTest(field=field), self.assertRaises(ValueError):
                vault.fetch(sdk, 'ap-singapore-1', 'ocid1.vault.synthetic', 'smtp_password')


if __name__ == '__main__':
    unittest.main()
