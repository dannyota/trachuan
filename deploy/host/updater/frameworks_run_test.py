import hashlib
import json
from pathlib import Path
import subprocess
import sys
import unittest
from unittest import mock

from updater import activation_test as fixture
from updater import deploy_io
from updater.deploy_corpus import Corpus
from updater.deploy_corpus_kind import FRAMEWORKS
from updater.deploy_io import canonical, CommandError
from updater.deploy_registry import Registry
from updater.frameworks_activation_test import Database
from updater.frameworks_verify_test import BLOCK, NESTED, description, manifest


MISSING_TAG = Path(__file__).with_name('fixtures') / 'oras-resolve-missing-tag.txt'


def missing_tag(args, **kwargs):
    real = subprocess.Popen
    def spawn(argv, **options):
        return real([sys.executable, '-I', '-c', 'import sys; sys.stderr.write(open(sys.argv[1]).read()); sys.exit(1)',
                     str(MISSING_TAG)], **options)
    with mock.patch.object(deploy_io.subprocess, 'Popen', side_effect=spawn):
        return deploy_io.command(args, **kwargs)


def interrupted_command():
    try:
        try:
            raise InterruptedError('updater interrupted')
        except InterruptedError as cause:
            raise CommandError('command failed') from cause
    except CommandError as error:
        return error


