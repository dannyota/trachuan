def fresh():
    return dict(version=1, signals={}, events={}, sends=[], retry_at=0)


def update(state, key, value, now, window=0, count=1, attempt=None, facts='', failure_at=None):
    item = state['signals'].setdefault(key, dict(
        firing=False, told=False, told_at=None, fired_at=None, since=None,
        elapsed=0, count=0, attempts=[], checked_at=now, previous=None, cleared_at=None, facts=''))
    delta = max(0, now-item['checked_at']) if item['previous'] == 'bad' else 0
    item['checked_at'], item['previous'] = now, value
    if value is None:
        return
    item['facts'] = facts
    if value == 'good':
        if item['firing']:
            item['cleared_at'] = now
        item.update(firing=False, since=None, elapsed=0, count=0, attempts=[])
        return
    if value == 'hold':
        if not item['firing']:
            item.update(since=None, elapsed=0, count=0, attempts=[])
        return
    if item['since'] is None:
        item['since'] = now
    item['elapsed'] += delta
    if attempt is None:
        item['count'] += 1
    elif attempt not in item['attempts']:
        item['attempts'].append(attempt)
        item['attempts'] = item['attempts'][-3:]
        item['count'] = len(item['attempts'])
    if failure_at is not None:
        item['failure_at'] = max(item.get('failure_at', failure_at), failure_at)
    if item['elapsed'] >= window and item['count'] >= count and not item['firing']:
        item['firing'], item['fired_at'], item['cleared_at'] = True, now, None


def status_value(result, bad, good):
    if result is None:
        return None
    status = result['status']
    return 'bad' if status in bad else 'good' if status in good else None


def event(state, key, identity):
    if identity is None:
        return
    if key not in state['events']:
        state['events'][key] = dict(observed=identity, told=identity)
    else:
        state['events'][key]['observed'] = identity


def release_signals(state, prefix, release, now, message):
    failure = status_value(release, ('failed',), ('ok', 'current', 'verified'))
    if release and release['status'] == 'waiting':
        failure = 'hold'
    update(state, prefix + '_failed', failure,
           now, 1800, 2, attempt=(release or {}).get('started_at'), facts=message(release))
    waiting = status_value(release, (), ('ok', 'current', 'verified'))
    if release and release['status'] in ('waiting', 'failed') and release.get('waiting_since') is not None:
        waiting = 'bad' if now-release['waiting_since'] > 86400 else 'hold'
    update(state, prefix + '_waiting', waiting, now, facts=message(release))


def observe(state, data, now):
    ready = data['ready']
    update(state, 'site', None if ready is None else 'good' if ready == 200 else 'bad',
           now, 180, 3, facts=f'/readyz last HTTP {ready or "unavailable"}.')
    deploy, release, secrets = (data[name] for name in ('deploy', 'release', 'secrets'))
    message = lambda result: ' '.join(str((result or {}).get('message', '')).split())[:160]
    update(state, 'deploy_stopped', status_value(deploy, ('stopped',), ('ok', 'current')),
           now, facts=message(deploy))
    update(state, 'deploy_failed', status_value(deploy, ('failed', 'rolled_back'), ('ok', 'current')),
           now, 900, 3, attempt=(deploy or {}).get('started_at'), facts=message(deploy))
    release_signals(state, 'release', release, now, message)
    if data['frameworks'] is None:
        for key in ('frameworks_failed', 'frameworks_waiting'):
            state['signals'].pop(key, None)
    else:
        release_signals(state, 'frameworks', data['frameworks'], now, message)
    secret_value = status_value(secrets, ('failed',), ('ok', 'current'))
    stale = (secrets or {}).get('last_success_at')
    stale = stale is not None and now-stale > 7200
    if secret_value is not None and stale:
        secret_value = 'bad'
    elif secret_value == 'good' and (secrets['last_success_at'] is None or
                                     now-secrets['last_success_at'] >= 7200):
        secret_value = 'hold'
    update(state, 'secrets', secret_value, now, 0 if stale else 1800, 1 if stale else 2,
           attempt=(secrets or {}).get('started_at'), facts=message(secrets))
    for job in ('full', 'diff', 'verify', 'health', 'restore-check'):
        result = data['backups'].get(job)
        key = 'backup_' + job
        value = status_value(result, ('failed', 'unknown'), ('ok',))
        if value == 'good':
            success = result.get('last_success_at')
            failure = state['signals'].get(key, {}).get('failure_at')
            if success is None or (failure is not None and success <= failure):
                value = None
        failed_at = (result.get('finished_at') or result.get('started_at') or now) if value == 'bad' else None
        update(state, key, value, now, facts=message(result), failure_at=failed_at)
    successes = [result['last_success_at'] for job, result in data['backups'].items()
                 if job in ('full', 'diff') and result and result.get('last_success_at') is not None]
    age = now-max(successes) if successes else None
    update(state, 'backup_old', None if age is None else 'bad' if age > 93600 else 'good' if age < 93600 else 'hold',
           now, facts=f'Newest full or diff success age: {int(age or 0)} seconds.')
    for mount in ('/', '/srv/trachuan/data'):
        used = data['disk'].get(mount)
        value = None if used is None else 'bad' if used > 70 else 'good' if used < 65 else 'hold'
        update(state, 'disk:' + mount, value, now, 900, facts=f'{mount} used {used}% (fire above 70%; clear below 65%).')
    memory = data['memory']
    value = None if memory is None else 'bad' if memory < 10 else 'good' if memory > 15 else 'hold'
    update(state, 'memory', value, now, 900,
           facts=f'MemAvailable {memory}% (fire below 10%; clear above 15%).')
    if deploy and deploy['status'] in ('ok', 'current'):
        if deploy.get('release') is not None and deploy.get('sequence') is not None:
            event(state, 'release_live', [deploy['release'], deploy['sequence']])
    event(state, 'corpus_live', data['corpus'])


def due(state, now):
    messages = []
    for key, item in state['signals'].items():
        kind = None
        if item['firing'] != item['told']:
            kind = 'failure' if item['firing'] else 'recovery'
        elif item['firing'] and now-item['told_at'] >= 86400:
            kind = 'reminder'
        if kind is None:
            continue
        red = key in ('site', 'deploy_stopped', 'backup_old') or key.startswith('backup_')
        priority = 2 if kind == 'recovery' else 1 if kind == 'reminder' else 0 if red else 1
        messages.append(dict(key=key, kind=kind, priority=priority, facts=item['facts'],
                             since=item['since'] if item['since'] is not None else item['fired_at'],
                             count=item['count'], duration=max(0, (item['cleared_at'] if item['cleared_at'] is not None else now)-
                                          (item['fired_at'] if item['fired_at'] is not None else now))))
    for key, item in state['events'].items():
        if item['observed'] != item['told']:
            messages.append(dict(key=key, kind='event', priority=2, identity=item['observed']))
    return sorted(messages, key=lambda message: (message['priority'], message['key']))


def accept(state, message, now):
    if message['kind'] == 'event':
        state['events'][message['key']]['told'] = message['identity']
    else:
        item = state['signals'][message['key']]
        item['told'], item['told_at'] = item['firing'], now
