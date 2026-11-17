from . import messages, signals, storage


def safe_code(value):
    return value if type(value) is int and 0 <= value <= 999 else 0


def deliver(state, now, path, credentials, send, log, save=storage.save, clock=None):
    clock = clock or (lambda: now)
    state['sends'] = [sent for sent in state['sends'] if sent > now-3600]
    due = signals.due(state, now)
    accepted = 0
    for item in due:
        log(dict(timestamp=storage.utc(now), signal=item['key'], kind=item['kind'], message='watch message due'))
    save(path, state)
    if not due:
        return True
    if now < state['retry_at'] or len(state['sends']) >= 20:
        return False
    try:
        token, chat = credentials()
    except Exception as error:
        log(dict(timestamp=storage.utc(now), message='Vault request failed',
                 http_status=safe_code(getattr(error, 'status', 0))))
        return False
    for item in due:
        now = clock()
        state['sends'] = [sent for sent in state['sends'] if sent > now-3600]
        if len(state['sends']) >= 20:
            break
        state['sends'].append(now)
        save(path, state)
        try:
            status, body = send(token, chat, messages.render(item, now))
        except Exception:
            log(dict(timestamp=storage.utc(now), message='Telegram request failed'))
            break
        code = safe_code(body.get('error_code')) if isinstance(body, dict) else 0
        if status == 429:
            parameters = body.get('parameters') if isinstance(body, dict) else None
            retry = parameters.get('retry_after') if isinstance(parameters, dict) else None
            state['retry_at'] = int(clock())+(retry if type(retry) is int and retry > 0 else 60)
            save(path, state)
        if not 200 <= status < 300 or not isinstance(body, dict) or body.get('ok') is not True:
            log(dict(timestamp=storage.utc(now), message='Telegram request failed',
                     http_status=safe_code(status), error_code=code))
            break
        signals.accept(state, item, clock())
        save(path, state)
        accepted += 1
    return accepted > 0 or not signals.due(state, now)
