import json
from pathlib import Path
import unittest
from unittest import mock

from updater import activation_test as fixture
from updater.deploy_corpus import Corpus
from updater.deploy_corpus_kind import FRAMEWORKS
from updater.deploy_io import canonical, CommandError
from updater.frameworks_activation_test import Database
from updater.frameworks_verify_test import BLOCK, description, manifest


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

    def test_new_activation_refuses_at_restore_and_discards_staging(self):
        law_before = json.dumps(self.state.data['accepted']['corpus'])
        self.assertFalse(self.run_phase())
        result = self.result()
        self.assertEqual((result['status'], result['step'], result['message'], result['mode']),
                         ('failed', 'activation', 'frameworks restore not implemented', 'new'))
        self.assertEqual(result['rel'], self.m['rel'])
        self.assertEqual(self.downloads, [self.staging])
        self.assertFalse(self.staging.exists())
        self.assertEqual(self.state.failed('frameworks'), [result['manifest_hash']])
        self.assertEqual(self.state.data['failed'], [])
        self.assertEqual(json.dumps(self.state.data['accepted']['corpus']), law_before)
        self.assertEqual(self.db.created, 0)
        self.assertEqual([r['step'] for r in self.db.rows], ['started', 'aborted'])
        self.assertTrue(self.run_phase() is False)
        self.assertEqual(self.result()['message'], 'frameworks manifest failed; re-sign the release to retry')
        self.prepare.assert_called_once()

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

    def test_leftover_staging_from_an_interrupted_run_is_removed(self):
        leftover = self.paths.root / 'srv/trachuan/data/releases/frameworks-20990101_1'
        leftover.mkdir(parents=True)
        (leftover / 'frameworks.dump').write_bytes(b'x')
        law = self.paths.root / 'srv/trachuan/data/releases/vn-20990101_1'
        law.mkdir()
        self.db.describe.return_value = dict(description(self.m), query_recipe_hash='f' * 64)
        self.assertTrue(self.run_phase())
        self.assertFalse(leftover.exists())
        self.assertTrue(law.exists())

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
