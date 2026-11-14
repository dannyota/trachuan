import importlib.util
import json
from datetime import datetime, timezone
from pathlib import Path
import sys
import tempfile
import types
import unittest
from unittest import mock

sys.dont_write_bytecode = True
HOST = Path(__file__).resolve().parent
sys.modules.setdefault("oci", types.ModuleType("oci"))
spec = importlib.util.spec_from_file_location("publisher", HOST / "metrics/publish-metrics.py")
publisher = importlib.util.module_from_spec(spec)


class MetricsTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        spec.loader.exec_module(publisher)

    def test_filesystem_uses_available_blocks_for_percentage(self):
        stats = types.SimpleNamespace(f_blocks=100, f_bfree=20, f_bavail=10, f_frsize=4096)
        with mock.patch.object(publisher.os, "statvfs", return_value=stats):
            used, free = publisher.filesystem("/")
        self.assertAlmostEqual(used, 80 / 90 * 100)
        self.assertEqual(free, 40960)

    def test_backup_missing_and_statuses(self):
        now = datetime(2026, 1, 2, tzinfo=timezone.utc)
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "full.json"
            self.assertEqual(publisher.backup(path, now), [])
            for status in ("ok", "failed", "unknown", "skipped", "running"):
                with self.subTest(status=status):
                    path.write_text(json.dumps({"status": status}))
                    self.assertEqual(publisher.backup(path, now), [("backup_failed", int(status in ("failed", "unknown")))])
                    path.write_text(json.dumps({"status": status, "last_success_at": "2026-01-01T00:00:00Z"}))
                    self.assertEqual(publisher.backup(path, now), [
                        ("backup_failed", int(status in ("failed", "unknown"))),
                        ("backup_last_success_age_seconds", 86400),
                    ])

    def test_metadata_and_ingestion_use_instance_principal(self):
        sdk = mock.MagicMock()
        response = mock.MagicMock()
        response.__enter__.return_value.read.return_value = b'{"compartmentId":"compartment","region":"region-1"}'
        with mock.patch.object(publisher, "oci", sdk, create=True), \
             mock.patch.object(publisher, "urlopen", return_value=response) as urlopen, \
             mock.patch.object(publisher.signal, "signal"), \
             mock.patch.object(publisher.signal, "alarm") as alarm, \
             mock.patch.object(publisher, "collect", return_value=[]), \
             mock.patch.object(publisher, "post") as post:
            publisher.main()
        request = urlopen.call_args.args[0]
        self.assertEqual(request.full_url, "http://169.254.169.254/opc/v2/instance/")
        self.assertEqual(request.get_header("Authorization"), "Bearer Oracle")
        self.assertEqual(urlopen.call_args.kwargs["timeout"], 20)
        alarm.assert_called_once_with(20)
        sdk.auth.signers.InstancePrincipalsSecurityTokenSigner.assert_called_once()
        self.assertEqual(sdk.monitoring.MonitoringClient.call_args.kwargs["service_endpoint"],
                         "https://telemetry-ingestion.region-1.oraclecloud.com")
        self.assertEqual(post.call_args.args[1], "compartment")

    def test_payload_names_dimensions_and_sdk_call(self):
        now = datetime(2026, 1, 2, tzinfo=timezone.utc)
        with tempfile.TemporaryDirectory() as directory, \
             mock.patch.object(publisher, "BACKUP_DIR", Path(directory)), \
             mock.patch.object(publisher, "filesystem", return_value=(25, 100)), \
             mock.patch.object(publisher.os.path, "ismount", return_value=True):
            (Path(directory) / "full.json").write_text('{"status":"failed","last_success_at":"2026-01-01T00:00:00Z","private":"ignore"}')
            metrics = publisher.collect("host", now)
        self.assertEqual({m[0] for m in metrics}, {"heartbeat", "filesystem_used_percent", "filesystem_free_bytes", "data_mounted", "backup_failed", "backup_last_success_age_seconds"})
        for name, _, dimensions in metrics:
            expected = {"host", "mount"} if name.startswith("filesystem_") else {"host", "job"} if name.startswith("backup_") else {"host"}
            self.assertEqual(set(dimensions), expected)
            self.assertNotIn("private", str(dimensions))
        sdk = mock.MagicMock()
        publisher.oci = mock.MagicMock()
        publisher.oci.monitoring.models.PostMetricDataDetails.side_effect = lambda **kw: types.SimpleNamespace(**kw)
        sdk.post_metric_data.return_value.data.failed_metrics_count = 0
        publisher.post(sdk, "compartment", metrics, now)
        sdk.post_metric_data.assert_called_once()
        details = sdk.post_metric_data.call_args.args[0]
        self.assertEqual(len(details.metric_data), len(metrics))


if __name__ == "__main__":
    unittest.main()
