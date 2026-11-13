"""Exercise the passwordless-connection check without a PostgreSQL service."""

import os
from pathlib import Path
import subprocess
import tempfile
import unittest

HOST = Path(__file__).resolve().parent
FAKE_PODMAN = r'''#!/usr/bin/env python3
import os, sys
args = sys.argv[1:]
mode = os.environ['POSTGRES_AUTH_TEST_MODE']
if not args or args[0] != 'exec': sys.exit(90)
if '/usr/local/bin/wait-postgres.sh' in args:
    if mode == 'unavailable': sys.exit(2)
    open(os.environ['POSTGRES_AUTH_TEST_READY'], 'w').close()
    sys.exit(0)
if not os.path.exists(os.environ['POSTGRES_AUTH_TEST_READY']): sys.exit(92)
if '/usr/local/bin/postgres-admin.sh' in args:
    if mode == 'unavailable': sys.exit(2)
    print('1'); sys.exit(0)
for setting in ['PGPASSWORD=', 'PGPASSFILE=/dev/null']:
    if setting not in args: sys.exit(91)
# An empty PGSERVICE makes libpq look up a service named "" and fail for the wrong reason.
if any(a.startswith(('PGSERVICE=', 'PGHOSTADDR=')) for a in args): sys.exit(93)
host = args[args.index('--host') + 1]
if mode == 'tcp-trust' and host == '127.0.0.1': print('1'); sys.exit(0)
if mode == 'socket-trust' and host == '/var/run/postgresql': print('1'); sys.exit(0)
message = 'connection refused' if mode == 'network-error' else 'fe_sendauth: no password supplied'
print(message, file=sys.stderr)
sys.exit(2)
'''


class PasswordCheckTest(unittest.TestCase):
    def run_check(self, mode):
        with tempfile.TemporaryDirectory(prefix="trachuan-auth-test-") as work:
            fake = Path(work) / "podman"
            fake.write_text(FAKE_PODMAN)
            fake.chmod(0o755)
            env = dict(os.environ, PATH=work + os.pathsep + os.environ["PATH"], POSTGRES_AUTH_TEST_MODE=mode,
                       POSTGRES_AUTH_TEST_READY=str(Path(work) / "ready"))
            return subprocess.run(["bash", str(HOST / "check-postgres-auth.sh")], env=env, text=True, capture_output=True)

    def test_authenticated_service_rejecting_passwordless_connections_passes(self):
        result = self.run_check("scram")
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_trust_and_unavailable_services_fail_closed(self):
        for mode in ["tcp-trust", "socket-trust", "unavailable", "network-error"]:
            with self.subTest(mode=mode):
                result = self.run_check(mode)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("postgres-auth:", result.stderr)


if __name__ == "__main__":
    unittest.main()
