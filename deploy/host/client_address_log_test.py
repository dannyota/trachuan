"""Restore default logging and remove every proof log after diagnostics."""

import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

HOST = Path(__file__).resolve().parent


class LogLifecycleTest(unittest.TestCase):
    def run_case(self, work, action, fail_restart=False):
        work = Path(work)
        (work / "runtime").mkdir(exist_ok=True, mode=0o700)
        binary_dir = work / "bin"
        binary_dir.mkdir(exist_ok=True)
        (binary_dir / "systemctl").write_text('''#!/usr/bin/env python3
import os, sys
sys.exit(1 if os.environ.get('FAIL_RESTART') == '1' and 'restart' in sys.argv else 0)
''')
        (binary_dir / "podman").write_text('''#!/usr/bin/env python3
import subprocess, sys
if sys.argv[1] != 'unshare': sys.exit(90)
sys.exit(subprocess.run(sys.argv[2:]).returncode)
''')
        for binary in binary_dir.iterdir():
            binary.chmod(0o755)
        script = work / "client-address-log.sh"
        if (HOST / script.name).exists():
            shutil.copyfile(HOST / script.name, script)
        snippet = HOST / "client-address-check.caddy"
        (work / snippet.name).write_text(snippet.read_text() if snippet.exists() else "log client_address_check\n")
        env = dict(os.environ, PATH=str(binary_dir) + os.pathsep + os.environ["PATH"],
                   XDG_RUNTIME_DIR=str(work / "runtime"), FAIL_RESTART=str(int(fail_restart)))
        return subprocess.run(["bash", str(script), action], env=env, text=True, capture_output=True)

    def test_logging_requires_explicit_enable_then_disable(self):
        with tempfile.TemporaryDirectory(prefix="trachuan-log-test-") as work:
            proof = Path(work) / "runtime/trachuan-caddy-proof/client-address-check.caddy"
            self.assertEqual(self.run_case(work, "prepare").returncode, 0)
            self.assertFalse(proof.exists())
            self.assertEqual(self.run_case(work, "enable").returncode, 0)
            self.assertTrue(proof.exists())
            self.assertEqual(self.run_case(work, "disable").returncode, 0)
            self.assertFalse(proof.exists())

    def test_cleanup_removes_active_rotated_and_compressed_logs_even_after_restart_failure(self):
        for failure in [False, True]:
            with self.subTest(restart_failure=failure), tempfile.TemporaryDirectory(prefix="trachuan-log-test-") as work:
                root = Path(work) / "runtime"
                proof = root / "trachuan-caddy-proof/client-address-check.caddy"
                proof.parent.mkdir(parents=True)
                proof.write_text("log client_address_check\n")
                logs = root / "trachuan-caddy-proof-logs"
                logs.mkdir()
                for name in ["client-address-check.json", "client-address-check-old.json",
                             "client-address-check-old.json.gz", "client-address-check-old.json.zst", "unrelated"]:
                    (logs / name).write_text("fixture\n")
                result = self.run_case(work, "disable", fail_restart=failure)
                self.assertEqual(result.returncode == 0, not failure, result.stderr)
                self.assertFalse(proof.exists())
                self.assertEqual([p.name for p in logs.iterdir()], ["unrelated"])


@unittest.skipUnless(shutil.which("caddy"), "native Caddy is unavailable")
class ProofSnippetTest(unittest.TestCase):
    def test_explicit_snippet_enables_only_the_bounded_proof_log(self):
        with tempfile.TemporaryDirectory(prefix="trachuan-caddy-proof-test-") as work:
            work = Path(work)
            proof = work / "proof"
            proof.mkdir()
            shutil.copyfile(HOST / "client-address-check.caddy", proof / "client-address-check.caddy")
            config_file = work / "Caddyfile"
            config_file.write_text((HOST / "Caddyfile").read_text().replace("/etc/caddy/proof/*.caddy", str(proof / "*.caddy")))
            config = json.loads(subprocess.check_output([
                "caddy", "adapt", "--config", str(config_file), "--adapter", "caddyfile",
            ], text=True))
            log = config["logging"]["logs"]["client_address_check"]
            self.assertEqual(log["writer"]["filename"], "/var/log/caddy-proof/client-address-check.json")
            self.assertEqual(log["writer"]["roll_size_mb"], 1)
            self.assertEqual(log["encoder"]["fields"]["request>headers"]["filter"], "delete")
            self.assertEqual(log["encoder"]["fields"]["request>uri"]["filter"], "delete")


if __name__ == "__main__":
    unittest.main()
