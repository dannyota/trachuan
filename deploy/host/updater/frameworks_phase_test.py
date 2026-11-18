import json
import unittest
from unittest import mock

from updater import activation_test as fixture
from updater.deploy_io import canonical
from updater.deploy_run import Updater
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
