import json
from pathlib import Path
import tempfile
import unittest

from . import runtime, signals, storage
from .signals_test import NOW, record, sample


class RuntimeTest(unittest.TestCase):
    def test_run_health_and_blocked_events_survive_restart(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            log = root / 'watch.log'
            data = sample() | {'corpus': '20990101'}
            def vault():
                return 'FAKE_BOT_TOKEN', 'FAKE_CHAT_ID'
            def blocked(*args):
                return 403, {'ok': False}
            def run(offset, sender=blocked):
                return runtime.run(root, log, lambda: data, vault, sender, lambda: NOW+offset)
            self.assertEqual(run(0), 0)
            data['corpus'] = '20990102'
            self.assertEqual(run(60), 1)
            result = json.loads((root/'result.json').read_text())
            self.assertEqual(result['last_success_at'], '2026-11-17T01:00:00Z')
            self.assertEqual(result['status'], 'failed')
            self.assertEqual(len(signals.due(storage.load_state(root/'state.json'), NOW+60)), 1)
            data['corpus'] = '20990103'
            sent = []
            def accepted(token, chat, text):
                sent.append(text)
                return 200, {'ok': True}
            self.assertEqual(run(120, accepted), 0)
            self.assertIn('20990103', sent[0])
            self.assertEqual(signals.due(storage.load_state(root/'state.json'), NOW+120), [])
            self.assertEqual(run(180, accepted), 0)
            self.assertEqual(len(sent), 1)
            output = ''.join(path.read_text() for path in root.iterdir())
            self.assertNotIn('FAKE_BOT_TOKEN', output)
            self.assertNotIn('FAKE_CHAT_ID', output)

    def test_signal_read_failure_records_fixed_failure_without_credentials(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            def broken():
                raise ValueError('sensitive synthetic input')
            def forbidden():
                self.fail('credentials read without a due message')
            self.assertEqual(runtime.run(root, root/'watch.log', broken, forbidden, None, lambda: NOW), 1)
            output = ''.join(path.read_text() for path in root.iterdir())
            self.assertNotIn('sensitive synthetic input', output)
            self.assertIn('watch failed', output)
            self.assertIsNone(json.loads((root/'result.json').read_text())['last_success_at'])


if __name__ == '__main__':
    unittest.main()
