import importlib
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

from frameworks_test_support import PREVIOUS, calls, digest, install_fakes, manifest_object, object_bytes, publish_tag, state


class FrameworksPreviousTest(unittest.TestCase):
    def setUp(self):
        self.module = importlib.import_module('frameworks_previous')
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name)
        install_fakes(self.path / 'bin')
        self.registry = self.path / 'registry'
        self.log = self.path / 'calls.jsonl'
        self.earlier = state(PREVIOUS)
        self.earlier_report = object_bytes(self.earlier)['report.json']
        self.sha = digest(self.earlier_report)
        self.named = dict(rel=PREVIOUS, report_sha256=self.sha)
        patcher = patch.dict(os.environ, dict(PATH=str(self.path / 'bin') + os.pathsep + os.environ['PATH'],
                                              FAKE_LOG=str(self.log), FAKE_REGISTRY=str(self.registry)))
        patcher.start()
        self.addCleanup(patcher.stop)
        self.requests = []
        self.stored = self.earlier_report

    def client(self, args):
        self.requests.append(args)
        size = len(self.stored)
        if args[0] == 'head-object':
            return dict(ContentLength=size, ETag='"opaque"')
        Path(args[-1]).write_bytes(self.stored)
        return dict(ETag='"opaque"', ContentRange=f'bytes 0-{size - 1}/{size}')

    def publish(self, tag='frameworks-production', **changes):
        publish_tag(self.registry, tag, manifest_object(self.earlier) | changes)

    def active(self, previous):
        return self.module.require_active(dict(previous=previous), self.path / 'work', self.client, 'dannyota/trachuan')

    def test_first_release_needs_null_previous_and_no_fetch(self):
        self.active(None)
        self.assertEqual(self.requests, [])
        with self.assertRaises(ValueError):
            self.active(self.named)

    def test_published_release_requires_the_active_reference(self):
        self.publish()
        self.active(self.named)
        keys = [args[args.index('--key') + 1] for args in self.requests]
        self.assertEqual(keys, [f'frameworks/{PREVIOUS}/report.json'] * 2)
        self.assertIn('trachuan-frameworks-releases', self.requests[0])
        self.assertIn(f'bytes=0-{len(self.stored) - 1}', self.requests[1])
        self.assertIn('--if-match', self.requests[1])
        self.assertTrue(Path(self.requests[1][-1]).is_relative_to(self.path / 'work'))

    def test_active_check_refuses_each_mismatch(self):
        self.publish()
        for previous in (None, dict(self.named, rel='20990100'), dict(self.named, report_sha256='0' * 64)):
            with self.subTest(previous=previous), self.assertRaises(ValueError):
                self.active(previous)
        self.stored = b'replaced report'
        with self.assertRaises(ValueError):
            self.active(self.named)

    def test_active_check_requires_the_signed_hash_too(self):
        self.publish(report=dict(sha256='0' * 64, quality_pass=True))
        with self.assertRaises(ValueError):
            self.active(self.named)

    def test_active_check_refuses_oversize_and_wrong_ranges(self):
        self.publish()
        with self.assertRaises(ValueError):
            self.module.require_active(dict(previous=self.named), self.path / 'work',
                                       lambda args: dict(ContentLength=16 * 1024**2 + 1, ETag='"opaque"'), 'dannyota/trachuan')
        size = len(self.stored)

        def bad_range(args):
            if args[0] == 'head-object':
                return dict(ContentLength=size, ETag='"opaque"')
            Path(args[-1]).write_bytes(self.stored)
            return dict(ETag='"opaque"', ContentRange=f'bytes 0-{size - 1}/{size + 1}')

        with self.assertRaises(ValueError):
            self.module.require_active(dict(previous=self.named), self.path / 'work', bad_range, 'dannyota/trachuan')

    def test_bad_signature_stops_the_active_check_before_any_fetch(self):
        self.publish()
        with patch.dict(os.environ, FAKE_BAD_SIGNATURE='1'), self.assertRaises(subprocess.CalledProcessError):
            self.active(self.named)
        self.assertEqual(self.requests, [])

    def test_signed_check_allows_null_and_a_retry_or_rollback_target(self):
        self.module.require_signed(dict(previous=None), self.path / 'previous', 'dannyota/trachuan')
        self.assertEqual(calls(self.log), [])
        self.publish('frameworks-' + PREVIOUS)
        self.publish('frameworks-production', rel='20990103', database='frameworks_20990103',
                     dump=dict(object='frameworks/20990103/frameworks.dump', sha256='1' * 64, size=1))
        self.module.require_signed(dict(previous=self.named), self.path / 'previous', 'dannyota/trachuan')
        resolved = [call['args'][1] for call in calls(self.log) if call['args'][0] == 'resolve']
        self.assertEqual(resolved, ['ghcr.io/dannyota/trachuan-corpus-release:frameworks-' + PREVIOUS])
        self.assertTrue(any(call['name'] == 'cosign' for call in calls(self.log)))

    def test_signed_check_refuses_missing_unsigned_or_different_reports(self):
        with self.assertRaises(ValueError):
            self.module.require_signed(dict(previous=self.named), self.path / 'previous', 'dannyota/trachuan')
        self.publish('frameworks-' + PREVIOUS)
        with self.assertRaises(ValueError):
            self.module.require_signed(dict(previous=dict(self.named, report_sha256='0' * 64)), self.path / 'previous', 'dannyota/trachuan')
        with patch.dict(os.environ, FAKE_BAD_SIGNATURE='1'), self.assertRaises(subprocess.CalledProcessError):
            self.module.require_signed(dict(previous=self.named), self.path / 'previous', 'dannyota/trachuan')
        self.assertEqual(json.loads(json.dumps(self.named))['rel'], PREVIOUS)


if __name__ == '__main__':
    unittest.main()
