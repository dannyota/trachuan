from repositories_test_support import enable_dual_policy as setUpModule

import importlib
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

from frameworks_test_support import PREVIOUS, REL, calls, install_fakes, manifest_object, publish_tag, state

ROOT = Path(__file__).resolve().parents[2]
REPOSITORY = 'ghcr.io/dannyota/trachuan-corpus-release'


class FrameworksRegistryTest(unittest.TestCase):
    def setUp(self):
        self.module = importlib.import_module('frameworks_registry')
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name)
        install_fakes(self.path / 'bin')
        self.registry = self.path / 'registry'
        self.registry.mkdir()
        self.log = self.path / 'calls.jsonl'
        self.value = state(PREVIOUS)
        self.env = dict(PATH=str(self.path / 'bin') + os.pathsep + os.environ['PATH'], FAKE_LOG=str(self.log),
                        FAKE_REGISTRY=str(self.registry))
        patcher = patch.dict(os.environ, self.env)
        patcher.start()
        self.addCleanup(patcher.stop)

    def publish(self, tag='frameworks-production', **changes):
        publish_tag(self.registry, tag, manifest_object(self.value) | changes)

    def test_absent_tag_is_none_only_for_the_exact_registry_error(self):
        self.assertIsNone(self.module.production('dannyota/trachuan', self.path / 'out'))
        self.assertEqual(calls(self.log)[0]['args'], ['resolve', REPOSITORY + ':frameworks-production'])

    def test_host_fixture_text_is_the_recognised_error(self):
        text = (ROOT / 'deploy/host/updater/fixtures/oras-resolve-missing-tag.txt').read_text()
        self.assertTrue(self.module.missing_tag(text, REPOSITORY + ':frameworks-production'))

    def test_other_resolve_failures_are_not_absence(self):
        reference = REPOSITORY + ':frameworks-production'
        for text in ('Error response from registry: unauthorized',
                     'Error response from registry: failed to resolve digest: ghcr.io/other/repo:frameworks-production: not found',
                     f'Error response from registry: failed to resolve digest: {reference}: not found: retry',
                     'dial tcp: i/o timeout', ''):
            with self.subTest(text=text), patch.dict(os.environ, FAKE_RESOLVE_ERROR=text or ' '):
                with self.assertRaises(ValueError):
                    self.module.production('dannyota/trachuan', self.path / 'out')

    def test_production_is_verified_against_the_frameworks_workflow_and_commit(self):
        self.publish(sequence=9)
        result = self.module.production('dannyota/trachuan', self.path / 'out')
        self.assertEqual(result['sequence'], 9)
        command = next(call['args'] for call in calls(self.log) if call['name'] == 'cosign')
        self.assertEqual(command[command.index('--certificate-identity') + 1],
                         'https://github.com/dannyota/trachuan/.github/workflows/frameworks-release.yml@refs/heads/main')
        self.assertEqual(command[command.index('--certificate-github-workflow-sha') + 1], 'a' * 40)

    def test_release_reads_its_own_tag_and_requires_it(self):
        self.publish('frameworks-' + PREVIOUS)
        self.assertEqual(self.module.release('dannyota/trachuan', PREVIOUS, self.path / 'out')['rel'], PREVIOUS)
        with self.assertRaises(ValueError):
            self.module.release('dannyota/trachuan', REL, self.path / 'out')
        with self.assertRaises(ValueError):
            self.module.release('dannyota/trachuan', '../bad', self.path / 'out')
        self.publish('frameworks-' + REL)
        with self.assertRaises(ValueError):
            self.module.release('dannyota/trachuan', REL, self.path / 'out')

    def test_bad_signature_artifact_type_and_manifest_are_refused(self):
        self.publish()
        with patch.dict(os.environ, FAKE_BAD_SIGNATURE='1'), self.assertRaises(subprocess.CalledProcessError):
            self.module.production('dannyota/trachuan', self.path / 'out')
        with patch.dict(os.environ, FAKE_ARTIFACT_TYPE='application/other'), self.assertRaises(ValueError):
            self.module.production('dannyota/trachuan', self.path / 'out')
        self.publish(corpus_kind='law')
        with self.assertRaises(ValueError):
            self.module.production('dannyota/trachuan', self.path / 'out')


if __name__ == '__main__':
    unittest.main()
