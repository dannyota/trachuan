import base64
import json
from pathlib import Path
from unittest import mock
import unittest

from updater import activation_test as fixture
from updater.deploy_io import CommandError
from updater.deploy_state import State
from updater.deploy_secrets import Secrets, consumers


class SecretsFixture(fixture.ActivationFixture):
    def setUp(self):
        super().setUp()
        self.rules['vault_id'] = 'ocid1.vault.oc1.ap-singapore-1.synthetic'
        self.rules['region'] = 'ap-singapore-1'
        self.rules['secrets'] = ['google_client_secret', 'smtp_username', 'smtp_password',
                                'account_keys', 'postgres_password', 'auth_db_password',
                                'corpus_reader_password', 'activator_db_password',
                                'backup_s3_key', 'backup_s3_key_secret', 'server_signup_emails']
        self.values = {name: b'synthetic-' + name.encode() for name in self.rules['secrets']}
        self.values['auth_db_password'] = b'synthetic:/?#@ %'
        self.versions = dict.fromkeys(self.values, 1)
        self.installed = dict(self.values, server_auth_database_url=b'existing-url',
                              server_corpus_database_url=b'existing-corpus-url')
        self.vault_error = None
        self.fail_create = None
        self.secret_calls = []
        for path in (Path(__file__).parents[1] / 'quadlet').glob('*.container'):
            self.paths.quadlet.joinpath(path.name).write_bytes(path.read_bytes())
        self.host.runner = self.secret_runner
        self.reconcile = Secrets(self.state, self.host, self.rules, clock=lambda: 10000)

    def secret_runner(self, args, input=None, **kwargs):
        self.secret_calls.append((args, input, kwargs))
        if len(args) > 2 and Path(args[2]).name == 'deploy_vault.py':
            name = args[-1]
            if name == self.vault_error:
                raise CommandError('synthetic secret value must not escape', code='NotAuthorizedOrNotFound')
            return name.encode() + b'\0' + str(self.versions[name]).encode() + b'\0' + base64.b64encode(self.values[name]) + b'\0'
        if args[:3] == ['podman', 'secret', 'exists']:
            if args[-1] not in self.installed:
                raise CommandError('missing', returncode=1)
            return b''
        if args[:3] == ['podman', 'secret', 'inspect']:
            value = self.installed[args[-1]]
            if '--format' in args:
                return value + b'\n'
            return json.dumps([{'SecretData': value.decode('utf-8')}]).encode() + b'\n'
        if args[:3] == ['podman', 'secret', 'create']:
            if args[-2] == self.fail_create:
                raise CommandError('synthetic secret value must not escape')
            self.installed[args[-2]] = input
            return b'synthetic-id\n'
        return self.runner(args, input=input, **kwargs)

    def versions_recorded(self):
        return State(self.state.path).data['secrets']['versions']

    def changed(self, name='google_client_secret'):
        self.state.data['secrets']['versions'] = dict(self.versions)
        self.versions[name] = 2

    def restarts(self):
        return [args for args, _, _ in self.secret_calls if 'systemctl' in args]

    def assert_safe(self):
        for path in (self.paths.result, self.paths.result.with_name('secrets.json'), self.paths.log, self.state.path):
            if path.exists():
                for value in self.values.values():
                    self.assertNotIn(value, path.read_bytes())
                self.assertNotIn(b'synthetic secret value must not escape', path.read_bytes())
        for args, _, _ in self.secret_calls:
            for value in self.values.values():
                self.assertNotIn(value, ' '.join(args).encode())


