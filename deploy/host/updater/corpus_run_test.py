from pathlib import Path
import unittest
from unittest import mock

from updater import activation_test as fixture
from updater.corpus_verify_test import manifest, description
from updater.corpus_activation_test import Database, Crash
from updater.deploy_io import canonical, CommandError
from updater import deploy_corpus as c
from updater.deploy_corpus_result import ReleaseResult
from updater.deploy_run import Updater


class CorpusRunTest(fixture.ActivationFixture, unittest.TestCase):
    def setUp(self):
        super().setUp()
        self.m = manifest()
        self.directory = self.paths.state/'corpus-fixture'
        self.directory.mkdir()
        self.directory.joinpath('manifest.json').write_bytes(canonical(self.m))
        self.directory.joinpath('manifest.sigstore.json').write_bytes(b'{}')
        self.db = Database()
        self.db.check_extensions = mock.Mock()
        self.db.describe = mock.Mock(return_value=description(self.m))
        self.registry = mock.Mock()
        self.registry.fetch.return_value = self.directory
        self.prepare = mock.Mock(return_value=True)
        self.phase = c.Corpus(self.state, self.store, self.host, self.rules, self.registry,
                              database=self.db, prepare=self.prepare, clock=lambda: 100000)
        self.phase.resources = mock.Mock()
        self.phase.download = mock.Mock(return_value=Path('/synthetic/dump'))

    def run_phase(self, mode='activate'):
        return self.phase.run(self.paths.state/'run', mode)

    def result(self):
        import json
        return json.loads(self.phase.result.path.read_bytes())

    def test_unpublished_corpus_package_is_current_until_a_manifest_was_accepted(self):
        for reason in ('denied', 'not_found'):
            with self.subTest(reason=reason):
                self.registry.fetch.side_effect = CommandError('package unreadable', reason=reason)
                self.assertTrue(self.run_phase())
                self.assertEqual(self.result()['status'], 'current')
                self.assertEqual(self.result()['message'], 'no corpus release published')
        self.state.data['accepted']['corpus']['1'] = {'hash': 'a' * 64, 'status': 'active'}
        self.registry.fetch.side_effect = CommandError('package unreadable', reason='denied')
        self.assertFalse(self.run_phase())
        self.assertEqual(self.result()['status'], 'failed')
        self.registry.fetch.side_effect = CommandError('registry failed')
        self.state.data['accepted']['corpus'].clear()
        self.assertFalse(self.run_phase())

    def test_missing_mount_waits_without_any_activator_connection(self):
        for mode in ('activate', 'verify'):
            for missing in ('/usr/local/bin/provision-activator.sh', '/run/secrets/activator_db_password'):
                with self.subTest(mode=mode, missing=missing):
                    def runner(args, **kwargs):
                        if args[-3:] == ['test', '-r', missing]:
                            raise CommandError('unreadable mount', returncode=1)
                        return self.runner(args, **kwargs)
                    self.host.runner = runner
                    self.db.lock = mock.Mock(side_effect=CommandError('activator must not connect'))
                    self.assertTrue(self.run_phase(mode))
                    self.assertEqual(self.result()['status'], 'waiting')
                    self.assertEqual(self.result()['message'], 'activator not provisioned')
                    self.db.lock.assert_not_called()
                    self.registry.fetch.assert_not_called()
                    self.prepare.assert_not_called()

    def test_next_corpus_run_repairs_first_upgrade_from_previous_updater(self):
        role = dict(exists=False, grants=False, migration_skipped_grants=False)
        def runner(args, **kwargs):
            if 'start' in args and 'trachuan-migrate.service' in args:
                role['migration_skipped_grants'] = not role['exists']
                role['grants'] = role['exists']
                self.schema = 8
            if 'bash' in args and args[-1] == '/usr/local/bin/provision-activator.sh':
                role.update(exists=True, grants=True)
            return self.runner(args, **kwargs)
        self.host.runner = runner
        def previous_migrate():
            self.host.systemctl('start', 'queryembed')
            self.host.systemctl('start', 'migrate')
            self.assertEqual(self.host.user('systemctl', '--user', 'show', '--property=Result', '--value',
                                           'trachuan-migrate.service').strip(), b'success')
        with mock.patch.object(self.host, 'migrate', previous_migrate):
            self.engine.activate(self.new)
        self.assertTrue(role['migration_skipped_grants'])
        self.assertEqual(self.state.data['schemas'][self.new.hash], 8)
        self.assertFalse(role['exists'])
        lock = self.db.lock
        def connect():
            if not role['exists'] or not role['grants']:
                raise CommandError('activator role or grants missing')
            return lock()
        self.db.lock = mock.Mock(side_effect=connect)
        self.assertTrue(self.run_phase())
        self.assertEqual(self.result()['status'], 'ok')
        self.assertTrue(role['exists'])
        self.assertTrue(role['grants'])
        self.db.lock.assert_called()

    def test_waiting_stays_pending_and_keeps_original_timestamp(self):
        self.db.describe.return_value['dense_query_hash'] = 'f'*64
        self.assertTrue(self.run_phase())
        first = self.result()
        self.assertEqual(first['status'], 'waiting')
        self.assertEqual(self.state.decision('corpus', first['manifest_hash'], self.m['sequence']), 'pending')
        self.phase.clock = lambda: 190000
        self.assertTrue(self.run_phase())
        self.assertEqual(self.result()['waiting_since'], first['waiting_since'])
        self.phase.download.assert_not_called()
        self.assertFalse(self.db.rows)

    def test_new_activation_current_noop_and_daily_prune(self):
        self.assertTrue(self.run_phase())
        self.assertEqual(self.result()['status'], 'ok')
        self.assertEqual(self.result()['rel'], self.m['rel'])
        self.assertEqual(self.result()['mode'], 'new')
        records = list(self.state.data['accepted']['corpus'].values())
        self.assertEqual(records[0]['status'], 'active')
        self.phase.download.reset_mock()
        self.assertTrue(self.run_phase())
        self.phase.download.assert_not_called()
        self.assertEqual(self.result()['status'], 'current')
        self.assertEqual(self.result()['mode'], 'new')

    def test_waiting_age_survives_transient_failure_and_process_restart(self):
        self.db.describe.return_value['dense_query_hash'] = 'f'*64
        self.assertTrue(self.run_phase())
        first = self.result()['waiting_since']
        self.phase.result.write('failed', 'fetch', 'transient failure', self.phase.hash, self.m, 'new')
        self.phase.result = ReleaseResult(self.paths, lambda: 200000)
        self.assertTrue(self.run_phase())
        self.assertEqual(self.result()['waiting_since'], first)

    def test_verify_binds_sequence_but_does_not_download_or_change_databases(self):
        self.assertTrue(self.run_phase('verify'))
        self.assertEqual(self.result()['status'], 'verified')
        self.assertFalse(self.db.rows)
        self.phase.download.assert_not_called()
        self.prepare.assert_not_called()

    def test_low_resources_and_extension_mismatch_refuse_before_download(self):
        for target in (self.phase.resources, self.db.check_extensions):
            target.side_effect = ValueError('refused')
            self.assertFalse(self.run_phase())
            self.assertEqual(self.result()['status'], 'failed')
            self.phase.download.assert_not_called()
            target.side_effect = None

    def test_interrupted_activation_resumes_before_fetching_new_candidate(self):
        self.db.after = 'restored'
        with self.assertRaises(Crash):
            self.run_phase()
        self.registry.fetch.side_effect = CommandError('registry unavailable')
        self.registry.fetch.reset_mock()
        self.assertTrue(self.run_phase())
        self.registry.fetch.assert_not_called()
        self.assertEqual(self.db.active, self.m['database'])

    def test_complete_journal_only_finishes_bookkeeping_after_crash(self):
        self.db.after = 'activated'
        with self.assertRaises(Crash):
            self.run_phase()
        self.phase.download.reset_mock()
        self.assertTrue(self.run_phase())
        self.phase.download.assert_not_called()
        self.assertEqual(len([r for r in self.db.rows if r['step']=='activated']), 1)

    def test_committed_transaction_with_lost_response_is_not_aborted(self):
        activate = self.db.activate
        def lost_response(row):
            activate(row)
            raise ValueError('lost activation response')
        self.db.activate = lost_response
        self.assertTrue(self.run_phase())
        self.assertEqual(self.db.operations()[0]['step'], 'activated')
        self.assertEqual(self.result()['status'], 'ok')

    def test_resume_rechecks_target_with_current_server_after_completed_smoke(self):
        self.db.after = 'checked'
        with self.assertRaises(Crash):
            self.run_phase()
        self.db.smoke = mock.Mock()
        self.assertTrue(self.run_phase())
        self.db.smoke.assert_called_once()

    def test_sequence_reuse_and_older_sequence_refused(self):
        self.state.accept('corpus', 'f'*64, self.m['sequence'])
        self.assertFalse(self.run_phase())
        self.assertIn('sequence reuse', self.result()['message'])
        self.m['sequence'] -= 1
        self.directory.joinpath('manifest.json').write_bytes(canonical(self.m))
        self.assertFalse(self.run_phase())
        self.assertIn('older sequence', self.result()['message'])

    def test_acceptance_supersedes_lower_pending_corpus_without_changing_image(self):
        self.state.accept('corpus', 'f'*64, 1)
        active_image = self.state.data['active']
        self.assertTrue(self.run_phase())
        self.assertEqual(self.state.data['accepted']['corpus']['1']['status'], 'failed')
        self.assertEqual(self.state.data['active'], active_image)

    def test_three_phase_order_and_corpus_failure_preserves_image_result(self):
        paths = self.paths
        paths.policy.parent.mkdir(parents=True, exist_ok=True)
        paths.policy.write_bytes(canonical(self.rules))
        paths.mode.write_text('activate')
        paths.lock.parent.mkdir(parents=True, exist_ok=True)
        paths.backup_lock.parent.mkdir(parents=True, exist_ok=True)
        paths.backup_lock.touch()
        app = Updater(paths, self.runner, user_prefix=[])
        self.assertEqual(app.phases, ('secrets','images','corpus'))
        calls = []
        for name in app.phases:
            setattr(app, name, lambda *args, phase=name: calls.append(phase) or (phase != 'corpus'))
        self.assertEqual(app.run(), 1)
        self.assertEqual(calls, ['secrets','images','corpus'])

    def test_resource_thresholds(self):
        with mock.patch('updater.deploy_corpus.Path.read_text', return_value='MemAvailable: 2441407 kB\n'), \
             mock.patch('updater.deploy_corpus.shutil.disk_usage', return_value=(0,0,10000000000)):
            c.Corpus.resources(self.phase)
        with mock.patch('updater.deploy_corpus.Path.read_text', return_value='MemAvailable: 2441406 kB\n'), \
             mock.patch('updater.deploy_corpus.shutil.disk_usage', return_value=(0,0,10000000000)):
            with self.assertRaises(ValueError):
                c.Corpus.resources(self.phase)
        with mock.patch('updater.deploy_corpus.Path.read_text', return_value='MemAvailable: 2441407 kB\n'), \
             mock.patch('updater.deploy_corpus.shutil.disk_usage', return_value=(0,0,9999999999)):
            with self.assertRaises(ValueError):
                c.Corpus.resources(self.phase)
