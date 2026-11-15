from contextlib import redirect_stderr
from datetime import datetime, timezone
import hashlib
import io
import json
import unittest
from unittest import mock

from metrics.deploy_test import publisher
from updater import deploy_state
from updater import main
from updater.activation_test import ActivationFixture, Crash
from updater.deploy_host import pins
from updater.deploy_io import canonical
from updater.deploy_run import Updater
from updater.deploy_state import State
from updater.deploy_store import Candidate
from updater.deploy_verify import policy
from updater.publish import file
from updater.verification_test import archive


class ReleaseFloorTest(ActivationFixture, unittest.TestCase):
    def setUp(self):
        super().setUp()
        del self.state.data['accepted']['image']['15']
        self.state.save()
        self.paths.lock.parent.mkdir(parents=True, exist_ok=True)
        self.paths.backup_lock.parent.mkdir(parents=True, exist_ok=True)
        self.paths.backup_lock.touch()
        self.paths.mode.parent.mkdir(parents=True, exist_ok=True)
        self.paths.mode.write_text('verify\n')
        self.paths.policy.write_bytes(canonical(self.rules))
        self.app = Updater(self.paths, self.runner, user_prefix=[])
        self.app.registry = mock.Mock()
        self.app.registry.fetch.return_value = self.new.directory
        self.app.registry.latest_rollback.return_value = None
        self.app.phases = ('secrets', 'images')
        for target in ('updater.deploy_run.Secrets.run', 'updater.deploy_run.Verifier.images'):
            patch = mock.patch(target, return_value=True)
            patch.start()
            self.addCleanup(patch.stop)

    def floor_candidate(self, candidate, floor):
        rules = dict(self.rules, release_floor=floor)
        data = archive(server=candidate.files['server.env'], policy=rules)
        candidate.directory.joinpath('host-bundle.tar.gz').write_bytes(data)
        candidate.manifest['host_bundle'] = dict(sha256=hashlib.sha256(data).hexdigest(), size=len(data))
        candidate.directory.joinpath('manifest.json').write_bytes(canonical(candidate.manifest))
        return Candidate.read(candidate.directory, self.rules, self.runner)

    def test_floor_is_saved_with_activated_and_preserved_by_later_saves(self):
        candidate = self.floor_candidate(self.new, 15)
        self.state.accept('image', candidate.hash, 15)
        writes = []
        atomic = deploy_state.atomic
        def record(path, data):
            writes.append(json.loads(data))
            atomic(path, data)
        with mock.patch.object(deploy_state, 'atomic', side_effect=record):
            self.engine.activate(candidate)
        self.assertTrue(writes)
        for value in writes:
            if (value['journal'] or {}).get('status') == 'activated':
                self.assertEqual(value.get('release_floor'), 15)
            else:
                self.assertNotIn('release_floor', value)
        self.assertEqual(State(self.state.path).data.get('release_floor'), 15)
        for sequence, floor in ((16, 14), (17, None)):
            later = self.candidate(sequence, str(sequence).encode())
            if floor is not None:
                later = self.floor_candidate(later, floor)
            self.state.accept('image', later.hash, sequence)
            self.engine.activate(later)
            self.assertEqual(State(self.state.path).data['release_floor'], 15)
        authorization = 'e' * 64
        auth = dict(current=later.hash, target=candidate.hash, sequence=1)
        self.engine.activate(candidate, expected_schema=7, authorization=authorization, rollback=auth)
        self.assertEqual(State(self.state.path).data['release_floor'], 15)
        broken = self.candidate(18, b'broken')
        self.state.accept('image', broken.hash, 18)
        self.crash_step = 'migrate'
        with self.assertRaises(Crash):
            self.engine.activate(broken)
        self.assertEqual(self.engine.recover(), 'rolled_back')
        self.state.fail(broken.hash)
        self.assertEqual(State(self.state.path).data['release_floor'], 15)

    def test_own_bundle_floor_is_checked_before_installation(self):
        for floor in (0, -1, True, 1.5, '15', None):
            with self.subTest(invalid=floor):
                self.paths.policy.write_bytes(canonical(dict(self.rules, release_floor=floor)))
                with self.assertRaises(ValueError):
                    policy(self.paths.policy)
                with self.assertRaises(ValueError):
                    file(self.paths.policy, self.paths.state / 'published-policy', policy_file=True)
        self.paths.policy.write_bytes(canonical(dict(self.rules, release_floor=16)))
        self.assertEqual(policy(self.paths.policy)['release_floor'], 16)
        file(self.paths.policy, self.paths.state / 'published-policy', policy_file=True)
        with self.assertRaisesRegex(ValueError, 'release floor'):
            self.floor_candidate(self.new, 16)
        self.paths.mode.write_text('activate\n')
        self.assertEqual(self.app.run(), 1)
        self.assertIn('release floor', json.loads(self.paths.result.read_bytes())['message'])
        self.assertIsNone(State(self.state.path).data['journal'])
        self.assertFalse(any('install.sh' in ' '.join(args) for args, _ in self.calls))

    def test_image_acceptance_refuses_below_floor_but_corpus_is_unaffected(self):
        self.state.data['release_floor'] = 15
        self.state.save()
        state = State(self.state.path)
        before = self.state.path.read_bytes()
        for hash_value, sequence in ((self.old.hash, 14), ('c' * 64, 13)):
            with self.assertRaisesRegex(ValueError, 'release floor'):
                state.accept('image', hash_value, sequence)
        self.assertEqual(self.state.path.read_bytes(), before)
        self.assertEqual(state.accept('image', self.new.hash, 15), 'new')
        self.assertEqual(state.accept('corpus', 'd' * 64, 1), 'new')
        self.assertEqual(State(self.state.path).data['release_floor'], 15)

    def test_rollback_below_floor_is_refused_without_consumption(self):
        self.state.accept('image', self.new.hash, 15)
        self.engine.activate(self.new)
        self.state.data['release_floor'] = 15
        self.state.save()
        directory = self.paths.state / 'rollback'
        directory.mkdir()
        data = canonical(dict(kind='trachuan.rollback', version=1, current=self.new.hash,
                              target=self.old.hash, sequence=1))
        directory.joinpath('manifest.json').write_bytes(data)
        directory.joinpath('manifest.sigstore.json').write_bytes(b'{}')
        self.app.registry.latest_rollback.return_value = '1'
        self.app.registry.fetch.side_effect = lambda repo, tag, path, kind: directory if kind == 'rollback' else self.new.directory
        before = self.state.path.read_bytes()
        self.calls.clear()
        self.assertEqual(self.app.run(), 1)
        self.assertEqual(self.state.path.read_bytes(), before)
        self.assertNotIn(hashlib.sha256(data).hexdigest(), State(self.state.path).data['consumed'])
        self.assertIn('release floor', json.loads(self.paths.result.read_bytes())['message'])
        self.assertFalse(any('install.sh' in ' '.join(args) for args, _ in self.calls))

    def test_recovery_below_floor_stops_and_keeps_ingress_held(self):
        self.state.data['release_floor'] = 15
        self.state.save()
        self.state.accept('image', self.new.hash, 15)
        self.crash_step = 'migrate'
        with self.assertRaises(Crash):
            self.engine.activate(self.new)
        self.calls.clear()
        self.assertEqual(self.app.run(), 1)
        state = State(self.state.path)
        self.assertEqual(state.data['journal']['status'], 'stopped')
        self.assertEqual(state.data['release_floor'], 15)
        self.assertTrue(self.paths.hold.exists())
        self.assertIn('release floor', json.loads(self.paths.result.read_bytes())['message'])
        self.assertFalse(any('start' in args or 'install.sh' in ' '.join(args) for args, _ in self.calls))
        self.assertEqual(self.app.run(), 0)
        self.assertTrue(self.paths.hold.exists())
        self.assertEqual(State(self.state.path).data['journal']['status'], 'stopped')
        self.assertIn('release floor', json.loads(self.paths.result.read_bytes())['message'])

    def test_bootstrap_uses_maximum_floor_and_refuses_below_it(self):
        self.app.registry.find.return_value = self.old.directory
        self.paths.host.joinpath('images.env').write_bytes(pins(self.old.manifest))
        for name in ('server.env', 'web.env'):
            self.paths.host.joinpath(name).write_bytes(self.old.files[name])
        for stored, installed, argument, expected in ((None, 12, 13, 13), (14, 12, 13, 14),
                                                      (12, 14, 13, 14), (12, 13, 14, 14)):
            with self.subTest(stored=stored, installed=installed, argument=argument):
                self.state.data.pop('release_floor', None)
                if stored is not None:
                    self.state.data['release_floor'] = stored
                self.state.save()
                if stored is None:
                    self.state.path.unlink()
                self.paths.policy.write_bytes(canonical(dict(self.rules, release_floor=installed)))
                self.app.rules = policy(self.paths.policy)
                with mock.patch.object(main, 'Paths', return_value=self.paths), \
                     mock.patch.object(main, 'Updater', return_value=self.app), \
                     mock.patch.object(main.os, 'geteuid', return_value=0), \
                     mock.patch.object(main.os, 'umask'), \
                     mock.patch.object(main, 'trusted'), \
                     mock.patch.object(main.sys, 'argv', ['trachuan-deploy', 'bootstrap', self.old.hash,
                                                         '--release-floor', str(argument)]):
                    self.assertEqual(main.main(), 0)
                self.assertEqual(State(self.state.path).data['release_floor'], expected)
        for source in ('stored', 'installed', 'argument'):
            with self.subTest(refused=source):
                self.state.data['release_floor'] = 15 if source == 'stored' else 12
                self.state.save()
                self.app.rules = dict(self.rules, release_floor=15 if source == 'installed' else 12)
                before = self.state.path.read_bytes()
                self.calls.clear()
                self.assertEqual(self.app.bootstrap(self.old.hash, release_floor=15 if source == 'argument' else 12), 1)
                self.assertEqual(self.state.path.read_bytes(), before)
                self.assertIn('release floor', json.loads(self.paths.result.read_bytes())['message'])
                self.assertFalse(any('systemctl' in args for args, _ in self.calls))

    def test_result_and_metric_report_stored_floor_or_none(self):
        now = datetime(2026, 1, 2, tzinfo=timezone.utc)
        for floor in (None, 15):
            with self.subTest(floor=floor):
                if floor is not None:
                    self.state.data['release_floor'] = floor
                    self.state.save()
                self.assertEqual(self.app.run(), 0)
                result = json.loads(self.paths.result.read_bytes())
                self.assertIn('release_floor', result)
                self.assertEqual(result['release_floor'], floor)
                self.assertIn(('deploy_release_floor', floor or 0), publisher.deploy(self.paths.result, now))
        self.app.registry.fetch.side_effect = ValueError('synthetic fetch failure')
        self.assertEqual(self.app.run(), 1)
        self.assertEqual(json.loads(self.paths.result.read_bytes())['release_floor'], 15)
        self.paths.mode.write_text('invalid\n')
        self.assertEqual(self.app.run(), 1)
        self.assertEqual(json.loads(self.paths.result.read_bytes())['release_floor'], 15)
        self.paths.policy.write_bytes(b'{invalid')
        with mock.patch.object(main, 'Paths', return_value=self.paths), \
             mock.patch.object(main.os, 'geteuid', return_value=0), \
             mock.patch.object(main.os, 'umask'), \
             mock.patch.object(main, 'trusted'), \
             mock.patch.object(main.sys, 'argv', ['trachuan-deploy', 'run']), \
             redirect_stderr(io.StringIO()):
            self.assertEqual(main.main(), 1)
        self.assertEqual(json.loads(self.paths.result.read_bytes())['release_floor'], 15)

    def test_state_without_floor_round_trips_without_changing_shape(self):
        before = self.state.path.read_bytes()
        self.assertNotIn('release_floor', json.loads(before))
        state = State(self.state.path)
        state.save()
        self.assertEqual(self.state.path.read_bytes(), before)
        self.assertEqual(self.app.run(), 0)
        self.assertNotIn('release_floor', State(self.state.path).data)


if __name__ == '__main__':
    unittest.main()
