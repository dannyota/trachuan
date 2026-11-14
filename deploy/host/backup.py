#!/usr/bin/env python3
import fcntl
from pathlib import Path
import re
import backup_process as process
import signal
import subprocess
import sys
import time

from backup_result import Result
from backup_status import Unknown, archiver_health, backup_health, backups, newest_good, verify_status


def interrupted(signum, frame):
    raise InterruptedError('backup interrupted')


CommandFailed = process.CommandFailed


def command(args, timeout, **kwargs):
    return process.command(args, timeout=timeout, **kwargs)


def work(kind, deadline, executor=None):
    pgbackrest = ['podman', 'exec', 'trachuan-postgres', 'pgbackrest', '--stanza=trachuan']
    def execute(args):
        remaining = deadline-time.monotonic()
        if remaining <= 0:
            raise subprocess.TimeoutExpired(args, remaining)
        if args[:3] == ['podman', 'exec', 'trachuan-postgres']:
            return executor.execute(args[3:], timeout=remaining)
        return command(args, timeout=remaining, diagnostics=kind == 'restore-check')
    def repository():
        try:
            return execute(pgbackrest + ['info', '--output=json'])
        except CommandFailed as error:
            raise Unknown('repository unreachable') from error
    if kind in {'full', 'diff'}:
        execute(pgbackrest + ['backup', '--type='+kind, '--no-expire-auto'])
        rows = backups(repository(), time.time())
        if max(rows, key=lambda row: row['timestamp']['stop'])['error']:
            return 'failed', 'backup recorded an error'
        execute(pgbackrest + ['expire'])
        return 'ok', 'backup and expiry completed'
    if kind == 'verify':
        return verify_status(execute(pgbackrest + ['verify', '--output=text', '--verbose']))
    if kind == 'health':
        try:
            backup = backup_health(repository(), time.time())
        except Unknown:
            backup = ('unknown', 'repository unreachable')
        query = ("SELECT json_build_object('failed_count', failed_count, "
                 "'last_failed_time', extract(epoch FROM last_failed_time), "
                 "'last_archived_time', extract(epoch FROM last_archived_time), "
                 "'stats_reset', extract(epoch FROM stats_reset), "
                 "'wal_bytes', (SELECT coalesce(sum(size),0)::bigint FROM pg_ls_waldir())) "
                 "FROM pg_stat_archiver")
        try:
            output = execute(['podman', 'exec', 'trachuan-postgres', 'psql', '-XAt',
                              '--no-password', '--host=127.0.0.1', '--username=postgres',
                              '--dbname=postgres', '--set=ON_ERROR_STOP=1', '--command='+query])
            archiver = archiver_health(output, time.time())
        except CommandFailed:
            archiver = ('unknown', 'archiver query failed')
        for status in ['failed', 'unknown']:
            for result in [backup, archiver]:
                if result[0] == status:
                    return result
        return 'ok', 'backup and archiving healthy'
    label = newest_good(repository(), time.time())['label']
    image = execute(['podman', 'inspect', '--format', '{{.Image}}', 'trachuan-postgres']).strip()
    if not re.fullmatch(r'sha256:[0-9a-f]{64}', image):
        raise Unknown('invalid live image identity')
    started = time.monotonic()
    execute(['bash', str(Path(__file__).with_name('restore-check.sh')), '--locked', label, image])
    return 'ok', f'auth restore smoke passed in {int(time.monotonic()-started)} seconds'


def run(kind, state=Path('/srv/trachuan/state/backup')):
    if kind not in {'full', 'diff', 'verify', 'health', 'restore-check'}:
        raise ValueError('invalid backup unit')
    result = Result(state / (kind+'.json'))
    with (state / 'backup.lock').open('a') as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            result.write('running', 'waiting for backup lock')
            result.write('skipped', 'another backup unit holds the lock')
            return 0
        result.write('running', 'starting')
        limits = {'full': 14400, 'diff': 7200, 'verify': 14400, 'health': 300, 'restore-check': 1200}
        try:
            executor = process.Executor(state)
            executor.recover()
            status, reason = work(kind, time.monotonic()+limits[kind], executor)
        except subprocess.TimeoutExpired as error:
            detail = error.stderr or ''
            status, reason = 'failed', 'deadline exceeded'+(': '+detail if detail else '')
        except Unknown:
            status, reason = 'unknown', 'repository unreachable or invalid output'
        except CommandFailed as error:
            status, reason = 'failed', str(error) or 'command failed'
        except (OSError, InterruptedError):
            status, reason = 'failed', 'command failed, interrupted or exceeded deadline'
        result.write(status, reason)
        return 0 if status in {'ok', 'skipped'} else 1


if __name__ == '__main__':
    signal.signal(signal.SIGTERM, interrupted)
    signal.signal(signal.SIGINT, interrupted)
    if len(sys.argv) != 2:
        sys.exit('usage: backup.py full|diff|verify|health|restore-check')
    sys.exit(run(sys.argv[1]))
