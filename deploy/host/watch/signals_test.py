import unittest

from . import signals


NOW = 1794877200


def record(status, started=NOW, success=None, **values):
    return dict(status=status, started_at=started, finished_at=started,
                last_success_at=success, **values)


def sample():
    return dict(deploy=None, release=None, secrets=None,
                backups={}, ready=None, corpus=None, disk={}, memory=None)


class SignalsTest(unittest.TestCase):
    def observe(self, state, data, offset=0):
        signals.observe(state, data, NOW + offset)

    def test_all_failures_fire_hold_clear_and_remind(self):
        cases = [
            ('site', {'ready': 503}, {'ready': 200}, 180),
            ('deploy_stopped', {'deploy': record('stopped')}, {'deploy': record('current')}, 0),
            ('backup_full', {'backups': {'full': record('failed')}},
             {'backups': {'full': record('ok', success=NOW+1)}}, 0),
            ('backup_old', {'backups': {'diff': record('ok', success=NOW-93601)}},
             {'backups': {'full': record('ok', success=NOW)}}, 0),
            ('release_failed', {'release': record('failed')}, {'release': record('verified')}, 1800),
            ('release_waiting', {'release': record('waiting', waiting_since=NOW-86401)},
             {'release': record('ok')}, 0),
            ('secrets', {'secrets': record('failed', success=NOW)},
             {'secrets': record('current', success=NOW+2000)}, 1800),
            ('disk:/', {'disk': {'/': 71}}, {'disk': {'/': 64}}, 900),
            ('disk:/srv/trachuan/data', {'disk': {'/srv/trachuan/data': 71}},
             {'disk': {'/srv/trachuan/data': 64}}, 900),
            ('memory', {'memory': 9}, {'memory': 16}, 900),
        ]
        for key, bad, good, window in cases:
            with self.subTest(key=key):
                state = signals.fresh()
                data = sample() | bad
                self.observe(state, data)
                if window:
                    self.assertFalse(state['signals'][key]['firing'])
                    if key == 'release_failed':
                        data['release']['started_at'] = NOW+60
                    if key == 'secrets':
                        data['secrets']['started_at'] = NOW+60
                    self.observe(state, data, 60)
                    self.observe(state, data, window)
                self.assertTrue(state['signals'][key]['firing'])
                due = signals.due(state, NOW+window)
                message = next(item for item in due if item['key'] == key)
                signals.accept(state, message, NOW+window)
                self.assertFalse(any(m['key'] == key for m in signals.due(state, NOW+window+86399)))
                reminder = next(m for m in signals.due(state, NOW+window+86400) if m['key'] == key)
                self.assertEqual(reminder['kind'], 'reminder')
                self.assertEqual(reminder['priority'], 1)
                self.observe(state, sample(), window+1)
                self.assertTrue(state['signals'][key]['firing'])
                self.observe(state, sample() | good, window+2)
                recovery = next(m for m in signals.due(state, NOW+window+2) if m['key'] == key)
                self.assertEqual(recovery['kind'], 'recovery')
                self.assertGreaterEqual(recovery['duration'], 2)
                signals.accept(state, recovery, NOW+window+2)
                self.assertFalse(any(m['key'] == key for m in signals.due(state, NOW+window+2)))

    def test_single_failed_release_or_secrets_attempt_does_not_alert(self):
        for field, key in (('release', 'release_failed'), ('secrets', 'secrets')):
            with self.subTest(field=field):
                state = signals.fresh()
                data = sample() | {field: record('failed', success=NOW)}
                for offset in (0, 300, 900, 1800):
                    self.observe(state, data, offset)
                self.assertFalse(state['signals'][key]['firing'])
                data[field]['started_at'] = NOW+1800
                self.observe(state, data, 1860)
                self.assertTrue(state['signals'][key]['firing'])

    def test_delayed_recovery_reports_only_time_actually_firing(self):
        state, data = signals.fresh(), sample() | {'deploy': record('stopped')}
        self.observe(state, data)
        signals.accept(state, signals.due(state, NOW)[0], NOW)
        self.observe(state, sample() | {'deploy': record('ok')}, 120)
        message = signals.due(state, NOW+600)[0]
        self.assertEqual(message['duration'], 120)

    def test_each_backup_failure_requires_success_after_failure(self):
        for job in ('full', 'diff', 'verify', 'health', 'restore-check'):
            state, data = signals.fresh(), sample()
            data['backups'][job] = record('unknown')
            self.observe(state, data)
            key = 'backup_' + job
            self.assertTrue(state['signals'][key]['firing'])
            for status in ('running', 'skipped', 'ok'):
                data['backups'][job] = record(status, success=NOW)
                self.observe(state, data, 60)
                self.assertTrue(state['signals'][key]['firing'])
            data['backups'][job] = record('ok', success=NOW+1)
            self.observe(state, data, 120)
            self.assertFalse(state['signals'][key]['firing'])

    def test_deploy_counts_distinct_attempts_and_keeps_failure_until_success(self):
        state, data = signals.fresh(), sample()
        data['deploy'] = record('failed')
        for offset in (0, 300, 900, 1200):
            self.observe(state, data, offset)
        self.assertFalse(state['signals']['deploy_failed']['firing'])
        data['deploy'] = record('rolled_back', NOW+300)
        self.observe(state, data, 1500)
        self.assertFalse(state['signals']['deploy_failed']['firing'])
        data['deploy'] = record('failed', NOW+600)
        self.observe(state, data, 1800)
        self.assertTrue(state['signals']['deploy_failed']['firing'])
        failure = next(m for m in signals.due(state, NOW+1800) if m['key'] == 'deploy_failed')
        signals.accept(state, failure, NOW+1800)
        reminder = next(m for m in signals.due(state, NOW+88200) if m['key'] == 'deploy_failed')
        self.assertEqual(reminder['kind'], 'reminder')
        for status in ('verified', 'skipped', 'bootstrapped'):
            data['deploy'] = record(status)
            self.observe(state, data, 1860)
            self.assertTrue(state['signals']['deploy_failed']['firing'])
        data['deploy'] = record('ok')
        self.observe(state, data, 1920)
        self.assertFalse(state['signals']['deploy_failed']['firing'])
        recovery = next(m for m in signals.due(state, NOW+1920) if m['key'] == 'deploy_failed')
        self.assertEqual(recovery['kind'], 'recovery')

    def test_indeterminate_pauses_windows_and_good_resets_them(self):
        state, data = signals.fresh(), sample() | {'ready': 503}
        self.observe(state, data)
        self.observe(state, data, 60)
        self.observe(state, sample(), 120)
        self.observe(state, data, 180)
        self.assertFalse(state['signals']['site']['firing'])
        self.observe(state, data, 240)
        self.observe(state, data, 300)
        self.assertTrue(state['signals']['site']['firing'])
        self.observe(state, sample() | {'ready': 200}, 360)
        self.observe(state, data, 420)
        self.assertFalse(state['signals']['site']['firing'])
        self.assertFalse(any(m['key'] == 'site' for m in signals.due(state, NOW+420)))

    def test_hysteresis_and_strict_thresholds(self):
        for key, field, bad, middle, good in (
                ('memory', 'memory', 9, 10, 16),
                ('disk:/', 'disk', {'/': 71}, {'/': 70}, {'/': 64})):
            state = signals.fresh()
            self.observe(state, sample() | {field: bad})
            self.observe(state, sample() | {field: middle}, 900)
            self.assertFalse(state['signals'][key]['firing'])
            self.observe(state, sample() | {field: bad}, 960)
            self.observe(state, sample() | {field: bad}, 1860)
            self.assertTrue(state['signals'][key]['firing'])
            self.observe(state, sample() | {field: middle}, 1920)
            self.assertTrue(state['signals'][key]['firing'])
            self.observe(state, sample() | {field: good}, 1980)
            self.assertFalse(state['signals'][key]['firing'])

    def test_backup_old_clears_only_when_success_is_under_26_hours(self):
        state = signals.fresh()
        self.observe(state, sample() | {'backups': {'full': record('ok', success=NOW-93601)}})
        self.assertTrue(state['signals']['backup_old']['firing'])
        self.observe(state, sample() | {'backups': {'full': record('ok', success=NOW+60-93600)}}, 60)
        self.assertTrue(state['signals']['backup_old']['firing'])
        self.observe(state, sample() | {'backups': {'full': record('ok', success=NOW+120-93599)}}, 120)
        self.assertFalse(state['signals']['backup_old']['firing'])

    def test_stale_secrets_and_age_boundaries(self):
        state = signals.fresh()
        self.observe(state, sample() | {'secrets': record('ok', success=NOW-7200)})
        self.assertFalse(state['signals']['secrets']['firing'])
        self.observe(state, sample() | {'secrets': record('current', success=NOW-7201)}, 1)
        self.assertTrue(state['signals']['secrets']['firing'])
        self.observe(state, sample() | {'secrets': record('skipped', success=NOW)}, 2)
        self.assertTrue(state['signals']['secrets']['firing'])
        self.observe(state, sample() | {'secrets': record('ok', success=NOW)}, 3)
        self.assertFalse(state['signals']['secrets']['firing'])

    def test_live_events_initialize_silently_and_keep_newest_until_accepted(self):
        state = signals.fresh()
        data = sample() | {'deploy': record('ok', release='v1', sequence=1), 'corpus': '20990101'}
        self.observe(state, data)
        self.assertEqual(signals.due(state, NOW), [])
        data['deploy'] = record('current', release='v1', sequence=1)
        self.observe(state, data, 60)
        self.assertEqual(signals.due(state, NOW+60), [])
        data['deploy'] = record('ok', release='v2', sequence=2)
        data['corpus'] = '20990102'
        self.observe(state, data, 120)
        pending = signals.due(state, NOW+120)
        self.assertEqual({m['key'] for m in pending}, {'release_live', 'corpus_live'})
        data['deploy'] = record('current', release='v2', sequence=2)
        data['release'] = record('verified', release='20990103', mode='verify')
        self.observe(state, data, 180)
        self.assertEqual(signals.due(state, NOW+180), pending)
        data['corpus'] = '20990104'
        self.observe(state, data, 240)
        pending = signals.due(state, NOW+240)
        self.assertEqual(next(m['identity'] for m in pending if m['key'] == 'corpus_live'), '20990104')
        for item in pending:
            signals.accept(state, item, NOW+240)
        self.assertEqual(signals.due(state, NOW+240), [])

    def test_absent_initial_events_establish_baseline_on_first_observation(self):
        state = signals.fresh()
        self.observe(state, sample())
        self.observe(state, sample() | {'corpus': '20990101'}, 60)
        self.assertEqual(signals.due(state, NOW+60), [])


if __name__ == '__main__':
    unittest.main()