class FrameworksRunTest(fixture.ActivationFixture, unittest.TestCase):
    def setUp(self):
        super().setUp()
        self.rules = dict(self.rules, frameworks=dict(BLOCK))
        self.state.data['release_floor'] = BLOCK['min_release_floor']
        self.state.save()
        self.m = manifest()
        self.m['previous'] = None
        self.directory = self.paths.state / 'frameworks-fixture'
        self.directory.mkdir()
        self.write_manifest()
        self.db = Database()
        self.db.check_extensions = mock.Mock()
        self.db.describe = mock.Mock(return_value=description(self.m))
        self.registry = mock.Mock()
        self.registry.fetch.return_value = self.directory
        self.prepare = mock.Mock(return_value=True)
        self.phase = Corpus(self.state, self.store, self.host, self.rules, self.registry, database=self.db,
                            prepare=self.prepare, clock=lambda: 100000, kind=FRAMEWORKS)
        self.phase.resources = mock.Mock()
        self.phase.engine.steps = self.db
        self.staging = self.paths.root / 'srv/trachuan/data/releases' / ('frameworks-' + self.m['rel'])
        self.downloads = []
        def download():
            self.staging.mkdir(parents=True, exist_ok=True)
            (self.staging / 'frameworks.dump').write_bytes(b'synthetic dump')
            self.downloads.append(self.staging)
            return self.staging / 'frameworks.dump'
        self.phase.download = mock.Mock(side_effect=download)

    def write_manifest(self):
        self.directory.joinpath('manifest.json').write_bytes(canonical(self.m))
        self.directory.joinpath('manifest.sigstore.json').write_bytes(b'{}')

    def run_phase(self, mode='activate'):
        return self.phase.run(self.paths.state / 'run', mode)

    def result(self):
        return json.loads(self.paths.result.with_name('release-frameworks.json').read_bytes())

    def result_hash(self):
        return hashlib.sha256(self.directory.joinpath('manifest.json').read_bytes()).hexdigest()

    def test_waits_for_release_floor_before_any_host_or_registry_action(self):
        self.state.data['release_floor'] = BLOCK['min_release_floor'] - 1
        self.calls.clear()
        self.db.lock = mock.Mock(side_effect=AssertionError('no activator session'))
        self.assertTrue(self.run_phase())
        self.assertEqual((self.result()['status'], self.result()['message']), ('waiting', 'release floor'))
        self.assertIsNotNone(self.result()['waiting_since'])
        self.registry.fetch.assert_not_called()
        self.assertEqual(self.calls, [])
        self.assertNotIn('frameworks', self.state.data)
        del self.state.data['release_floor']
        self.assertTrue(self.run_phase())
        self.assertEqual(self.result()['message'], 'release floor')

    def test_unpublished_frameworks_package_is_current_until_a_manifest_was_accepted(self):
        for reason in ('denied', 'not_found'):
            self.registry.fetch.side_effect = CommandError('package unreadable', reason=reason)
            self.assertTrue(self.run_phase())
            self.assertEqual((self.result()['status'], self.result()['message']),
                             ('current', 'no frameworks release published'))
        args = self.registry.fetch.call_args.args
        self.assertEqual(args[:2], (BLOCK['repository'], 'frameworks-production'))
        self.assertEqual(args[2], self.paths.state / 'run' / 'frameworks')
        self.state.data['accepted']['corpus']['1'] = {'hash': 'a' * 64, 'status': 'active'}
        self.assertTrue(self.run_phase())
        self.state.accept('frameworks', 'a' * 64, 1)
        self.assertFalse(self.run_phase())
        self.assertEqual(self.result()['status'], 'failed')
        self.assertFalse(self.paths.result.with_name('release.json').exists())

    def test_new_bootstrap_downloads_restores_and_activates(self):
        law_before = json.dumps(self.state.data['accepted']['corpus'])
        self.assertTrue(self.run_phase())
        result = self.result()
        self.assertEqual((result['status'], result['step'], result['message'], result['mode']),
                         ('ok', 'complete', 'frameworks release activated', 'new'))
        self.assertEqual(self.downloads, [self.staging])
        self.assertFalse((self.staging / 'frameworks.dump').exists())
        self.assertEqual(self.db.steps, ['inspect', 'inspect', 'create', 'restore', 'validate', 'smoke'])
        self.assertEqual([r['step'] for r in self.db.rows][-1], 'activated')
        self.assertEqual(self.db.active, self.m['database'])
        self.assertEqual(self.state.decision('frameworks', result['manifest_hash'], self.m['sequence']), 'noop')
        self.assertEqual(json.dumps(self.state.data['accepted']['corpus']), law_before)

    def test_recovery_of_an_open_new_journal_downloads_and_resumes(self):
        digest = self.result_hash()
        self.phase.hash = digest
        self.phase.remember(self.directory)
        self.state.accept('frameworks', digest, self.m['sequence'])
        row = self.phase.engine.begin(digest, self.m, 'new', None)
        self.db.append(row, 'restoring')
        self.registry.fetch.side_effect = AssertionError('recovery must not fetch')
        self.assertTrue(self.run_phase())
        self.assertEqual((self.result()['status'], self.result()['message']), ('ok', 'frameworks release activated'))
        self.assertEqual(self.downloads, [self.staging])
        self.assertEqual(self.db.steps, ['restore', 'validate', 'smoke'])
        self.assertEqual(self.db.active, self.m['database'])

    def test_smoke_failure_reports_program_exit_and_safe_cause_only(self):
        self.db.smoke = mock.Mock(side_effect=CommandError(
            'command failed', detail='podman run: exit 1', safe='frameworks release-check search: SQLSTATE 42P01'))
        self.assertFalse(self.run_phase())
        self.assertEqual((self.result()['status'], self.result()['step'], self.result()['message']),
                         ('failed', 'activation', 'frameworks command failed or run interrupted: podman run: exit 1: '
                          'frameworks release-check search: SQLSTATE 42P01'))
        self.assertEqual([r['step'] for r in self.db.rows][-1], 'aborted')
        self.assertEqual(self.db.dropped, [self.m['database']])
        self.assertIsNone(self.db.active)
        self.assertIn(self.result()['manifest_hash'], self.state.failed('frameworks'))

    def test_restore_and_step_failures_never_log_command_output(self):
        marker = 'invented licensed marker'
        for error, message in (
                (CommandError('command failed', detail='pg_restore: exit 1: pg_restore: error: ' + marker),
                 'frameworks command failed or run interrupted: pg_restore: exit 1'),
                (CommandError('command failed', detail='psql: exit 3: ' + marker),
                 'frameworks command failed or run interrupted: psql: exit 3'),
                (ValueError('frameworks log settings refused'), 'frameworks log settings refused')):
            with self.subTest(message=message):
                self.setUp()
                self.db.restore = mock.Mock(side_effect=error)
                self.assertFalse(self.run_phase())
                self.assertEqual(self.result()['message'], message)
                log = self.paths.log.with_name('release-frameworks.log').read_text()
                self.assertNotIn(marker, log)
                self.assertNotIn(marker, json.dumps(self.db.rows))

    def test_compatibility_difference_waits_without_download(self):
        for change in (dict(query_recipe_hash='f' * 64), dict(schema_versions=[2]),
                       dict(retrieval_recipe_versions=[9]), dict(lexical_recipe_versions=[9])):
            with self.subTest(change=change):
                self.db.describe.return_value = dict(description(self.m), **change)
                self.assertTrue(self.run_phase())
                self.assertEqual((self.result()['status'], self.result()['step']), ('waiting', 'compatibility'))
        self.phase.download.assert_not_called()
        self.assertEqual(self.state.decision('frameworks', self.result()['manifest_hash'], self.m['sequence']), 'pending')

    def test_verify_mode_binds_sequence_without_download_or_journal(self):
        self.assertTrue(self.run_phase('verify'))
        self.assertEqual(self.result()['status'], 'verified')
        self.assertFalse(self.db.rows)
        self.phase.download.assert_not_called()
        self.prepare.assert_not_called()

    def test_bootstrap_refused_once_a_pointer_exists(self):
        self.db.active = 'frameworks_20990101'
        self.assertFalse(self.run_phase())
        self.assertEqual(self.result()['message'], 'new frameworks previous does not equal active release')
        self.phase.download.assert_not_called()

    def test_command_failure_message_keeps_only_program_and_exit(self):
        self.registry.fetch.side_effect = CommandError(
            'command failed', detail='oras resolve: exit 1: Error: synthetic manifest text')
        self.assertFalse(self.run_phase())
        self.assertEqual(self.result()['message'], 'frameworks command failed or run interrupted: oras resolve: exit 1')
        lines = self.paths.log.with_name('release-frameworks.log').read_text().splitlines()
        self.assertFalse(any('synthetic manifest text' in line for line in lines))

    def test_missing_production_tag_is_unpublished_until_a_manifest_was_accepted(self):
        self.phase.registry = Registry(self.rules, missing_tag)
        self.assertTrue(self.run_phase())
        self.assertEqual((self.result()['status'], self.result()['message']), ('current', 'no frameworks release published'))
        self.state.accept('frameworks', 'a' * 64, 1)
        self.assertFalse(self.run_phase())
        self.assertEqual((self.result()['status'], self.result()['message']),
                         ('failed', 'frameworks command failed or run interrupted: oras resolve: exit 1'))

    def test_interruption_is_flagged_directly_or_through_a_command(self):
        for error, expected in ((InterruptedError('updater interrupted'), True), (interrupted_command(), True),
                                (CommandError('command failed'), False), (ValueError('refused'), False)):
            with self.subTest(error=error):
                self.registry.fetch.side_effect = error
                phase = Corpus(self.state, self.store, self.host, self.rules, self.registry, database=self.db,
                               prepare=self.prepare, clock=lambda: 100000, kind=FRAMEWORKS)
                self.assertFalse(phase.run(self.paths.state / 'run', 'activate'))
                self.assertEqual(phase.interrupted, expected)
                self.assertEqual(self.result()['status'], 'failed')

    def test_nested_manifest_and_other_recursion_end_as_failed_results(self):
        self.directory.joinpath('manifest.json').write_bytes(NESTED)
        self.assertFalse(self.run_phase())
        self.assertEqual((self.result()['status'], self.result()['step'], self.result()['message']),
                         ('failed', 'manifest', 'invalid frameworks manifest JSON'))
        self.write_manifest()
        self.db.describe.side_effect = RecursionError('synthetic recursion')
        self.assertFalse(self.run_phase())
        self.assertEqual((self.result()['status'], self.result()['step']), ('failed', 'compatibility'))

    def test_download_uses_block_bucket_into_private_staging(self):
        calls = []
        def runner(args, **kwargs):
            calls.append((args, kwargs))
            Path(args[-1]).write_bytes(b'synthetic dump')
            return b''
        self.host.runner = runner
        self.phase.manifest = self.m
        target = Corpus.download(self.phase)
        self.assertEqual(target, self.staging / 'frameworks.dump')
        self.assertEqual(self.staging.stat().st_mode & 0o777, 0o700)
        args, kwargs = calls[0]
        self.assertEqual(args[-4:], [self.rules['region'], BLOCK['namespace'], BLOCK['bucket'], str(target)])
        self.assertEqual(json.loads(kwargs['input']), self.m['dump'])


if __name__ == '__main__':
    unittest.main()
