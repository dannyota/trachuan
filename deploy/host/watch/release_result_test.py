from pathlib import Path
import tempfile
import unittest

from updater.deploy_corpus_result import ReleaseResult
from updater.deploy_store import Paths

from . import observations, signals
from .signals_test import NOW, sample


class ReleaseResultTest(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.paths = Paths(Path(directory.name))
        self.paths.result.parent.mkdir(parents=True)
        self.paths.log.parent.mkdir(parents=True)
        self.now = NOW
        self.state = signals.fresh()
        self.manifest = dict(rel='20990101', sequence=1)
        self.digest = 'a'*64

    def observe(self, offset):
        data = sample()
        data['release'] = observations.result(self.paths.result.with_name('release.json'), 'release')
        signals.observe(self.state, data, NOW+offset)

    def write(self, status, offset):
        self.now = NOW+offset
        result = ReleaseResult(self.paths, lambda: self.now)
        result.write(status, 'complete', 'synthetic corpus result', self.digest, self.manifest, 'new')
        self.observe(offset)

    def test_waiting_breaks_pending_failure_streak_before_next_completed_failure(self):
        self.write('failed', 0)
        self.observe(300)
        self.write('waiting', 600)
        self.write('failed', 900)
        self.observe(2700)
        self.assertFalse(self.state['signals']['release_failed']['firing'])
        self.assertFalse(any(item['key'] == 'release_failed' for item in signals.due(self.state, NOW+2700)))
        self.write('failed', 3000)
        self.assertTrue(self.state['signals']['release_failed']['firing'])
        self.assertEqual(next(item['kind'] for item in signals.due(self.state, NOW+3000)
                              if item['key'] == 'release_failed'), 'failure')

    def test_waiting_preserves_fired_failure_until_confirmed_success(self):
        for status in ('ok', 'current', 'verified'):
            with self.subTest(status=status):
                self.state = signals.fresh()
                self.write('failed', 0)
                self.write('failed', 300)
                self.observe(1800)
                self.assertTrue(self.state['signals']['release_failed']['firing'])
                failure = next(item for item in signals.due(self.state, NOW+1800)
                               if item['key'] == 'release_failed')
                signals.accept(self.state, failure, NOW+1800)
                self.write('waiting', 1860)
                self.assertTrue(self.state['signals']['release_failed']['firing'])
                self.assertFalse(any(item['key'] == 'release_failed'
                                     for item in signals.due(self.state, NOW+1860)))
                self.write(status, 1920)
                self.assertFalse(self.state['signals']['release_failed']['firing'])
                recovery = next(item for item in signals.due(self.state, NOW+1920)
                                if item['key'] == 'release_failed')
                self.assertEqual(recovery['kind'], 'recovery')

    def test_retained_waiting_age_fires_on_failed_result_and_clears_on_success(self):
        for status in ('ok', 'current', 'verified'):
            with self.subTest(status=status):
                self.state = signals.fresh()
                self.write('waiting', 0)
                self.write('failed', 3600)
                result = observations.result(self.paths.result.with_name('release.json'), 'release')
                self.assertEqual(result['status'], 'failed')
                self.assertEqual(result['waiting_since'], NOW)
                self.observe(86400)
                self.assertFalse(self.state['signals']['release_waiting']['firing'])
                self.observe(86401)
                self.assertTrue(self.state['signals']['release_waiting']['firing'])
                failure = next(item for item in signals.due(self.state, NOW+86401)
                               if item['key'] == 'release_waiting')
                self.assertEqual(failure['kind'], 'failure')
                signals.accept(self.state, failure, NOW+86401)
                self.write('failed', 86460)
                self.assertTrue(self.state['signals']['release_waiting']['firing'])
                self.write('skipped', 86520)
                self.assertTrue(self.state['signals']['release_waiting']['firing'])
                self.write(status, 86580)
                self.assertFalse(self.state['signals']['release_waiting']['firing'])
                recovery = next(item for item in signals.due(self.state, NOW+86580)
                                if item['key'] == 'release_waiting')
                self.assertEqual(recovery['kind'], 'recovery')

    def test_missing_waiting_timestamp_and_indeterminate_results_do_not_fire_waiting(self):
        self.write('failed', 0)
        self.observe(90000)
        self.assertFalse(self.state['signals']['release_waiting']['firing'])
        self.write('waiting', 90060)
        for status, offset in (('skipped', 180060), ('running', 180120)):
            self.write(status, offset)
            self.assertFalse(self.state['signals']['release_waiting']['firing'])
        self.write('failed', 180180)
        self.assertTrue(self.state['signals']['release_waiting']['firing'])


if __name__ == '__main__':
    unittest.main()
