from repositories_test_support import enable_dual_policy as setUpModule

import contextlib
import importlib
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
from unittest.mock import patch

from frameworks_test_support import (COMMIT, PREVIOUS, REL, SENTINEL, bind, calls, canonical, digest, install_fakes,
                                     manifest_object, object_bytes, publish_tag, report_hash, state, write_objects)

MAIN = 'refs/heads/main'
REPOSITORY = 'ghcr.io/dannyota/trachuan-corpus-release'


class FrameworksPublishTest(unittest.TestCase):
    def setUp(self):
        self.module = importlib.import_module('frameworks_publish')
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name)
        install_fakes(self.path / 'bin')
        self.registry = self.path / 'registry'
        self.registry.mkdir()
        self.log = self.path / 'calls.jsonl'
        self.directory = self.path / 'release'
        self.directory.mkdir()
        self.value = state()
        write_objects(self.directory / 'objects', self.value)
        patcher = patch.dict(os.environ, dict(PATH=str(self.path / 'bin') + os.pathsep + os.environ['PATH'],
                                              FAKE_LOG=str(self.log), FAKE_REGISTRY=str(self.registry)))
        patcher.start()
        self.addCleanup(patcher.stop)

    def run_release(self, sequence=5, ref=MAIN, commit=COMMIT, report_sha256=None):
        output = io.StringIO()
        with contextlib.redirect_stdout(output):
            self.module.release(REL, report_sha256 or report_hash(self.value), self.directory, ref, commit, sequence, 'dannyota/trachuan')
        return output.getvalue()

    def command(self):
        return ['python3', str(Path(__file__).with_name('frameworks_publish.py')), REL, report_hash(self.value), str(self.directory)]

    def pushed(self, tag):
        return json.loads((self.registry / tag / 'manifest.json').read_bytes())

    def test_signs_pushes_both_tags_and_keeps_signing_directory_clean(self):
        self.run_release()
        self.assertEqual(self.pushed('frameworks-' + REL), manifest_object(self.value, sequence=5))
        self.assertEqual(self.pushed('frameworks-production')['sequence'], 5)
        self.assertEqual({path.name for path in (self.directory / 'sign').iterdir()},
                         {'manifest.json', 'manifest.sigstore.json'})
        log = calls(self.log)
        sign = next(call for call in log if call['args'][0] == 'sign-blob')
        self.assertEqual(Path(sign['args'][-1]), self.directory / 'sign/manifest.json')
        verify = next(call['args'] for call in log if call['args'][0] == 'verify-blob')
        self.assertIn('https://github.com/dannyota/trachuan/.github/workflows/frameworks-release.yml@refs/heads/main', verify)
        push = next(call for call in log if call['args'][0] == 'push')
        self.assertEqual(push['args'][1], f'{REPOSITORY}:frameworks-{REL}')
        self.assertEqual(Path(push['cwd']).resolve(), (self.directory / 'sign').resolve())
        self.assertIn('application/vnd.trachuan.corpus-release.v1', push['args'])
        self.assertEqual(log[-1]['args'], ['tag', f'{REPOSITORY}:frameworks-{REL}', 'frameworks-production'])

    def test_production_moves_only_above_the_verified_sequence(self):
        for current, moved in ((4, True), (5, False), (6, False)):
            with self.subTest(current=current):
                publish_tag(self.registry, 'frameworks-production', manifest_object(state(PREVIOUS), sequence=current))
                self.log.unlink(missing_ok=True)
                shutil.rmtree(self.directory / 'sign', ignore_errors=True)
                shutil.rmtree(self.directory / 'production', ignore_errors=True)
                self.run_release()
                self.assertEqual(any(call['args'][0] == 'tag' for call in calls(self.log)), moved)
                self.assertEqual(self.pushed('frameworks-production')['sequence'], 5 if moved else current)
                self.assertEqual(self.pushed('frameworks-' + REL)['sequence'], 5)

    def test_unverifiable_production_stops_before_the_tag_moves(self):
        publish_tag(self.registry, 'frameworks-production', manifest_object(state(PREVIOUS), sequence=1) | {'corpus_kind': 'law'})
        with self.assertRaises(ValueError):
            self.run_release()
        self.assertFalse(any(call['args'][0] == 'tag' for call in calls(self.log)))

    def test_previous_is_checked_before_signing(self):
        earlier = state(PREVIOUS)
        named = dict(rel=PREVIOUS, report_sha256=digest(object_bytes(earlier)['report.json']))
        self.value['report']['previous'] = named
        write_objects(self.directory / 'objects', self.value)
        with self.assertRaises(ValueError):
            self.run_release()
        self.assertFalse(any(call['name'] == 'cosign' and call['args'][0] == 'sign-blob' for call in calls(self.log)))
        shutil.rmtree(self.directory / 'previous', ignore_errors=True)
        publish_tag(self.registry, 'frameworks-' + PREVIOUS, manifest_object(earlier))
        self.run_release()
        self.assertEqual(self.pushed('frameworks-' + REL)['previous'], PREVIOUS)

    def test_a_retry_of_a_signed_release_needs_no_active_previous(self):
        publish_tag(self.registry, 'frameworks-production', manifest_object(self.value, sequence=9))
        self.run_release(sequence=10)
        self.assertEqual(self.pushed('frameworks-' + REL)['sequence'], 10)
        self.assertEqual(self.pushed('frameworks-production')['sequence'], 10)

    def test_invalid_release_is_never_signed(self):
        self.value['report']['quality_pass'] = False
        write_objects(self.directory / 'objects', self.value)
        with self.assertRaises(ValueError):
            self.run_release()
        self.assertEqual(calls(self.log), [])
        self.assertFalse((self.directory / 'sign').exists())

    def test_report_must_have_the_dispatched_hash(self):
        for report_sha256 in ('0' * 64, 'short'):
            with self.subTest(report_sha256=report_sha256), self.assertRaises(ValueError):
                self.run_release(report_sha256=report_sha256)
        self.assertEqual(calls(self.log), [])
        self.assertFalse((self.directory / 'sign').exists())

    def test_forged_consistent_set_is_refused_by_the_dispatched_hash(self):
        dispatched = report_hash(self.value)
        forged = state()
        forged['dump'] = b'forged dump'
        bind(forged)
        write_objects(self.directory / 'objects', forged)
        with self.assertRaises(ValueError):
            self.run_release(report_sha256=dispatched)
        self.assertEqual(calls(self.log), [])

    def test_wrong_ref_or_commit_prevents_signing(self):
        with self.assertRaises(ValueError):
            self.run_release(ref='refs/heads/other')
        with self.assertRaises(ValueError):
            self.run_release(commit='short')
        self.assertEqual(calls(self.log), [])

    def test_publish_refuses_a_manifest_from_another_commit(self):
        sign = self.directory / 'sign'
        sign.mkdir()
        (sign / 'manifest.json').write_bytes(canonical(manifest_object(self.value)))
        with self.assertRaises(ValueError):
            self.module.publish(sign, self.directory / 'production', MAIN, 'b' * 40, 'dannyota/trachuan')
        self.assertEqual(calls(self.log), [])

    def test_failed_signing_pushes_nothing(self):
        with patch.dict(os.environ, FAKE_BAD_SIGNATURE='1'), self.assertRaises(subprocess.CalledProcessError):
            self.run_release()
        self.assertFalse(any(call['args'][0] in ('push', 'tag') for call in calls(self.log)))

    def test_log_holds_no_signature_or_release_content(self):
        result = subprocess.run(self.command(),
                                env=os.environ | dict(GITHUB_REF=MAIN, GITHUB_SHA=COMMIT, GITHUB_RUN_NUMBER='5', GITHUB_REPOSITORY='dntmakers/trachuan',
                                                      PYTHONDONTWRITEBYTECODE='1'),
                                capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        text = result.stdout + result.stderr
        self.assertNotIn(SENTINEL, text)
        self.assertNotIn('SIGNATURE-ON-STDOUT', text)

    def test_cli_failure_prints_a_fixed_message_without_a_traceback(self):
        self.value['report']['quality_pass'] = SENTINEL
        write_objects(self.directory / 'objects', self.value)
        result = subprocess.run(self.command(),
                                env=os.environ | dict(GITHUB_REF=MAIN, GITHUB_SHA=COMMIT, GITHUB_RUN_NUMBER='5', GITHUB_REPOSITORY='dntmakers/trachuan',
                                                      PYTHONDONTWRITEBYTECODE='1'),
                                capture_output=True, text=True)
        self.assertEqual(result.returncode, 1)
        self.assertNotIn(SENTINEL, result.stdout + result.stderr)
        self.assertNotIn('Traceback', result.stderr)


if __name__ == '__main__':
    unittest.main()
