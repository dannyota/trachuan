import base64
import contextlib
import io
import json
import re
from urllib.error import HTTPError
from urllib.request import HTTPRedirectHandler, ProxyHandler, Request, build_opener


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, request, file, code, message, headers, newurl):
        return None


class RequestFailed(Exception):
    def __init__(self, message, status=0):
        super().__init__(message)
        self.status = status if type(status) is int and 0 <= status <= 999 else 0


def open_request(request, timeout):
    return build_opener(ProxyHandler({}), NoRedirect()).open(request, timeout=timeout)


def response(request, timeout, opener, decode=True):
    try:
        opened = opener(request, timeout=timeout)
    except HTTPError as error:
        opened = error
    with opened as source:
        status = getattr(source, 'status', None) or getattr(source, 'code', None) or 200
        if not decode:
            return status, {}
        raw = source.read(65537)
    if len(raw) > 65536:
        return status, {}
    try:
        return status, json.loads(raw)
    except (ValueError, UnicodeError, RecursionError):
        return status, {}


def local(path, timeout):
    if path not in ('/readyz', '/api/v1/public/status'):
        raise ValueError('invalid watch endpoint')
    return response(Request('http://127.0.0.1:8080'+path), timeout, open_request, path != '/readyz')


def send(token, chat, text, opener=open_request):
    try:
        if not re.fullmatch('[A-Za-z0-9_:-]{1,512}', token):
            raise ValueError('invalid token')
        request = Request('https://api.telegram.org/bot'+token+'/sendMessage',
                          data=json.dumps(dict(chat_id=chat, text=text, parse_mode='HTML',
                                               link_preview_options={'is_disabled': True})).encode(),
                          headers={'Content-Type': 'application/json'})
        return response(request, 10, opener)
    except Exception:
        raise RequestFailed('Telegram request failed') from None


def sdk():
    import oci
    return oci


def credentials(policy, load_sdk=sdk):
    try:
        with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
            sdk = load_sdk()
            retry = sdk.retry.NoneRetryStrategy()
            signer = sdk.auth.signers.InstancePrincipalsSecurityTokenSigner(retry_strategy=retry)
            client = sdk.secrets.SecretsClient(config={'region': policy['region']}, signer=signer,
                                              timeout=(5, 10), retry_strategy=retry)
            values = []
            for name in ('telegram_bot_token', 'telegram_chat_id'):
                data = client.get_secret_bundle_by_name(secret_name=name, vault_id=policy['vault_id'],
                                                        retry_strategy=retry).data
                content = data.secret_bundle_content
                if type(data.version_number) is not int or data.version_number < 1 or content.content_type != 'BASE64':
                    raise ValueError('invalid Vault bundle')
                raw = base64.b64decode(content.content, validate=True)
                if not 0 < len(raw) <= 1024:
                    raise ValueError('invalid Vault value')
                values.append(raw.decode('utf-8'))
            return tuple(values)
    except Exception as error:
        raise RequestFailed('Vault request failed', getattr(error, 'status', 0)) from None
