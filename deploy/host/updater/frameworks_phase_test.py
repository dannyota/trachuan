import fcntl
import json
import unittest
from unittest import mock

from updater import activation_test as fixture
from updater.activation_test import Crash
from updater.deploy_corpus_kind import FRAMEWORKS
from updater.deploy_io import canonical
from updater.deploy_run import INTERRUPTED, Updater
from updater.deploy_state import State
from updater.frameworks_verify_test import BLOCK


class FrameworksPhaseTest(fixture.ActivationFixture, unittest.TestCase):
    def setUp(self):
        super().setUp()
        self.paths.lock.parent.mkdir(parents=True, exist_ok=True)
        self.paths.backup_lock.parent.mkdir(parents=True, exist_ok=True)
        self.paths.backup_lock.touch()
        self.paths.mode.parent.mkdir(parents=True, exist_ok=True)
        self.paths.mode.write_text('activate\n')

    def app(self, block=True):
        rules = dict(self.rules, frameworks=dict(BLOCK)) if block else self.rules
        self.paths.policy.write_bytes(canonical(rules))
        return Updater(self.paths, self.runner, user_prefix=[])

    def stub(self, app, **outcomes):
        calls = []
        def phase(name):
            def run(*args):
                calls.append(name)
                outcome = outcomes.get(name, True)
                if isinstance(outcome, BaseException):
                    raise outcome
                return outcome
            return run
        for name in app.phases:
            setattr(app, name, phase(name))
        return calls

    def staged(self):
        releases = self.paths.root / 'srv/trachuan/data/releases'
        staging = releases / 'frameworks-20990101'
        staging.mkdir(parents=True, exist_ok=True)
        (staging / 'frameworks.dump').write_bytes(b'synthetic licensed dump')
        law = releases / 'vn-20990101'
        law.mkdir(exist_ok=True)
        (law / 'corpus.dump').write_bytes(b'synthetic law dump')
        return staging, law

    def test_interrupted_release_phase_ends_the_run_before_the_next_phase(self):
        app = self.app()
        calls = self.stub(app, corpus=INTERRUPTED)
        self.assertEqual(app.run(), 1)
        self.assertEqual(calls, ['secrets', 'images', 'corpus'])
        app = self.app()
        for name in ('corpus', 'frameworks'):
            with self.subTest(name=name), mock.patch('updater.deploy_run.Corpus') as phase:
                phase.return_value.run.return_value = False
                phase.return_value.interrupted = True
                self.assertEqual(getattr(app, name)(self.state, self.paths.state, 'activate', None), INTERRUPTED)
                phase.return_value.interrupted = False
                self.assertIs(getattr(app, name)(self.state, self.paths.state, 'activate', None), False)

    def test_staging_is_discarded_on_every_locked_run(self):
        cases = [('success', True, {}), ('secrets failure', True, dict(secrets=False)),
                 ('image exception', True, dict(images=ValueError('synthetic image failure'))),
                 ('law failure', True, dict(corpus=False)), ('without block', False, {})]
        for name, block, outcomes in cases:
            with self.subTest(name=name):
                staging, law = self.staged()
                app = self.app(block)
                self.stub(app, **outcomes)
                app.run()
                self.assertFalse(staging.exists())
                self.assertTrue((law / 'corpus.dump').exists())
        staging, _ = self.staged()
        marker = self.paths.backup_lock.with_name('active-job')
        marker.touch()
        self.assertEqual(self.app().run(), 0)
        self.assertFalse(staging.exists())
        marker.unlink()
        staging, _ = self.staged()
        app = self.app()
        self.stub(app, corpus=Crash())
        with self.assertRaises(Crash):
            app.run()
        self.assertFalse(staging.exists())

    def test_busy_run_leaves_staging_alone(self):
        staging, _ = self.staged()
        with self.paths.lock.open('a') as lock:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            self.assertEqual(self.app().run(), 0)
        self.assertTrue(staging.exists())

    def test_discard_never_follows_symlinks(self):
        releases = self.paths.root / 'srv/trachuan/data/releases'
        releases.mkdir(parents=True)
        outside = self.paths.root / 'outside'
        (outside / 'frameworks-20990103').mkdir(parents=True)
        (outside / 'frameworks-20990103' / 'keep').write_bytes(b'x')
        (outside / 'keep').write_bytes(b'x')
        (releases / 'frameworks-20990102').symlink_to(outside)
        (releases / 'frameworks-20990104').write_bytes(b'stray file')
        app = self.app()
        self.stub(app)
        app.run()
        self.assertFalse((releases / 'frameworks-20990102').is_symlink())
        self.assertFalse((releases / 'frameworks-20990104').exists())
        self.assertTrue((outside / 'keep').exists())
        releases.rmdir()
        releases.symlink_to(outside)
        app.run()
        self.assertTrue((outside / 'frameworks-20990103' / 'keep').exists())

    def test_cleanup_failure_is_logged_and_fails_the_run(self):
        app = self.app()
        self.stub(app)
        with mock.patch.object(FRAMEWORKS, 'discard', side_effect=OSError('synthetic')):
            self.assertEqual(app.run(), 1)
        lines = [json.loads(line) for line in self.paths.log.read_text().splitlines()]
        self.assertIn(('warning', 'cleanup', 'frameworks cleanup failed'),
                      [(line.get('status'), line.get('step'), line.get('message')) for line in lines])

    def test_policy_without_block_removes_stale_frameworks_result_and_marker(self):
        result = self.paths.result.with_name('release-frameworks.json')
        marker = self.paths.state / 'frameworks-prune.json'
        for block in (True, False):
            with self.subTest(block=block):
                result.write_text('{"status":"failed"}')
                marker.write_text('"2099-01-01"')
                app = self.app(block)
                self.stub(app, images=ValueError('synthetic image failure'))
                app.run()
                self.assertEqual(result.exists(), block)
                self.assertEqual(marker.exists(), block)

    def test_frameworks_runs_after_law_and_each_failure_is_independent(self):
        app = self.app()
        self.assertEqual(app.phases, ('secrets', 'images', 'corpus', 'frameworks'))
        for law, frameworks, code in ((False, True, 1), (True, False, 1), (None, None, 0), (True, True, 0)):
            calls = []
            outcomes = dict(secrets=True, images=None, corpus=law, frameworks=frameworks)
            for name in app.phases:
                setattr(app, name, lambda *args, phase=name: calls.append(phase) or outcomes[phase])
            with self.subTest(law=law, frameworks=frameworks):
                self.assertEqual(app.run(), code)
                self.assertEqual(calls, ['secrets', 'images', 'corpus', 'frameworks'])

    def test_secrets_failure_still_stops_before_release_phases(self):
        app = self.app()
        calls = []
        for name in app.phases:
            setattr(app, name, lambda *args, phase=name: calls.append(phase) or (phase != 'secrets'))
        self.assertEqual(app.run(), 1)
        self.assertEqual(calls, ['secrets'])

    def test_without_block_phase_writes_nothing_and_keeps_frameworks_state(self):
        frameworks = dict(accepted={'3': {'hash': 'c' * 64, 'status': 'active'}}, retained=[], failed=['d' * 64])
        self.state.data['frameworks'] = frameworks
        self.state.save()
        app = self.app(block=False)
        self.assertIsNone(app.frameworks(self.state, self.paths.state, 'activate', None))
        app.registry = mock.Mock()
        app.registry.fetch.return_value = self.new.directory
        app.registry.latest_rollback.return_value = None
        app.phases = ('secrets', 'images', 'frameworks')
        with mock.patch('updater.deploy_run.Secrets.run', return_value=True), \
             mock.patch('updater.deploy_run.Verifier.images'):
            self.assertEqual(app.run(), 0)
        state = State(self.state.path)
        self.assertEqual(state.data['active'], self.new.hash)
        self.assertEqual(state.data['frameworks'], frameworks)
        self.assertFalse(self.paths.result.with_name('release-frameworks.json').exists())
        app.registry.fetch.assert_called_once()

    def test_stopped_journal_skips_frameworks_without_result(self):
        app = self.app()
        self.state.data['journal'] = dict(target=self.new.hash, previous=self.old.hash, baseline=7,
                                          restart_postgres=False, status='stopped', step='stopped', authorization=None)
        with mock.patch('updater.deploy_run.Corpus') as phase:
            self.assertFalse(app.frameworks(self.state, self.paths.state, 'activate', None))
            phase.assert_not_called()
        self.assertFalse(self.paths.result.with_name('release-frameworks.json').exists())

    def test_frameworks_phase_uses_frameworks_kind(self):
        app = self.app()
        with mock.patch('updater.deploy_run.Corpus') as phase:
            phase.return_value.run.return_value = True
            self.assertTrue(app.frameworks(self.state, self.paths.state, 'verify', None))
        self.assertEqual(phase.call_args.kwargs['kind'].name, 'frameworks')
        with mock.patch('updater.deploy_run.Corpus') as phase:
            app.corpus(self.state, self.paths.state, 'verify', None)
        self.assertEqual(phase.call_args.kwargs.get('kind', None), None)
        self.assertEqual(json.loads(self.paths.policy.read_bytes())['frameworks'], BLOCK)


if __name__ == '__main__':
    unittest.main()
