import json
from pathlib import Path
import tempfile
import unittest

from . import delivery, messages, signals, storage
from .signals_test import NOW, record, sample


class DeliveryTest(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.path = Path(self.directory.name) / 'state.json'
        self.state = signals.fresh()
        self.logs, self.sent, self.fetched = [], [], []

    def credentials(self):
        self.fetched.append(True)
        return 'FAKE_BOT_TOKEN', 'FAKE_CHAT_ID'

    def send(self, token, chat, text):
        self.assertEqual((token, chat), ('FAKE_BOT_TOKEN', 'FAKE_CHAT_ID'))
        self.sent.append(text)
        return 200, {'ok': True}

    def run_delivery(self, now=NOW, sender=None):
        return delivery.deliver(self.state, now, self.path, self.credentials,
                                sender or self.send, self.logs.append)

    def fire_site(self):
        data = sample() | {'ready': 503}
        for offset in (0, 60, 180):
            signals.observe(self.state, data, NOW+offset)

    def test_first_run_needs_no_credentials_and_failed_run_preserves_success(self):
        self.assertTrue(self.run_delivery())
        self.assertEqual(self.fetched, [])
        storage.result(self.path.parent, True, NOW)
        storage.result(self.path.parent, False, NOW+60)
        result = json.loads(self.path.with_name('result.json').read_text())
        self.assertEqual(result['status'], 'failed')
        self.assertEqual(result['last_success_at'], '2026-11-17T01:00:00Z')

    def test_rolling_limit_orders_red_orange_green_and_carries_remaining(self):
        data = sample() | {'ready': 503, 'memory': 9, 'corpus': '20990101'}
        signals.observe(self.state, data, NOW)
        data['corpus'] = '20990102'
        signals.observe(self.state, data, NOW+60)
        signals.observe(self.state, data, NOW+900)
        self.state['sends'] = [NOW+1] * 19 + [NOW-3600]
        self.assertTrue(self.run_delivery(NOW+900))
        self.assertEqual(len(self.sent), 1)
        self.assertIn('site not ready', self.sent[0])
        self.assertEqual(len(signals.due(self.state, NOW+900)), 2)
        self.assertFalse(self.run_delivery(NOW+960))
        self.assertEqual(len(self.fetched), 1)
        self.assertTrue(self.run_delivery(NOW+3601))
        self.assertIn('memory', self.sent[1])
        self.assertIn('corpus live', self.sent[2])
        self.assertEqual(len(self.logs), 7)

    def test_failed_requests_also_consume_the_rolling_hour_limit(self):
        self.fire_site()
        calls = []
        def denied(*args):
            calls.append(True)
            return 403, {'ok': False}
        for minute in range(21):
            self.assertFalse(self.run_delivery(NOW+180+minute*60, denied))
        self.assertEqual(len(calls), 20)
        self.assertEqual(len(self.state['sends']), 20)
        self.assertEqual(len(self.fetched), 20)

    def test_429_stores_deadline_without_in_run_retry_and_blocks_vault(self):
        self.fire_site()
        for retry, expected in ((120, 120), (0, 60), ('10', 60), (True, 60), (None, 60)):
            with self.subTest(retry=retry):
                self.state['retry_at'] = 0
                def blocked(token, chat, text):
                    self.sent.append(text)
                    return 429, {'ok': False, 'error_code': 429, 'parameters': {'retry_after': retry}}
                self.assertFalse(self.run_delivery(NOW+180, blocked))
                self.assertEqual(self.state['retry_at'], NOW+180+expected)
                count = len(self.fetched)
                self.assertFalse(self.run_delivery(NOW+181))
                self.assertEqual(len(self.fetched), count)
        self.assertEqual(len(self.sent), 5)
        self.assertTrue(self.run_delivery(NOW+240))

    def test_partial_acceptance_is_success_and_send_times_use_injected_clock(self):
        signals.observe(self.state, sample() | {'deploy': record('stopped'),
                        'backups': {'full': record('failed')}}, NOW)
        count = []
        def partial(*args):
            count.append(True)
            return (200, {'ok': True}) if len(count) == 1 else (503, {'ok': False})
        self.assertTrue(delivery.deliver(self.state, NOW, self.path, self.credentials,
                                        partial, self.logs.append, clock=lambda: NOW+10))
        self.assertEqual(self.state['sends'], [NOW+10, NOW+10])
        self.assertEqual(len(signals.due(self.state, NOW+10)), 1)

    def test_error_status_or_ok_false_stops_sending_and_does_not_mark_told(self):
        self.fire_site()
        for status, body in ((500, {'ok': True}), (200, {'ok': False}), (200, {})):
            self.assertFalse(self.run_delivery(NOW+180, lambda *args: (status, body)))
            self.assertTrue(signals.due(self.state, NOW+180))
        self.assertTrue(self.run_delivery(NOW+180))
        self.assertEqual(signals.due(self.state, NOW+180), [])
        self.assertEqual(signals.due(storage.load_state(self.path), NOW+180), [])

    def test_crash_before_state_write_can_repeat_but_after_write_does_not(self):
        self.fire_site()
        storage.save(self.path, self.state)
        def crash(path, state):
            if any(item['told'] for item in state['signals'].values()):
                raise SystemExit('simulated crash')
            storage.save(path, state)
        with self.assertRaises(SystemExit):
            delivery.deliver(self.state, NOW+180, self.path, self.credentials,
                             self.send, self.logs.append, save=crash)
        self.state = storage.load_state(self.path)
        self.assertTrue(self.run_delivery(NOW+240))
        self.assertEqual(len(self.sent), 2)
        self.state = storage.load_state(self.path)
        self.assertTrue(self.run_delivery(NOW+300))
        self.assertEqual(len(self.sent), 2)

    def test_vault_and_send_exceptions_never_expose_credentials(self):
        self.fire_site()
        def broken(*args):
            raise RuntimeError('FAKE_BOT_TOKEN FAKE_CHAT_ID')
        self.assertFalse(delivery.deliver(self.state, NOW+180, self.path, broken,
                                          self.send, self.logs.append))
        self.assertFalse(self.run_delivery(NOW+240, broken))
        storage.result(self.path.parent, False, NOW)
        output = ''.join(p.read_text() for p in self.path.parent.iterdir()) + str(self.logs)
        for value in ('FAKE_BOT_TOKEN', 'FAKE_CHAT_ID'):
            self.assertNotIn(value, output)


class MessagesTest(unittest.TestCase):
    def test_escaping_message_cap_and_four_line_limit(self):
        state = signals.fresh()
        signals.observe(state, sample() | {'deploy': record('stopped', message='\n  A&B <failed>  '*1000)}, NOW)
        item = signals.due(state, NOW)[0]
        text = messages.render(item, NOW)
        self.assertEqual(len(text.splitlines()), 4)
        self.assertIn('&amp;', text)
        self.assertIn('&lt;failed&gt;', text)
        self.assertNotIn('<failed>', text)
        self.assertLess(len(text), 4096)
        self.assertLessEqual(len(item['facts']), 160)
        event = dict(key='release_live', kind='event', priority=2,
                     identity=['&'*5000+'\n<&', 123])
        text = messages.render(event, NOW)
        self.assertLess(len(text), 4096)
        self.assertTrue(text.endswith('</i>'))
        self.assertEqual(len(text.splitlines()), 4)
        self.assertNotIn('<&', text)

    def test_ict_utc_midnight_and_recovery_duration(self):
        item = dict(key='site', kind='failure', priority=0, facts='/readyz last HTTP 503.',
                    since=1794936600, count=3, duration=0)
        text = messages.render(item, 1794938400)
        self.assertIn('01:00 ICT · 18:00 UTC', text)
        self.assertIn('00:30 ICT', text)
        self.assertIn('3 checks', text)
        item.update(kind='recovery', priority=2, duration=3660)
        self.assertIn('61 min', messages.render(item, 1794938400))

    def test_all_message_footers_point_to_real_runbooks(self):
        root = Path(__file__).resolve().parents[3]
        for key in ('site', 'deploy_failed', 'deploy_stopped', 'release_failed', 'release_waiting',
                    'secrets', 'backup_full', 'backup_old', 'memory', 'disk:/',
                    'release_live', 'corpus_live'):
            item = dict(key=key, kind='failure', priority=0, facts='', since=NOW, count=3, duration=0)
            text = messages.render(item, NOW)
            path = text.split(' · ')[-1].removesuffix('</i>')
            self.assertTrue((root / path).is_file(), path)


if __name__ == '__main__':
    unittest.main()
