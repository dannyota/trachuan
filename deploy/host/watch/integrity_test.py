import json
from pathlib import Path
import tempfile
import unittest

from . import messages, observations, signals, storage
from .signals_test import NOW, record, sample


CODES = ['adoption_failed', 'pointer_changed']


class IntegritySignalTest(unittest.TestCase):
    def test_both_signals_fire_on_first_sight_and_carry_only_the_codes(self):
        for key, field in (('release_integrity', 'release'), ('frameworks_integrity', 'frameworks')):
            with self.subTest(key=key):
                state = signals.fresh()
                result = dict(record('waiting', waiting_since=NOW), integrity=CODES, message='secret database name')
                signals.observe(state, sample() | {field: result}, NOW)
                item = state['signals'][key]
                self.assertTrue(item['firing'])
                self.assertEqual(item['facts'], 'adoption_failed,pointer_changed')
                due = [message for message in signals.due(state, NOW) if message['key'] == key]
                self.assertEqual((due[0]['kind'], due[0]['priority']), ('failure', 0))
                self.assertNotIn('secret', messages.render(due[0], NOW))

    def test_an_empty_list_or_older_result_clears_and_recovers(self):
        state = signals.fresh()
        signals.observe(state, sample() | {'release': dict(record('ok'), integrity=CODES)}, NOW)
        signals.accept(state, signals.due(state, NOW)[0], NOW)
        signals.observe(state, sample() | {'release': dict(record('ok'), integrity=[])}, NOW + 60)
        self.assertFalse(state['signals']['release_integrity']['firing'])
        recovery = next(item for item in signals.due(state, NOW + 60) if item['key'] == 'release_integrity')
        self.assertEqual(recovery['kind'], 'recovery')
        signals.observe(state, sample() | {'release': record('ok')}, NOW + 120)
        self.assertFalse(state['signals']['release_integrity']['firing'])

    def test_a_missing_result_changes_nothing(self):
        state = signals.fresh()
        signals.observe(state, sample() | {'release': dict(record('ok'), integrity=CODES)}, NOW)
        signals.observe(state, sample() | {'release': None}, NOW + 60)
        self.assertTrue(state['signals']['release_integrity']['firing'])

    def test_frameworks_signal_exists_only_with_a_frameworks_result(self):
        state = signals.fresh()
        signals.observe(state, sample() | {'frameworks': dict(record('ok'), integrity=CODES)}, NOW)
        self.assertIn('frameworks_integrity', state['signals'])
        signals.observe(state, sample(), NOW + 60)
        self.assertNotIn('frameworks_integrity', state['signals'])

    def test_state_with_the_signals_round_trips(self):
        state = signals.fresh()
        signals.observe(state, sample() | {'release': dict(record('ok'), integrity=CODES),
                                           'frameworks': dict(record('ok'), integrity=CODES)}, NOW)
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'state.json'
            storage.save(path, state)
            self.assertEqual(storage.load_state(path), state)

    def test_messages_name_the_runbook_and_the_kind(self):
        for key, title in (('release_integrity', 'corpus release integrity alert'),
                           ('frameworks_integrity', 'frameworks release integrity alert')):
            text = messages.render(dict(key=key, kind='failure', priority=0, facts='pointer_changed',
                                        since=NOW, count=1, duration=0), NOW)
            with self.subTest(key=key):
                self.assertIn(title, text)
                self.assertIn('pointer_changed', text)
                self.assertIn('docs/runbooks/alarms.md', text)


class IntegrityObservationTest(unittest.TestCase):
    def result(self, integrity):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'release.json'
            value = dict(status='ok', started_at='2026-11-17T01:00:00Z')
            if integrity is not None:
                value['integrity'] = integrity
            path.write_text(json.dumps(value))
            return observations.result(path, 'release')

    def test_known_codes_and_a_missing_list_are_accepted(self):
        self.assertEqual(self.result(CODES)['integrity'], CODES)
        self.assertNotIn('integrity', self.result(None))

    def test_anything_else_is_refused(self):
        for value in ('pointer_changed', ['other'], [1], {'code': 'pointer_changed'}):
            with self.subTest(value=value), self.assertRaises(ValueError):
                self.result(value)


if __name__ == '__main__':
    unittest.main()
