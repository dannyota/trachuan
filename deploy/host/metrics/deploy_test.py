from datetime import datetime, timezone
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock

spec = importlib.util.spec_from_file_location('deploy_publisher', Path(__file__).with_name('publish-metrics.py'))
publisher = importlib.util.module_from_spec(spec)
spec.loader.exec_module(publisher)


class DeployMetricsTest(unittest.TestCase):
    def test_failure_statuses_and_last_success_are_published(self):
        now = datetime(2026, 1, 2, tzinfo=timezone.utc)
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'result.json'
            self.assertEqual(publisher.deploy(path, now), [])
            for status in ('failed', 'rolled_back', 'stopped', 'ok', 'verified', 'skipped', 'bootstrapped', 'current'):
                with self.subTest(status=status):
                    path.write_text(json.dumps({'status': status, 'last_success_at': '2026-01-01T00:00:00Z'}))
                    self.assertEqual(publisher.deploy(path, now), [
                        ('deploy_failed', int(status in ('failed', 'rolled_back', 'stopped'))),
                        ('deploy_last_success_age_seconds', 86400)])
            with mock.patch.object(publisher, 'DEPLOY_RESULT', path), \
                 mock.patch.object(publisher, 'BACKUP_DIR', Path(directory)), \
                 mock.patch.object(publisher, 'filesystem', return_value=(25, 100)):
                metrics = publisher.collect('test-host', now)
            self.assertIn(('deploy_failed', 0, {'host': 'test-host'}), metrics)

    def test_secrets_metrics_are_collected_with_success_age(self):
        now = datetime(2026, 1, 2, tzinfo=timezone.utc)
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'secrets.json'
            for status in ('ok', 'current', 'failed', 'skipped'):
                path.write_text(json.dumps({'status': status, 'last_success_at': '2026-01-01T00:00:00Z'}))
                with mock.patch.object(publisher, 'SECRETS_RESULT', path), \
                     mock.patch.object(publisher, 'DEPLOY_RESULT', Path(directory) / 'missing'), \
                     mock.patch.object(publisher, 'BACKUP_DIR', Path(directory)), \
                     mock.patch.object(publisher, 'filesystem', return_value=(25, 100)):
                    metrics = publisher.collect('test-host', now)
                self.assertIn(('secrets_failed', int(status == 'failed'), {'host': 'test-host'}), metrics)
                self.assertIn(('secrets_last_success_age_seconds', 86400, {'host': 'test-host'}), metrics)

    def test_deploy_result_rejects_symlinks_oversize_and_unknown_status(self):
        now = datetime(2026, 1, 2, tzinfo=timezone.utc)
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'result.json'
            path.write_bytes(b'x' * 65537)
            with self.assertRaises(ValueError):
                publisher.deploy(path, now)
            path.write_text('{"status":"invalid"}')
            with self.assertRaises(ValueError):
                publisher.deploy(path, now)
            link = path.with_name('link')
            link.symlink_to(path)
            with self.assertRaises(OSError):
                publisher.deploy(link, now)


if __name__ == '__main__':
    unittest.main()
