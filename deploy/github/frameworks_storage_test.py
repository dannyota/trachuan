import contextlib
import importlib
import io
import json
import os
from pathlib import Path
import tempfile
import unittest
from types import SimpleNamespace
from unittest.mock import patch

from frameworks_test_support import OBJECTS, REL, SENTINEL, calls, digest, install_fakes, object_bytes, state

BUCKET = 'trachuan-frameworks-releases'


class FrameworksStorageTest(unittest.TestCase):
    def setUp(self):
        self.module = importlib.import_module('frameworks_storage')
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name)
        install_fakes(self.path / 'bin')
        self.log = self.path / 'calls.jsonl'
        self.remote = self.path / 'remote'
        self.stored = self.remote / BUCKET / 'frameworks' / REL
        self.stored.mkdir(parents=True)
        self.objects = object_bytes(state())
        for name, data in self.objects.items():
            (self.stored / name).write_bytes(data)
        self.env = dict(PATH=str(self.path / 'bin') + os.pathsep + os.environ['PATH'], FAKE_LOG=str(self.log),
                        FAKE_REMOTE=str(self.remote), OCI_S3_ENDPOINT='https://synthetic.example',
                        PYTHONDONTWRITEBYTECODE='1')

    def download(self, target='objects', **extra):
        with patch.dict(os.environ, self.env | extra, clear=True):
            self.module.download(REL, self.path / target)

    def test_downloads_the_five_objects_with_head_range_and_etag_checks(self):
        output = io.StringIO()
        with contextlib.redirect_stdout(output):
            self.download()
        target = self.path / 'objects'
        self.assertEqual(target.stat().st_mode & 0o777, 0o700)
        self.assertEqual({path.name for path in target.iterdir()}, set(OBJECTS))
        for name, data in self.objects.items():
            self.assertEqual((target / name).read_bytes(), data)
        log = calls(self.log)
        s3 = [call for call in log if '--version' not in call['args']]
        heads = [i for i, call in enumerate(s3) if 'head-object' in call['args']]
        gets = [i for i, call in enumerate(s3) if 'get-object' in call['args']]
        self.assertEqual((len(heads), len(gets)), (5, 5))
        self.assertLess(max(heads), min(gets))
        for call in s3:
            args = call['args']
            self.assertEqual(args[args.index('--bucket') + 1], BUCKET)
            self.assertEqual(call['checksum'], 'when_required')
            self.assertIn('addressing_style = path', call['config'])
            self.assertNotIn('--profile', args)
            if 'get-object' in args:
                name = args[args.index('--key') + 1].rsplit('/', 1)[1]
                self.assertEqual(args[args.index('--key') + 1], f'frameworks/{REL}/{name}')
                self.assertEqual(args[args.index('--range') + 1], f'bytes=0-{len(self.objects[name]) - 1}')
                self.assertEqual(args[args.index('--if-match') + 1], '"' + digest(self.objects[name]) + '"')
        self.assertIn('aws-cli/2.40.0', output.getvalue())
        self.assertNotIn(SENTINEL, output.getvalue())

    def test_refuses_limits_before_any_download(self):
        limits = {'frameworks.dump': 16 * 1024**3, 'build.json': 16 * 1024**2, 'evaluate.json': 16 * 1024**2,
                  'report.json': 16 * 1024**2, 'release.json': 16 * 1024**2}
        for name, limit in limits.items():
            self.log.unlink(missing_ok=True)
            with self.subTest(name=name), self.assertRaises(ValueError):
                self.download('limit-' + name, FAKE_HEAD_SIZES=json.dumps({name: limit + 1}))
            self.assertFalse(any('get-object' in call['args'] for call in calls(self.log)))

    def test_refuses_when_free_disk_would_fall_below_the_reserve(self):
        total = sum(map(len, self.objects.values()))
        with patch('shutil.disk_usage', return_value=SimpleNamespace(free=2 * 1024**3 + total)), \
                self.assertRaises(ValueError):
            self.download('space')
        self.assertFalse(any('get-object' in call['args'] for call in calls(self.log)))

    def test_refuses_changed_objects_and_missing_metadata(self):
        name = 'report.json'
        size = len(self.objects[name])
        faults = [dict(FAKE_BAD_SIZE=name), dict(FAKE_GROW_AFTER_HEAD=name),
                  dict(FAKE_HEAD_ETAGS=json.dumps({name: None})),
                  dict(FAKE_GET_METADATA=json.dumps({name: {'ETag': '"other"'}})),
                  dict(FAKE_GET_METADATA=json.dumps({name: {'ContentRange': f'bytes 0-{size - 1}/*'}}))]
        for index, fault in enumerate(faults):
            with self.subTest(fault=fault), self.assertRaises(ValueError):
                self.download(f'fault-{index}', **fault)

    def test_missing_object_and_wrong_tool_or_identity_stop(self):
        (self.stored / 'evaluate.json').unlink()
        with self.assertRaises(Exception):
            self.download('missing')
        self.assertFalse(any('get-object' in call['args'] for call in calls(self.log)))
        for version in ('2.39.0', '2.40.01', '3.0.0'):
            with self.subTest(version=version), self.assertRaises(ValueError):
                self.download('tool', FAKE_AWS_VERSION=version)
            self.assertFalse((self.path / 'tool').exists())
        with patch.dict(os.environ, self.env, clear=True), self.assertRaises(ValueError):
            self.module.download('../x', self.path / 'unused')
        with self.assertRaises(ValueError):
            self.download('endpoint', OCI_S3_ENDPOINT='http://synthetic.example')


if __name__ == '__main__':
    unittest.main()
