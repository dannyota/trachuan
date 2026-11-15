from pathlib import Path
import shlex
import subprocess
import tempfile
import unittest

HOST = Path(__file__).resolve().parent


class LogInstallTest(unittest.TestCase):
    def test_log_provisioning_preserves_existing_lines_and_grants_writer_access(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            log = root / 'backup.log'
            calls = root / 'calls'
            source = (HOST / 'install.sh').read_text()
            start = source.index('setfacl -m u:trachuan:--x /var/log/trachuan')
            block = source[start:source.index('setfacl -m u:trachuan-metrics:r-x,d:u:trachuan-metrics:r-x /srv/trachuan/state/deploy')]
            block = block.replace('/var/log/trachuan', str(root))
            prelude = ('set -euo pipefail\n'
                       'record() { printf "%s\\n" "$*" >>' + shlex.quote(str(calls)) + '; }\n'
                       'setfacl() { record setfacl "$@"; }\n'
                       'chown() { record chown "$@"; }\n'
                       'install() { record install "$@"; shift 4; command install "$@"; }\n'
                       'fail() { exit 1; }\n')
            for existing in (False, True):
                with self.subTest(existing=existing):
                    if existing:
                        log.write_text('synthetic existing line\n')
                    result = subprocess.run(['bash', '-c', prelude+block], capture_output=True, text=True)
                    self.assertEqual(result.returncode, 0, result.stderr)
                    self.assertEqual(log.read_text(), 'synthetic existing line\n' if existing else '')
                    self.assertEqual(log.stat().st_mode & 0o777, 0o640)
            rows = calls.read_text().splitlines()
            self.assertIn(f'setfacl -m u:trachuan:--x {root}', rows)
            self.assertIn(f'chown trachuan:trachuan {log}', rows)
            self.assertEqual(rows.count(f'install -o trachuan -g trachuan -m 0640 /dev/null {log}'), 1)


if __name__ == '__main__':
    unittest.main()
