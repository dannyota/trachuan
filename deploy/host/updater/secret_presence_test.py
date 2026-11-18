import hashlib
import io
import json
import tarfile
import unittest
from unittest import mock

from updater import deploy_run
from updater import secrets_test as fixture
from updater.deploy_activate import Activator
from updater.deploy_io import canonical, CommandError
from updater.deploy_state import State
from updater.deploy_store import Candidate


UNITS = {
    'quadlet/trachuan-server.container': (
        b'[Unit]\nSecret=ignored_unit\n[Container]\n'
        b' Secret=server_frameworks_key,type=mount,uid=65532,mode=0400 \n'
        b'Secret=server_frameworks_key,type=env,target=SYNTHETIC\n'
        b'Secret=server_auth_database_url,type=env,target=DATABASE_URL\n'
        b'[Service]\nSecret=ignored_service\n'),
    'quadlet/trachuan-web.container': b'[Container]\nSecret=smtp_username,type=env\n',
    'quadlet/nested/trachuan-extra.container': b'[Container]\nSecret=ignored_nested\n',
    'unrelated.container': b'[Container]\nSecret=ignored_file\n',
}
MISSING = 'secret missing: server_frameworks_key'


class SecretPresenceTest(fixture.SecretsFixture, unittest.TestCase):
    def setUp(self):
        super().setUp()
        self.paths.lock.parent.mkdir(parents=True, exist_ok=True)
        self.paths.backup_lock.parent.mkdir(parents=True, exist_ok=True)
        self.paths.backup_lock.touch()
        self.paths.mode.parent.mkdir(parents=True, exist_ok=True)
        self.paths.mode.write_text('activate\n')
        self.paths.policy.write_bytes(canonical(self.rules))
        del self.installed['server_frameworks_key']
        self.target = self.with_units(16)
        self.app = deploy_run.Updater(self.paths, self.secret_runner, user_prefix=[])
        self.app.phases = ('images',)
        self.app.registry = mock.Mock()
        self.app.registry.fetch.return_value = self.target.directory
        self.app.registry.latest_rollback.return_value = None
        patch = mock.patch('updater.deploy_run.Verifier.images')
        patch.start()
        self.addCleanup(patch.stop)

    def with_units(self, sequence):
        directory = self.paths.state / str(sequence)
        directory.mkdir()
        output = io.BytesIO()
        with tarfile.open(fileobj=output, mode='w:gz') as archive:
            for name, data in {**self.new.files, **UNITS}.items():
                entry = tarfile.TarInfo('deploy/host/' + name)
                entry.size = len(data)
                archive.addfile(entry, io.BytesIO(data))
        data = output.getvalue()
        (directory / 'host-bundle.tar.gz').write_bytes(data)
        manifest = dict(self.new.manifest, sequence=sequence,
                        host_bundle=dict(sha256=hashlib.sha256(data).hexdigest(), size=len(data)))
        (directory / 'manifest.json').write_bytes(canonical(manifest))
        (directory / 'manifest.sigstore.json').write_bytes(b'{}')
        return Candidate.read(directory, self.rules, self.runner)

    def rollback_artifact(self, current, target):
        directory = self.paths.state / 'rollback-fixture'
        directory.mkdir()
        data = canonical(dict(kind='trachuan.rollback', version=1, current=current, target=target, sequence=1))
        (directory / 'manifest.json').write_bytes(data)
        (directory / 'manifest.sigstore.json').write_bytes(b'{}')
        self.app.registry.latest_rollback.return_value = '1'
        self.app.registry.fetch.side_effect = (
            lambda repository, tag, path, kind: directory if kind == 'rollback' else self.new.directory)
        return hashlib.sha256(data).hexdigest()

    def probes(self, calls):
        return [args[-1] for args, _, _ in calls if args[:3] == ['podman', 'secret', 'exists']]

    def change(self, args):
        return ('systemctl' in args or (args[:2] == ['podman', 'exec'] and 'psql' not in args)
                or any(word.endswith(('install.sh', 'render-units.sh')) for word in args))

    def changes(self):
        return [args for args, _, _ in self.secret_calls if self.change(args)]

    def result(self):
        value = json.loads(self.paths.result.read_bytes())
        return value['status'], value['step'], value['message'], value['manifest_hash']

    def assert_stopped(self):
        state = State(self.state.path)
        self.assertEqual(state.data['active'], self.old.hash)
        self.assertIsNone(state.data['journal'])
        self.assertEqual(state.data['installed_bundle'], self.old.manifest['host_bundle']['sha256'])
        self.assertEqual(state.data['accepted']['image']['16'], {'hash': self.target.hash, 'status': 'pending'})
        self.assertFalse(self.paths.hold.exists())
        self.assertFalse(self.paths.host.joinpath('images.env').exists())
        self.assertEqual(self.changes(), [])
        self.assertEqual(self.result(), ('failed', 'secrets', MISSING, self.target.hash))

    def test_activation_proceeds_once_forced_reconciliation_creates_every_named_secret(self):
        self.assertEqual(self.app.run(), 0)
        self.assertEqual(State(self.state.path).data['active'], self.target.hash)
        self.assertEqual(self.installed['server_frameworks_key'], self.values['server_frameworks_key'])
        created = max(i for i, (args, _, _) in enumerate(self.secret_calls) if args[:3] == ['podman', 'secret', 'create'])
        changed = next(i for i, (args, _, _) in enumerate(self.secret_calls) if self.change(args))
        self.assertLess(created, changed)
        self.assertEqual(self.probes(self.secret_calls[created:changed]),
                         ['server_auth_database_url', 'server_frameworks_key', 'smtp_username'])
        self.assertEqual(self.result()[:3], ('ok', 'complete', 'release activated'))

    def test_missing_named_secret_stops_activation_before_any_container_change(self):
        with mock.patch('updater.deploy_run.Secrets.run', return_value=True):
            for attempt in range(2):
                with self.subTest(attempt=attempt):
                    self.assertEqual(self.app.run(), 1)
                    self.assert_stopped()
        self.assertEqual(self.probes(self.secret_calls), ['server_auth_database_url', 'server_frameworks_key'] * 2)

    def test_reconciliation_failure_then_missing_secret_keeps_serving_image(self):
        self.vault_error = 'server_frameworks_key'
        self.assertEqual(self.app.run(), 1)
        self.assert_stopped()
        secrets = json.loads(self.paths.result.with_name('secrets.json').read_bytes())
        self.assertEqual((secrets['status'], secrets['message']),
                         ('failed', 'server_frameworks_key: NotAuthorizedOrNotFound'))
        self.assert_safe()

    def test_secret_probe_error_fails_without_container_change(self):
        def unavailable(args, **kwargs):
            if args[:3] == ['podman', 'secret', 'exists']:
                raise CommandError('synthetic Podman unavailable', returncode=125)
            return self.secret_runner(args, **kwargs)
        self.app.host.runner = unavailable
        with mock.patch('updater.deploy_run.Secrets.run', return_value=True):
            self.assertEqual(self.app.run(), 1)
        state = State(self.state.path)
        self.assertEqual(state.data['active'], self.old.hash)
        self.assertIsNone(state.data['journal'])
        self.assertEqual(self.changes(), [])
        self.assertEqual(self.result()[:2], ('failed', 'secrets'))

    def test_rollback_to_image_naming_missing_secret_keeps_authorization_unconsumed(self):
        previous = self.with_units(13)
        self.state = State.bootstrap(self.state.path, previous.hash, 13, 7, previous.manifest['host_bundle']['sha256'])
        self.store.retain(previous)
        self.state.accept('image', self.new.hash, 15)
        Activator(self.state, self.store, self.host).activate(self.new)
        authorization = self.rollback_artifact(self.new.hash, previous.hash)
        journal = State(self.state.path).data['journal']
        self.secret_calls.clear()
        with mock.patch('updater.deploy_run.Secrets.run', return_value=True):
            self.assertEqual(self.app.run(), 1)
        state = State(self.state.path)
        self.assertEqual(state.data['active'], self.new.hash)
        self.assertEqual(state.data['journal'], journal)
        self.assertEqual(state.data['consumed'], [])
        self.assertEqual(state.data['accepted']['rollback'], {'1': {'hash': authorization, 'status': 'pending'}})
        self.assertEqual(self.changes(), [])
        self.assertEqual(self.result(), ('failed', 'secrets', MISSING, authorization))
