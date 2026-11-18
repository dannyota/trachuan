import importlib
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch


class FrameworksPathsTest(unittest.TestCase):
    def setUp(self):
        self.module = importlib.import_module('frameworks_paths')
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(os.path.realpath(self.temp.name))
        self.home = self.root / 'home'
        self.home.mkdir()
        self.work = self.root / 'work'
        (self.work / '20990102').mkdir(parents=True)
        patcher = patch.dict(os.environ, dict(HOME=str(self.home), FRAMEWORKS_WORK_DIR=str(self.work)))
        patcher.start()
        self.addCleanup(patcher.stop)

    def refused(self, path, name=None):
        with self.assertRaisesRegex(ValueError, 'sync folder refused') as caught:
            self.module.checked_directory(str(path))
        if name:
            self.assertIn(name, str(caught.exception))

    def test_refuses_every_sync_root_the_build_refuses(self):
        for folder in ('Library/Mobile Documents', 'Library/CloudStorage/Provider', 'Dropbox', 'Google Drive',
                       'OneDrive', 'OneDrive - Company', 'Desktop', 'Documents'):
            with self.subTest(folder=folder):
                root = self.home / folder
                root.mkdir(parents=True)
                self.refused(root / 'nested')
                self.refused(root)

    def test_case_variants_and_uncreated_onedrive_are_refused(self):
        (self.home / 'dropbox').mkdir()
        self.refused(self.home / 'dropbox' / 'nested')
        self.refused(self.home / 'OneDrive - NewCompany' / 'artifacts', 'OneDrive - NewCompany')

    def test_reported_root_and_marker_are_refused(self):
        reported = self.root / 'external'
        reported.mkdir()
        (self.home / '.dropbox').mkdir()
        (self.home / '.dropbox/info.json').write_text(json.dumps(dict(business=dict(path=str(reported)))))
        self.refused(reported / 'nested', 'external')
        marked = self.root / 'marked'
        (marked / 'deep').mkdir(parents=True)
        (marked / '.dropbox').write_text('')
        self.refused(marked / 'deep' / 'nested', 'marked')

    def test_configuration_directory_in_home_is_not_a_marker(self):
        (self.home / '.dropbox').mkdir()
        (self.home / '.dropbox/info.json').write_text('{"personal":{"path":"/invented/sync"}}')
        self.assertEqual(self.module.checked_directory(str(self.home / 'ordinary')), str(self.home / 'ordinary'))

    def test_invalid_client_configuration_refuses(self):
        (self.home / '.dropbox').mkdir()
        (self.home / '.dropbox/info.json').write_text('[]')
        with self.assertRaises(ValueError):
            self.module.checked_directory(str(self.work))

    def test_symlink_resolves_before_parent(self):
        child = self.home / 'Documents' / 'child'
        child.mkdir(parents=True)
        link = self.root / 'link'
        link.symlink_to(child)
        self.refused(str(link) + '/../source', 'Documents')
        self.refused(link)

    def test_missing_parent_and_file_parent_fail(self):
        with self.assertRaises(ValueError):
            self.module.real_path(str(self.root / 'missing' / '..' / 'x'))
        (self.root / 'file').write_text('')
        with self.assertRaises(ValueError):
            self.module.real_path(str(self.root / 'file' / 'x'))
        dangling = self.root / 'dangling'
        dangling.symlink_to(self.root / 'absent')
        with self.assertRaises(ValueError):
            self.module.real_path(str(dangling))

    def test_release_directory_requires_setting_existence_and_containment(self):
        self.assertEqual(self.module.release_directory('20990102'), self.work / '20990102')
        with self.assertRaises(ValueError):
            self.module.release_directory('20990103')
        outside = self.root / 'outside'
        outside.mkdir()
        (self.work / '20990104').symlink_to(outside)
        with self.assertRaises(ValueError):
            self.module.release_directory('20990104')
        with patch.dict(os.environ):
            del os.environ['FRAMEWORKS_WORK_DIR']
            with self.assertRaises(ValueError):
                self.module.release_directory('20990102')

    def test_source_file_must_be_a_regular_file_under_a_clean_directory(self):
        base = self.work / '20990102'
        (base / 'build').mkdir()
        (base / 'build/build.json').write_text('{}')
        self.assertEqual(self.module.source_file(base, 'build/build.json'), base / 'build/build.json')
        (base / 'build/link.json').symlink_to(base / 'build/build.json')
        for name in ('build/link.json', 'build/absent.json', 'build'):
            with self.subTest(name=name), self.assertRaises(ValueError):
                self.module.source_file(base, name)
        drive = self.home / 'Dropbox' / 'evaluate'
        drive.mkdir(parents=True)
        (drive / 'report.json').write_text('{}')
        (base / 'evaluate').symlink_to(drive)
        with self.assertRaisesRegex(ValueError, 'sync folder refused'):
            self.module.source_file(base, 'evaluate/report.json')

    def test_private_directory_is_fresh_private_and_removed_on_every_exit(self):
        base = self.work / '20990102'
        stale = base / '.upload'
        stale.mkdir()
        (stale / 'old').write_text('stale')
        with self.module.private_directory(base) as path:
            self.assertEqual(path, stale)
            self.assertEqual(path.stat().st_mode & 0o777, 0o700)
            self.assertEqual(list(path.iterdir()), [])
            (path / 'part').write_text('x')
        self.assertFalse(stale.exists())
        with self.assertRaisesRegex(RuntimeError, 'boom'):
            with self.module.private_directory(base) as path:
                (path / 'part').write_text('x')
                raise RuntimeError('boom')
        self.assertFalse(stale.exists())

    def test_private_directory_refuses_symlink_and_keeps_its_target(self):
        base = self.work / '20990102'
        target = base / 'build'
        target.mkdir()
        (target / 'keep').write_text('keep')
        (base / '.upload').symlink_to(target)
        with self.assertRaises(ValueError):
            with self.module.private_directory(base):
                self.fail('entered')
        self.assertEqual((target / 'keep').read_text(), 'keep')


if __name__ == '__main__':
    unittest.main()