class SecretsTest(SecretsFixture, unittest.TestCase):
    def test_adoption_equal_records_without_restart_or_replacement(self):
        self.assertTrue(self.reconcile.run())
        self.assertEqual(self.versions_recorded(), self.versions)
        self.assertEqual(self.restarts(), [])
        self.assertFalse(any('create' in args for args, _, _ in self.secret_calls))

    def test_adoption_reads_json_without_template_newline(self):
        name = 'google_client_secret'
        value = self.values[name]
        self.assertFalse(value.endswith(b'\n'))
        self.assertTrue(self.reconcile.run())
        self.assertEqual(self.versions_recorded()[name], self.versions[name])
        self.assertEqual(self.restarts(), [])
        inspect = [args for args, _, _ in self.secret_calls if args[:3] == ['podman', 'secret', 'inspect']]
        self.assertTrue(inspect)
        self.assertTrue(all(args == ['podman', 'secret', 'inspect', '--showsecret', args[-1]] for args in inspect))
        self.assertEqual(json.loads(self.secret_runner(inspect[0]))[0]['SecretData'].encode(), value)
        self.assertEqual(self.secret_runner(['podman', 'secret', 'inspect', '--showsecret',
                                             '--format', '{{.SecretData}}', name]), value + b'\n')

    def test_adoption_preserves_account_key_whitespace_exactly(self):
        name = 'account_keys'
        value = b' synthetic-account-key\n'
        self.values[name] = value
        self.installed[name] = value
        self.assertTrue(self.reconcile.run())
        self.assertEqual(self.restarts(), [])
        self.assertEqual(self.installed[name], value)
        self.assert_safe()

    def test_adoption_replaces_extra_or_missing_newlines(self):
        name = 'account_keys'
        for current, installed in ((b'synthetic-key', b'synthetic-key\n'),
                                   (b'synthetic-key\n', b'synthetic-key')):
            with self.subTest(newline=current.endswith(b'\n')):
                self.setUp()
                self.values[name] = current
                self.installed[name] = installed
                self.assertTrue(self.reconcile.run())
                self.assertEqual(self.installed[name], current)
                self.assertTrue(self.restarts())
                self.assert_safe()

    def test_adoption_different_replaces_and_recreates_consumer_before_commit(self):
        self.installed['google_client_secret'] = b'different-synthetic'
        self.assertTrue(self.reconcile.run())
        self.assertEqual(self.installed['google_client_secret'], self.values['google_client_secret'])
        self.assertTrue(any('trachuan-server.service' in args for args in self.restarts()))
        self.assertEqual(self.versions_recorded(), self.versions)

    def test_version_change_replaces_even_equal_value(self):
        self.changed()
        self.assertTrue(self.reconcile.run())
        self.assertTrue(any(args[-2:] == ['google_client_secret', '-'] for args, _, _ in self.secret_calls))
        self.assertEqual(self.versions_recorded()['google_client_secret'], 2)

    def test_missing_base_and_derived_each_trigger_replacement(self):
        for missing in ('auth_db_password', 'server_auth_database_url'):
            with self.subTest(missing=missing):
                self.setUp()
                self.state.data['secrets']['versions'] = dict(self.versions)
                del self.installed[missing]
                self.assertTrue(self.reconcile.run())
                self.assertEqual(self.installed['server_auth_database_url'],
                                 b'postgres://auth_owner:synthetic%3A%2F%3F%23%40%20%25@127.0.0.1:5432/auth?sslmode=disable')
                self.assertTrue(any('trachuan-postgres.service' in args for args in self.restarts()))

    def test_corpus_url_preserves_database_placeholder_and_quotes_password(self):
        self.changed('corpus_reader_password')
        self.values['corpus_reader_password'] = b'synthetic:/?#@ %'
        self.assertTrue(self.reconcile.run())
        self.assertEqual(self.installed['server_corpus_database_url'],
                         b'postgres://corpus_reader:synthetic%3A%2F%3F%23%40%20%25@127.0.0.1:5432/{database}?sslmode=disable')

    def test_half_done_stays_pending_and_retry_does_not_adopt(self):
        self.changed('auth_db_password')
        self.fail_create = 'server_auth_database_url'
        self.assertFalse(self.reconcile.run())
        state = State(self.state.path)
        self.assertEqual(state.data['secrets']['versions']['auth_db_password'], 1)
        self.assertIn('auth_db_password', state.data['secrets']['pending'])
        self.assertTrue(self.paths.hold.exists())
        self.fail_create = None
        self.reconcile = Secrets(state, self.host, self.rules, clock=lambda: 10001)
        self.assertTrue(self.reconcile.run())
        self.assertEqual(self.versions_recorded()['auth_db_password'], 2)
        self.assertFalse(self.paths.hold.exists())

    def test_sdk_failure_keeps_every_current_value_and_names_code_without_values(self):
        self.changed()
        before = dict(self.installed)
        self.vault_error = 'smtp_password'
        self.assertTrue(self.reconcile.run())
        self.assertTrue(self.reconcile.failed)
        self.assertEqual(self.installed, before)
        result = json.loads(self.paths.result.with_name('secrets.json').read_bytes())
        self.assertEqual(result['status'], 'failed')
        self.assertEqual(result['step'], 'secrets')
        self.assertIn('smtp_password', result['message'])
        self.assertIn('NotAuthorizedOrNotFound', result['message'])
        self.assertEqual(self.restarts(), [])
        self.assert_safe()

    def test_replacement_values_and_urls_only_travel_in_stdin(self):
        self.changed('auth_db_password')
        self.assertTrue(self.reconcile.run())
        self.assert_safe()
        for args, data, _ in self.secret_calls:
            if data and args[:3] == ['podman', 'secret', 'create']:
                self.assertEqual(args[-1], '-')
                self.assertNotIn(data, ' '.join(args).encode())

    def test_consumers_are_read_from_rendered_secret_lines(self):
        self.paths.quadlet.joinpath('trachuan-web.container').write_text(
            '[Container]\nSecret=google_client_secret,type=env,target=SYNTHETIC\n')
        self.assertEqual(consumers(self.paths.quadlet, {'google_client_secret'}), {'server', 'web'})
        self.changed()
        self.assertTrue(self.reconcile.run())
        self.assertTrue(any('trachuan-web.service' in args for args in self.restarts()))

    def test_postgres_wait_check_prewarm_loopback_then_edge(self):
        self.changed('backup_s3_key')
        self.assertTrue(self.reconcile.run())
        calls = self.secret_calls
        restart = next(i for i, (args, _, _) in enumerate(calls) if 'restart' in args and 'trachuan-postgres.service' in args)
        wait = next(i for i, (args, _, _) in enumerate(calls) if '/usr/local/bin/wait-postgres.sh' in args)
        check = next(i for i, (args, _, _) in enumerate(calls) if 'pgbackrest' in args)
        prewarm = next(i for i, (_, data, _) in enumerate(calls) if data and b'pg_prewarm' in data)
        ready = next(i for i, (args, _, _) in enumerate(calls) if args[-1] == 'http://127.0.0.1:8080/readyz')
        edge = next(i for i, (args, _, _) in enumerate(calls) if args[-1] == 'https://trachuan.vn/')
        self.assertEqual(sorted((restart, wait, check, prewarm, ready, edge)), [restart, wait, check, prewarm, ready, edge])

    def test_cadence_and_forced_activation(self):
        self.assertTrue(self.reconcile.run())
        self.secret_calls.clear()
        self.reconcile = Secrets(self.state, self.host, self.rules, clock=lambda: 13599)
        self.assertTrue(self.reconcile.run())
        self.assertEqual(self.secret_calls, [])
        self.assertTrue(self.reconcile.run(force=True))
        self.assertTrue(self.secret_calls)
        self.secret_calls.clear()
        self.reconcile = Secrets(self.state, self.host, self.rules, clock=lambda: 17199)
        self.assertTrue(self.reconcile.run())
        self.assertTrue(self.secret_calls)

    def test_placeholder_disables_only_secrets_and_no_sdk_call(self):
        self.rules['vault_id'] = 'ocid1.vault.REPLACE'
        self.assertTrue(self.reconcile.run(force=True))
        self.assertTrue(self.reconcile.failed)
        self.assertEqual(self.secret_calls, [])

    def test_unconsumed_secret_retry_releases_only_its_own_hold(self):
        self.changed('activator_db_password')
        self.fail_create = 'activator_db_password'
        self.assertFalse(self.reconcile.run())
        self.assertTrue(self.paths.hold.exists())
        self.fail_create = None
        self.assertTrue(self.reconcile.run())
        self.assertFalse(self.paths.hold.exists())
        self.assertEqual(self.versions_recorded()['activator_db_password'], 2)

    def test_sdk_failure_with_pending_blocks_images_and_keeps_hold(self):
        self.changed()
        self.fail_create = 'google_client_secret'
        self.assertFalse(self.reconcile.run())
        self.vault_error = 'smtp_password'
        self.assertFalse(self.reconcile.run())
        self.assertTrue(self.paths.hold.exists())

    def test_edge_failure_preserves_pending_and_hold(self):
        self.changed()
        runner = self.host.runner
        def fail_edge(args, **kwargs):
            if args[-1] == 'https://trachuan.vn/':
                raise CommandError('synthetic edge failure')
            return runner(args, **kwargs)
        self.host.runner = fail_edge
        self.assertFalse(self.reconcile.run())
        self.assertEqual(self.versions_recorded()['google_client_secret'], 1)
        self.assertTrue(self.paths.hold.exists())

    def test_missing_rendered_unit_fails_before_replacement(self):
        self.changed()
        self.paths.quadlet.joinpath('trachuan-server.container').unlink()
        self.assertTrue(self.reconcile.run())
        self.assertTrue(self.reconcile.failed)
        self.assertFalse(any('create' in args for args, _, _ in self.secret_calls))
        self.assertEqual(self.versions_recorded().get('google_client_secret'), None)

    def test_failed_completion_save_keeps_pending_and_blocks_images(self):
        self.changed()
        self.state.save()
        save = self.state.save
        def fail_completion():
            if not self.state.data['secrets']['pending']:
                raise OSError('synthetic completion failure')
            save()
        with mock.patch.object(self.state, 'save', side_effect=fail_completion):
            self.assertFalse(self.reconcile.run())
        self.assertEqual(self.versions_recorded()['google_client_secret'], 1)
        self.assertTrue(State(self.state.path).data['secrets']['pending'])
        self.assertTrue(self.paths.hold.exists())

    def test_caddy_stop_failure_still_reports_pending_without_escaping(self):
        self.changed()
        runner = self.host.runner
        def fail_stop(args, **kwargs):
            if 'stop' in args and 'trachuan-caddy.service' in args:
                raise CommandError('synthetic stop failed')
            return runner(args, **kwargs)
        self.host.runner = fail_stop
        self.assertFalse(self.reconcile.run())
        self.assertTrue(State(self.state.path).data['secrets']['pending'])
        self.assertTrue(self.paths.hold.exists())

    def test_existing_stopped_hold_is_not_cleared(self):
        self.changed()
        self.paths.hold.touch()
        self.assertTrue(self.reconcile.run())
        self.assertTrue(self.paths.hold.exists())
        self.assertFalse(any('start' in args and 'trachuan-caddy.service' in args for args in self.restarts()))


if __name__ == '__main__':
    unittest.main()
