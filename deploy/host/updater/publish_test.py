import os
from pathlib import Path
import stat
import tempfile
import unittest
from unittest import mock

from updater import publish


class PublishTest(unittest.TestCase):
    def test_invalid_or_interrupted_policy_publication_preserves_previous_file(self):
        with tempfile.TemporaryDirectory() as directory:
            target = Path(directory) / 'deploy-policy.json'
            target.write_bytes(b'previous policy')
            source = target.with_name('source.json')
            source.write_bytes(b'{}')
            with self.assertRaises(ValueError):
                publish.file(source, target, policy_file=True)
            self.assertEqual(target.read_bytes(), b'previous policy')
            source.write_bytes((Path(__file__).parents[1] / 'deploy-policy.json').read_bytes())
            with mock.patch('updater.deploy_io.os.replace', side_effect=InterruptedError('synthetic')):
                with self.assertRaises(InterruptedError):
                    publish.file(source, target, policy_file=True)
            self.assertEqual(target.read_bytes(), b'previous policy')
            self.assertEqual(sorted(p.name for p in Path(directory).iterdir()), ['deploy-policy.json', 'source.json'])

    def test_policy_is_fsynced_before_and_after_atomic_rename(self):
        events = []
        fsync, replace = os.fsync, os.replace
        def sync(fd):
            events.append('directory' if stat.S_ISDIR(os.fstat(fd).st_mode) else 'file')
            fsync(fd)
        def rename(source, target):
            events.append('rename')
            replace(source, target)
        with tempfile.TemporaryDirectory() as directory:
            source = Path(__file__).parents[1] / 'deploy-policy.json'
            target = Path(directory) / 'deploy-policy.json'
            with mock.patch('updater.deploy_io.os.fsync', side_effect=sync), \
                 mock.patch('updater.deploy_io.os.replace', side_effect=rename):
                publish.file(source, target, policy_file=True)
            self.assertEqual(events, ['file', 'rename', 'directory'])
            self.assertEqual(target.read_bytes(), source.read_bytes())
            self.assertEqual(target.stat().st_mode & 0o777, 0o644)


if __name__ == '__main__':
    unittest.main()
