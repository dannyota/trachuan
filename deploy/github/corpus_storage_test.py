import importlib
import json
import os
from pathlib import Path
import tempfile
import unittest
import io
from contextlib import redirect_stdout
from types import SimpleNamespace
from unittest.mock import patch

from corpus_upload_test import FAKE
from corpus_test_support import write_fixture


class CorpusStorageTest(unittest.TestCase):
    def setUp(self):
        self.storage = importlib.import_module('corpus_storage')
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name)
        self.remote = self.path / 'remote'
        self.remote.mkdir()
        write_fixture(self.remote)
        executable = self.path / 'aws'
        executable.write_text(FAKE)
        executable.chmod(0o755)
        self.log = self.path / 'calls.jsonl'
        self.env = dict(PATH=str(self.path) + os.pathsep + os.environ['PATH'],
                        FAKE_BUILD=str(self.remote), FAKE_LOG=str(self.log),
                        OCI_S3_ENDPOINT='https://synthetic.example', PYTHONDONTWRITEBYTECODE='1')

    def test_downloads_all_four_objects_with_path_style_and_size_checks(self):
        output = self.path / 'download'
        with patch.dict(os.environ, self.env, clear=True):
            self.storage.download('vn', '20990102_1', output)
        for name in ('corpus.dump', 'MANIFEST.sha256', 'release.json', 'report.json'):
            self.assertEqual((output / name).read_bytes(), (self.remote / name).read_bytes())
        calls = [json.loads(line) for line in self.log.read_text().splitlines()]
        self.assertEqual(sum('get-object' in call['args'] for call in calls), 4)
        self.assertGreater(min(i for i, call in enumerate(calls) if 'get-object' in call['args']),
                           max(i for i, call in enumerate(calls) if 'head-object' in call['args']))
        for call in calls:
            if 'get-object' in call['args']:
                args = call['args']
                name = args[args.index('--key') + 1].rsplit('/', 1)[1]
                expected = (self.remote / name).stat().st_size
                self.assertEqual(args[args.index('--range') + 1], f'bytes=0-{expected - 1}')
        for call in calls[1:]:
            self.assertEqual(call['checksum'], 'when_required')
            self.assertIn('addressing_style = path', call['config'])

    def test_size_mismatch_and_invalid_identity_stop_download(self):
        with patch.dict(os.environ, self.env | {'FAKE_BAD_SIZE': 'corpus.dump'}, clear=True):
            with self.assertRaises(ValueError):
                self.storage.download('vn', '20990102_1', self.path / 'download')
        self.log.unlink()
        with self.assertRaises(ValueError):
            self.storage.download('../', '20990102_1', self.path / 'unused')
        self.assertFalse(self.log.exists())

    def test_rejects_every_object_limit_before_downloading(self):
        limits = {'corpus.dump': 16 * 1024**3, 'report.json': 16 * 1024**2,
                  'release.json': 16 * 1024**2, 'MANIFEST.sha256': 1024**2}
        for name, limit in limits.items():
            with self.subTest(name=name), patch.dict(os.environ, self.env | {
                    'FAKE_HEAD_SIZES': json.dumps({name: limit + 1})}, clear=True):
                with self.assertRaises(ValueError):
                    self.storage.download('vn', '20990102_1', self.path / name)
                calls = [json.loads(line) for line in self.log.read_text().splitlines()]
                self.assertFalse(any('get-object' in call['args'] or 'cp' in call['args'] for call in calls))
                self.log.unlink()

    def test_checks_aggregate_against_free_space_with_two_gib_reserved(self):
        total = sum(path.stat().st_size for path in self.remote.iterdir())
        reserve = 2 * 1024**3
        for free in (reserve - 1, reserve + total - 1, reserve + total):
            with self.subTest(free=free), patch.dict(os.environ, self.env, clear=True), \
                    patch('shutil.disk_usage', return_value=SimpleNamespace(free=free)):
                with self.assertRaises(ValueError):
                    self.storage.download('vn', '20990102_1', self.path / str(free))
                calls = [json.loads(line) for line in self.log.read_text().splitlines()]
                self.assertFalse(any('get-object' in call['args'] or 'cp' in call['args'] for call in calls))
                self.log.unlink()

    def test_growth_after_head_cannot_exceed_the_requested_range(self):
        output = self.path / 'bounded'
        with patch.dict(os.environ, self.env | {'FAKE_GROW_AFTER_HEAD': 'corpus.dump'}, clear=True):
            self.storage.download('vn', '20990102_1', output)
        self.assertEqual((output / 'corpus.dump').read_bytes(), (self.remote / 'corpus.dump').read_bytes())
        with patch.dict(os.environ, self.env | {'FAKE_GROW_AFTER_HEAD': 'corpus.dump',
                                              'FAKE_IGNORE_RANGE': 'corpus.dump'}, clear=True):
            with self.assertRaises(ValueError):
                self.storage.download('vn', '20990102_1', self.path / 'bad-range')

    def test_logs_exact_aws_version(self):
        version = '2.31.0 Python/synthetic synthetic-os'
        with patch.dict(os.environ, self.env | {'FAKE_AWS_VERSION': version}, clear=True), \
                redirect_stdout(io.StringIO()) as output:
            self.storage.download('vn', '20990102_1', self.path / 'logged')
        self.assertIn('aws-cli/' + version, output.getvalue())


if __name__ == '__main__':
    unittest.main()
