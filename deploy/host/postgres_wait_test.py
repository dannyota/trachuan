"""Wait for database readiness without sleeping in unit tests."""

import os
from pathlib import Path
import subprocess
import tempfile
import unittest

HOST = Path(__file__).resolve().parent


class ReadinessTest(unittest.TestCase):
    def run_wait(self, host, timeout, ready_after):
        with tempfile.TemporaryDirectory(prefix="trachuan-ready-test-") as work:
            work = Path(work)
            counter = work / "probes"
            counter.write_text("0")
            probe = work / "pg_isready"
            probe.write_text('''#!/usr/bin/env python3
import os, pathlib, sys
args=sys.argv[1:]
expected=['--host', os.environ['READY_TEST_HOST'], '--port', '5432', '--username', 'postgres', '--dbname', 'postgres', '--timeout', '2']
if args != expected: sys.exit(90)
counter=pathlib.Path(os.environ['READY_TEST_COUNTER'])
count=int(counter.read_text())+1
counter.write_text(str(count))
sys.exit(0 if count >= int(os.environ['READY_TEST_AFTER']) else 1)
''')
            probe.chmod(0o755)
            sleeper = work / "sleep"
            sleeper.write_text("#!/bin/sh\nexit 0\n")
            sleeper.chmod(0o755)
            env = dict(os.environ, PATH=str(work) + os.pathsep + os.environ["PATH"],
                       READY_TEST_HOST=host, READY_TEST_COUNTER=str(counter), READY_TEST_AFTER=str(ready_after))
            result = subprocess.run(["bash", str(HOST / "wait-postgres.sh"), host, str(timeout)],
                                    env=env, text=True, capture_output=True, timeout=5)
            return result, int(counter.read_text())

    def test_retries_until_tcp_or_socket_is_ready(self):
        for host in ["127.0.0.1", "/var/run/postgresql"]:
            with self.subTest(host=host):
                result, probes = self.run_wait(host, 120, 3)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(probes, 3)

    def test_expired_deadline_fails_without_more_probes(self):
        result, probes = self.run_wait("127.0.0.1", 0, 10)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("timed out", result.stderr)
        self.assertEqual(probes, 0)


if __name__ == "__main__":
    unittest.main()
