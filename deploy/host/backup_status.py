#!/usr/bin/env python3
import json
import math
import re


class Unknown(ValueError):
    pass


def timestamp(value, now):
    if type(value) not in (int, float) or not math.isfinite(value) or not 0 < value <= now:
        raise Unknown('invalid timestamp')
    return value


def backups(text, now):
    try:
        data = json.loads(text)
        if not isinstance(data, list):
            raise Unknown('invalid repository output')
        stanzas = [s for s in data if s['name'] == 'trachuan']
        if len(stanzas) != 1 or stanzas[0]['status']['code'] != 0:
            raise Unknown('repository unavailable or no backup')
        rows = stanzas[0]['backup']
        if not rows:
            raise Unknown('no backup')
        for row in rows:
            timestamp(row['timestamp']['start'], now)
            timestamp(row['timestamp']['stop'], now)
            if row['timestamp']['stop'] < row['timestamp']['start']:
                raise Unknown('invalid timestamp')
            if type(row.get('error')) is not bool:
                raise Unknown('missing backup error status')
            if not re.fullmatch(r'\d{8}-\d{6}F(?:_\d{8}-\d{6}[DI])?', row['label']):
                raise Unknown('invalid backup label')
        return rows
    except (KeyError, TypeError, ValueError) as error:
        raise Unknown('invalid repository output') from error


def newest_good(text, now):
    rows = [row for row in backups(text, now) if not row['error']]
    if not rows:
        raise Unknown('no good backup')
    return max(rows, key=lambda row: row['timestamp']['stop'])


def backup_health(text, now):
    try:
        rows = backups(text, now)
        if any(row['error'] for row in rows):
            return 'failed', 'repository contains an errored backup'
        newest = newest_good(text, now)
        if now - newest['timestamp']['stop'] > 26 * 3600:
            return 'failed', 'newest good backup older than 26 hours'
        return 'ok', 'backup current'
    except Unknown:
        return 'unknown', 'repository unavailable, missing backup or invalid output'


def archiver_health(text, now):
    try:
        row = json.loads(text)
        size, count = row['wal_bytes'], row['failed_count']
        if type(size) is not int or size < 0 or type(count) is not int or count < 0:
            raise Unknown('invalid counters')
        failed = row['last_failed_time']
        archived = row['last_archived_time']
        if failed is not None:
            timestamp(failed, now)
        if archived is not None:
            timestamp(archived, now)
        if size > 10_000_000_000:
            return 'failed', 'pg_wal above 10 GB'
        if count and failed is None:
            raise Unknown('invalid archiver timestamp')
        if failed is not None and (archived is None or failed > archived):
            since = archived if archived is not None else timestamp(row['stats_reset'], now)
            if now - since > 1800:
                return 'failed', 'archiving failing for more than 30 minutes'
        return 'ok', 'archiving healthy'
    except (KeyError, TypeError, ValueError):
        return 'unknown', 'invalid archiver output'


def verify_status(text):
    lines = [line.strip() for line in text.splitlines()]
    if lines.count('stanza: trachuan') != 1 or sum(line.startswith('stanza:') for line in lines) != 1:
        return 'unknown', 'invalid verify output'
    if 'status: error' in lines or re.search(r'status: (?:invalid|manifest missing)', text):
        return 'failed', 'repository verification failed'
    counts = re.findall(r'missing: (\d+), checksum invalid: (\d+), size invalid: (\d+), other: (\d+)', text)
    if any(int(value) for group in counts for value in group):
        return 'failed', 'repository contains invalid files'
    summaries = re.findall(r'total (?:WAL|files) checked: (\d+), total valid (?:WAL|files): (\d+)', text)
    if any(checked != valid for checked, valid in summaries):
        return 'failed', 'repository contains invalid files'
    if lines.count('status: ok') != 1 or not counts or len(counts) != len(summaries):
        return 'unknown', 'incomplete verbose verify output'
    return 'ok', 'repository verification passed'
