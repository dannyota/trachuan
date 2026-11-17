import base64
import io
import json
import traceback
import types
import unittest
from unittest.mock import MagicMock
from urllib.error import HTTPError

from . import network


class NetworkTest(unittest.TestCase):
    def test_telegram_request_has_bounded_timeout_and_exact_payload(self):
        requests = []
        def open_request(request, timeout):
            requests.append((request, timeout))
            return io.BytesIO(b'{"ok":true}')
        result = network.send('FAKE_BOT_TOKEN', 'FAKE_CHAT_ID', '<b>test</b>', open_request)
        request, timeout = requests[0]
        self.assertEqual(result, (200, {'ok': True}))
        self.assertEqual(timeout, 10)
        self.assertEqual(request.full_url, 'https://api.telegram.org/botFAKE_BOT_TOKEN/sendMessage')
        self.assertEqual(json.loads(request.data), dict(chat_id='FAKE_CHAT_ID', text='<b>test</b>',
                         parse_mode='HTML', link_preview_options={'is_disabled': True}))
        self.assertEqual(request.get_header('Content-type'), 'application/json')

    def test_http_errors_return_only_status_and_telegram_response_data(self):
        def open_request(*args, **kwargs):
            raise HTTPError('https://api.telegram.org/botFAKE_BOT_TOKEN', 429, 'FAKE_CHAT_ID', {},
                            io.BytesIO(b'{"ok":false,"error_code":429,"parameters":{"retry_after":120}}'))
        status, body = network.send('FAKE_BOT_TOKEN', 'FAKE_CHAT_ID', 'test', open_request)
        self.assertEqual(status, 429)
        self.assertEqual(body['parameters']['retry_after'], 120)

    def test_redirect_is_not_followed_with_credentials(self):
        request = network.Request('https://api.telegram.org/botFAKE_BOT_TOKEN/sendMessage')
        self.assertIsNone(network.NoRedirect().redirect_request(request, None, 302, '', {}, 'https://other.invalid'))

    def test_malformed_429_keeps_status_for_default_retry_deadline(self):
        def open_request(*args, **kwargs):
            raise HTTPError('https://api.telegram.org/botFAKE_BOT_TOKEN', 429, 'FAKE_CHAT_ID', {},
                            io.BytesIO(b'not JSON'))
        self.assertEqual(network.send('FAKE_BOT_TOKEN', 'FAKE_CHAT_ID', 'test', open_request), (429, {}))

    def test_transport_errors_have_no_url_response_exception_or_credentials(self):
        def broken(*args, **kwargs):
            raise RuntimeError('FAKE_BOT_TOKEN FAKE_CHAT_ID')
        for operation in (lambda: network.send('FAKE_BOT_TOKEN', 'FAKE_CHAT_ID', 'test', broken),
                          lambda: network.credentials({'region': 'fake-region', 'vault_id': 'fake-vault'}, broken)):
            try:
                operation()
            except Exception as error:
                text = ''.join(traceback.format_exception(error))
                self.assertNotIn('FAKE_BOT_TOKEN', str(error))
                self.assertNotIn('FAKE_CHAT_ID', str(error))
                self.assertNotIn('RuntimeError: FAKE_BOT_TOKEN FAKE_CHAT_ID', text)
            else:
                self.fail('expected sanitized error')

    def test_vault_uses_instance_principal_only_two_names_and_base64(self):
        sdk = MagicMock()
        values = {'telegram_bot_token': 'FAKE_BOT_TOKEN', 'telegram_chat_id': 'FAKE_CHAT_ID'}
        def fetch(secret_name, vault_id, retry_strategy):
            self.assertEqual(vault_id, 'fake-vault')
            value = values[secret_name]
            content = types.SimpleNamespace(content_type='BASE64', content=base64.b64encode(value.encode()).decode())
            return types.SimpleNamespace(data=types.SimpleNamespace(version_number=1, secret_bundle_content=content))
        sdk.secrets.SecretsClient.return_value.get_secret_bundle_by_name.side_effect = fetch
        self.assertEqual(network.credentials({'region': 'fake-region', 'vault_id': 'fake-vault'}, lambda: sdk),
                         ('FAKE_BOT_TOKEN', 'FAKE_CHAT_ID'))
        names = [call.kwargs['secret_name'] for call in sdk.secrets.SecretsClient.return_value.get_secret_bundle_by_name.call_args_list]
        self.assertEqual(names, ['telegram_bot_token', 'telegram_chat_id'])
        self.assertIn('signer', sdk.secrets.SecretsClient.call_args.kwargs)
        self.assertEqual(sdk.secrets.SecretsClient.call_args.kwargs['config'], {'region': 'fake-region'})


if __name__ == '__main__':
    unittest.main()
