import unittest
from unittest import mock

from updater.deploy_corpus_sql import JournalRefused
from updater.deploy_io import canonical, CommandError, describe
from updater import deploy_corpus as c
from updater.deploy_corpus_result import ReleaseResult
from updater.deploy_registry import Registry
from updater.deploy_release_target import Leftover
from updater.deploy_run import Updater
from updater.frameworks_run_test import missing_tag
from updater.frameworks_verify_test import NESTED
from updater.world_test import Crash, LawCase, NOW


class CorpusRunTest(LawCase, unittest.TestCase):
    def setUp(self):
        super().setUp()
        self.baseline()

    def test_unpublished_corpus_package_is_current_until_a_manifest_was_accepted(self):
        self.state.data['accepted']['corpus'].clear()
        self.state.save()
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

    def test_missing_law_tag_keeps_failing_once_a_corpus_manifest_was_accepted(self):
        self.phase.registry = Registry(self.rules, missing_tag)
        self.assertFalse(self.run_phase())
        self.assertEqual(self.result()['status'], 'failed')
        self.assertTrue(self.result()['message'].startswith(
            'corpus command failed or run interrupted: oras resolve: exit 1: Error response from registry: '))

    def test_database_client_output_never_reaches_the_law_result_or_log(self):
        marker = 'invented dump marker'
        for program, code, message in (('pg_restore', 1, 'pg_restore: exit 1'), ('psql', 3, 'psql: exit 3')):
            with self.subTest(program=program):
                self.setUp()
                detail = describe(['podman', 'exec', 'trachuan-postgres', program], marker.encode(), code)
                self.db.failures['restore'] = CommandError('command failed', detail=detail)
                self.assertFalse(self.run_phase())
                self.assertEqual(self.result()['message'], 'corpus command failed or run interrupted: ' + message)
                self.assertNotIn(marker, self.phase.result.path.read_text())
                self.assertNotIn(marker, self.phase.result.log.read_text())

    def test_nested_law_manifest_ends_as_failed_result(self):
        self.directory.joinpath('manifest.json').write_bytes(NESTED)
        self.assertFalse(self.run_phase())
        self.assertEqual((self.result()['status'], self.result()['step'], self.result()['message']),
                         ('failed', 'manifest', 'invalid corpus manifest JSON'))

    def test_refused_journal_row_waits_at_recovery(self):
        self.db.operations = mock.Mock(side_effect=JournalRefused('invalid release journal row'))
        self.assertTrue(self.run_phase())
        self.assertEqual((self.result()['status'], self.result()['step'], self.result()['message']),
                         ('waiting', 'recovery', 'invalid release journal row'))
        self.assertIn('journal_row_refused', self.result()['integrity'])

    def test_missing_mount_waits_without_any_activator_connection(self):
        for mode in ('activate', 'verify'):
            for missing in ('/usr/local/bin/postgres-admin.sh', '/run/secrets/activator_db_password'):
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
                    self.assertEqual(self.db.lists_discarded, 0)

    def test_next_corpus_run_repairs_first_upgrade_from_previous_updater(self):
        role = dict(exists=False, grants=False, migration_skipped_grants=False)
        def runner(args, **kwargs):
            if 'start' in args and 'trachuan-migrate.service' in args:
                role['migration_skipped_grants'] = not role['exists']
                role['grants'] = role['exists']
                self.schema = 8
            if args[-4:] == ['bash', '-s', '--', '--no-auth-tables']:
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
        self.now = 190000 + NOW
        self.phase = self.make_phase()
        self.assertTrue(self.run_phase())
        self.assertEqual(self.result()['waiting_since'], first['waiting_since'])
        self.phase.download.assert_not_called()
        self.assertFalse(self.db.rows)

    def test_retained_activation_starts_with_a_sweep(self):
        oid = self.db.add(self.name)
        self.write_record(active=self.active_entry(),
                          retired=[dict(database=self.name, oid=oid, dump_sha256=self.dump,
                                        retired_at='2027-01-15T07:00:00Z')])
        self.m['previous'] = '20990105_1'
        self.write_manifest()
        self.assertTrue(self.run_phase())
        self.assertEqual(self.result()['mode'], 'retained')
        self.assertEqual(self.db.sweeps[0], 0)

    def test_verification_never_sweeps(self):
        self.assertTrue(self.run_phase('verify'))
        self.assertEqual(self.result()['status'], 'verified')
        self.assertEqual(self.db.sweeps, [])

    def test_leftovers_wait_without_spending_the_manifest(self):
        for reason in ('leftover database in use', 'leftover backend'):
            self.db.leftover = reason
            with self.subTest(reason=reason):
                self.assertTrue(self.run_phase())
                result = self.result()
                self.assertEqual((result['status'], result['step'], result['message']), ('waiting', 'leftovers', reason))
                self.assertFalse(self.db.rows)
                self.phase.download.assert_not_called()
                self.assertEqual(self.state.decision('corpus', result['manifest_hash'], self.m['sequence']), 'pending')
        self.db.leftover = None
        self.assertTrue(self.run_phase())
        self.assertEqual(self.result()['status'], 'ok')

    def test_leftover_during_the_operation_waits_and_the_next_run_resumes_it(self):
        create = self.db.create
        def refuse(row):
            self.db.create = create
            raise Leftover('leftover backend')
        self.db.create = refuse
        self.assertTrue(self.run_phase())
        self.assertEqual((self.result()['status'], self.result()['message']), ('waiting', 'leftover backend'))
        self.assertEqual(self.stored()['operation']['step'], 'creating')
        self.assertEqual(self.db.dropped, [])
        self.assertTrue(self.again())
        self.assertEqual(self.result()['status'], 'ok')

    def test_another_superuser_waits_without_spending_the_manifest(self):
        self.db.superuser = True
        self.assertTrue(self.run_phase())
        result = self.result()
        self.assertEqual((result['status'], result['step']), ('waiting', 'superuser'))
        self.assertEqual(result['message'], 'a role other than postgres is a superuser or a member of one')
        self.assertFalse(self.db.rows)
        self.phase.download.assert_not_called()
        self.assertEqual(self.state.decision('corpus', result['manifest_hash'], self.m['sequence']), 'pending')
        self.db.superuser = False
        self.assertTrue(self.run_phase())
        self.assertEqual(self.result()['status'], 'ok')

    def test_a_target_failure_aborts_spends_the_manifest_and_drops_the_target(self):
        self.db.failures['validate'] = ValueError('restored release row differs from manifest')
        self.assertFalse(self.run_phase())
        self.assertEqual((self.result()['status'], self.result()['message']),
                         ('failed', 'restored release row differs from manifest'))
        self.assertEqual(self.status(), 'failed')
        self.assertEqual(self.db.dropped, [self.name])
        self.assertEqual(self.db.rows[-1]['step'], 'aborted')
        self.assertEqual(self.db.lists_removed, [self.db.rows[0]['operation_id']])
        self.assertIsNone(self.stored()['operation'])

    def test_a_failed_create_keeps_a_target_activator_does_not_own(self):
        original = self.db.create
        def create(row):
            self.db.add(row['database_name'], owner='postgres')
            original(row)
        self.db.create = create
        self.assertFalse(self.run_phase())
        self.assertEqual(self.status(), 'failed')
        self.assertEqual(self.db.dropped, [])
        self.assertIn(self.name, self.db.databases)

    def test_new_activation_current_noop_and_daily_prune(self):
        self.assertTrue(self.run_phase())
        self.assertEqual(self.db.lists_discarded, 1)
        self.assertEqual(self.result()['status'], 'ok')
        self.assertEqual(self.result()['rel'], self.m['rel'])
        self.assertEqual(self.result()['mode'], 'new')
        self.assertEqual(self.status(), 'active')
        self.assertEqual(self.db.tidied, 1)
        self.phase.download.reset_mock()
        self.assertTrue(self.again())
        self.phase.download.assert_not_called()
        self.assertEqual(self.result()['status'], 'current')
        self.assertEqual(self.result()['mode'], 'new')

    def test_waiting_age_survives_transient_failure_and_process_restart(self):
        self.db.describe.return_value['dense_query_hash'] = 'f'*64
        self.assertTrue(self.run_phase())
        first = self.result()['waiting_since']
        self.phase.result.write('failed', 'fetch', 'transient failure', self.phase.hash, self.m, 'new')
        self.phase = self.make_phase()
        self.phase.result = ReleaseResult(self.paths, lambda: NOW + 100000)
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
        self.db.crashes.add('append:restored')
        with self.assertRaises(Crash):
            self.run_phase()
        self.registry.fetch.side_effect = CommandError('registry unavailable')
        self.registry.fetch.reset_mock()
        self.assertTrue(self.again())
        self.registry.fetch.assert_not_called()
        self.assertEqual(self.db.pointer_value, self.name)

    def test_committed_transaction_with_lost_response_finishes_on_the_next_run(self):
        original = self.db.swap
        def lost_response(statement, row):
            original(statement, row)
            raise CommandError('lost activation response')
        self.db.swap = lost_response
        self.assertFalse(self.run_phase())
        self.assertEqual(self.stored()['operation']['step'], 'swapping')
        self.assertEqual(self.status(), 'pending')
        del self.db.swap
        self.assertTrue(self.again())
        self.assertEqual(self.result()['status'], 'ok')
        self.assertEqual(self.stored()['active']['database'], self.name)

    def test_resume_rechecks_target_with_current_server_after_completed_smoke(self):
        self.db.crashes.add('append:checked')
        with self.assertRaises(Crash):
            self.run_phase()
        self.db.log.clear()
        self.assertTrue(self.again())
        self.assertEqual(self.db.log, ['validate', 'smoke'])

    def test_sequence_reuse_and_older_sequence_refused(self):
        self.state.accept('corpus', 'f'*64, self.m['sequence'])
        self.assertFalse(self.run_phase())
        self.assertIn('sequence reuse', self.result()['message'])
        self.m['sequence'] -= 1
        self.write_manifest()
        self.assertFalse(self.run_phase())
        self.assertIn('older sequence', self.result()['message'])

    def test_acceptance_supersedes_lower_pending_corpus_without_changing_image(self):
        self.state.accept('corpus', 'f'*64, 2)
        active_image = self.state.data['active']
        self.assertTrue(self.run_phase())
        self.assertEqual(self.state.data['accepted']['corpus']['2']['status'], 'failed')
        self.assertEqual(self.state.data['active'], active_image)

    def test_phase_order_and_corpus_failure_preserves_image_result(self):
        paths = self.paths
        paths.policy.parent.mkdir(parents=True, exist_ok=True)
        paths.policy.write_bytes(canonical(self.rules))
        paths.mode.write_text('activate')
        paths.lock.parent.mkdir(parents=True, exist_ok=True)
        paths.backup_lock.parent.mkdir(parents=True, exist_ok=True)
        paths.backup_lock.touch()
        app = Updater(paths, self.runner, user_prefix=[])
        self.assertEqual(app.phases, ('secrets','images','corpus','frameworks'))
        calls = []
        for name in app.phases:
            setattr(app, name, lambda *args, phase=name: calls.append(phase) or (phase != 'corpus'))
        self.assertEqual(app.run(), 1)
        self.assertEqual(calls, ['secrets','images','corpus','frameworks'])

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


if __name__ == '__main__':
    unittest.main()
