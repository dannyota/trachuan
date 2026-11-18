import json
from pathlib import Path
import tempfile
import unittest

from . import messages, observations, signals, storage
from .signals_test import NOW, record, sample


class FrameworksReleaseWatchTest(unittest.TestCase):
    def test_collect_reads_frameworks_result_with_release_statuses(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'deploy').mkdir()
            (root / 'backup').mkdir()
            def collect():
                return observations.collect(root, lambda: 'active', lambda *args: (503, {}),
                                            lambda path: 20, lambda: 50)
            self.assertIsNone(collect()['frameworks'])
            path = root / 'deploy/release-frameworks.json'
            path.write_text(json.dumps(dict(status='waiting', started_at='2026-11-17T01:00:00Z',
                                            waiting_since='2026-11-17T01:00:00Z', message='release floor')))
            value = collect()
            self.assertEqual((value['frameworks']['status'], value['frameworks']['waiting_since']), ('waiting', NOW))
            self.assertIsNone(value['release'])
            path.write_text(json.dumps(dict(status='rolled_back', started_at='2026-11-17T01:00:00Z')))
            with self.assertRaises(ValueError):
                collect()

    def test_frameworks_failure_and_waiting_fire_their_own_signals(self):
        state = signals.fresh()
        data = sample() | {'release': record('ok'), 'frameworks': record('failed')}
        signals.observe(state, data, NOW)
        data['frameworks'] = record('failed', NOW + 60)
        signals.observe(state, data, NOW + 60)
        signals.observe(state, data, NOW + 1800)
        self.assertTrue(state['signals']['frameworks_failed']['firing'])
        self.assertFalse(state['signals']['release_failed']['firing'])
        data = sample() | {'release': record('failed'), 'frameworks': record('waiting', waiting_since=NOW - 86401)}
        signals.observe(state, data, NOW + 1860)
        self.assertTrue(state['signals']['frameworks_waiting']['firing'])
        self.assertTrue(state['signals']['frameworks_failed']['firing'])
        self.assertFalse(state['signals'].get('release_waiting', {}).get('firing', False))
        due = [item for item in signals.due(state, NOW + 1860) if item['key'].startswith('frameworks_')]
        self.assertEqual({item['key'] for item in due}, {'frameworks_failed', 'frameworks_waiting'})
        for item in due:
            signals.accept(state, item, NOW + 1860)
        signals.observe(state, sample() | {'frameworks': record('current')}, NOW + 1920)
        self.assertFalse(state['signals']['frameworks_failed']['firing'])
        self.assertFalse(state['signals']['frameworks_waiting']['firing'])
        recovered = {item['key'] for item in signals.due(state, NOW + 1920) if item['kind'] == 'recovery'}
        self.assertEqual(recovered, {'frameworks_failed', 'frameworks_waiting'})

    def test_frameworks_signals_exist_only_while_a_frameworks_result_is_observed(self):
        state = signals.fresh()
        signals.observe(state, sample() | {'release': record('ok')}, NOW)
        self.assertFalse([key for key in state['signals'] if key.startswith('frameworks_')])
        signals.observe(state, sample() | {'frameworks': record('ok')}, NOW + 60)
        self.assertLessEqual({'frameworks_failed', 'frameworks_waiting'}, set(state['signals']))
        signals.observe(state, sample(), NOW + 120)
        self.assertFalse([key for key in state['signals'] if key.startswith('frameworks_')])

    def test_state_and_messages_accept_frameworks_signals(self):
        state = signals.fresh()
        signals.observe(state, sample() | {'frameworks': record('waiting', waiting_since=NOW - 86401)}, NOW)
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'state.json'
            storage.save(path, state)
            self.assertEqual(storage.load_state(path), state)
        for key, title in (('frameworks_failed', 'frameworks release failing'),
                           ('frameworks_waiting', 'frameworks release waiting')):
            text = messages.render(dict(key=key, kind='failure', priority=1, facts='release floor',
                                        since=NOW, count=1, duration=0), NOW)
            with self.subTest(key=key):
                self.assertIn(title, text)
                self.assertIn('release floor', text)


if __name__ == '__main__':
    unittest.main()
