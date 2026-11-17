from datetime import datetime, timezone
from html import escape
from zoneinfo import ZoneInfo


def clock(now):
    time = datetime.fromtimestamp(now, timezone.utc)
    return time.astimezone(ZoneInfo('Asia/Ho_Chi_Minh')).strftime('%H:%M ICT') + ' · ' + time.strftime('%H:%M UTC')


def details(key):
    if key.startswith('backup_'):
        return ('backup old' if key == 'backup_old' else 'backup failed ('+key[7:]+')',
                'Action: check backups and the latest restore proof.', 'backups', 'backups.md')
    if key.startswith('disk:'):
        return 'filesystem', 'Action: check free space and retained releases.', key[5:], 'data-volume.md'
    return {
        'site': ('site not ready', 'Action: check the server and deploy result.', 'readyz', 'deploy.md'),
        'deploy_stopped': ('deploy stopped', 'Action: recover the stopped deployment.', 'deploy', 'deploy.md'),
        'deploy_failed': ('deploy failing', 'System: the updater retries every 5 min.', 'deploy', 'deploy.md'),
        'release_failed': ('corpus release failing', 'System: release activation retries every 5 min.', 'release', 'deploy.md'),
        'release_waiting': ('corpus release waiting', 'Action: check corpus schema support.', 'release', 'deploy.md'),
        'secrets': ('secrets failing or stale', 'System: secret reconciliation retries on the updater timer.', 'secrets', 'host.md'),
        'memory': ('memory', 'Action: check host memory and service limits.', 'memory', 'host.md'),
        'release_live': ('release live', 'System: the host serves the active release.', 'deploy', 'deploy.md'),
        'corpus_live': ('corpus live', 'System: the server serves the active corpus.', 'release', 'deploy.md'),
    }[key]


def render(item, now):
    title, action, job, runbook = details(item['key'])
    kind = item['kind']
    mark = ('🔴', '🟠', '🟢')[item['priority']]
    if kind == 'recovery':
        title += ' recovered'
        facts = f'Recovered after firing for {int(item["duration"] // 60)} min.'
    elif kind == 'event':
        identity = item.get('identity', '')
        facts = (f'Release {identity[0]}; sequence {identity[1]}.' if isinstance(identity, list)
                 else f'Corpus {identity}.')
    else:
        if kind == 'reminder':
            title += ' reminder'
        start = clock(item['since']) if item.get('since') is not None else clock(now)
        counts = f'{item["count"]} checks in a row' if item['key'] == 'site' else f'{item["count"]} observations or attempts'
        facts = f'{counts} since {start}; {item.get("facts", "")}'
    lines = [mark+' <b>trachuan</b> · '+escape(title),
             escape(' '.join(facts.split())[:600]), escape(action),
             '<i>'+escape(clock(now)+' · '+job+' · docs/runbooks/'+runbook)+'</i>']
    return '\n'.join(lines)
