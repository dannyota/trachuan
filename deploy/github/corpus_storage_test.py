import importlib
import json
import os
from pathlib import Path
import tempfile
import unittest
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
        self.assertEqual(sum('cp' in call['args'] for call in calls), 4)
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


if __name__ == '__main__':
    unittest.main()
