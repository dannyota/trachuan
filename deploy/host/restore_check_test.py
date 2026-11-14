import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

HOST = Path(__file__).resolve().parent


class RestoreCheckTest(unittest.TestCase):
    def run_check(self, fail):
        self.assertTrue((HOST / 'restore-check.sh').exists(), 'restore check is missing')
        with tempfile.TemporaryDirectory(prefix='trachuan-restore-test-') as work:
            work = Path(work)
            podman = work / 'podman'
            podman.write_text('''#!/usr/bin/env python3
import json, os, pathlib, sys
args=sys.argv[1:]
with pathlib.Path(os.environ['CALLS']).open('a') as out: out.write(json.dumps(args)+'\\n')
if args[0]=='run':
    sys.exit(1 if os.environ['FAIL']=='run' else 0)
if args[0]=='wait':
    if os.environ['FAIL']=='signal':
        import signal
        os.kill(os.getppid(), signal.SIGTERM)
    print('1' if os.environ['FAIL']=='wait' else '0')
''')
            podman.chmod(0o755)
            env = dict(os.environ, PATH=str(work)+os.pathsep+os.environ['PATH'], CALLS=str(work / 'calls'), FAIL=fail)
            result = subprocess.run(['bash', str(HOST / 'restore-check.sh'), '--locked',
                                     '20270115-020000F', 'sha256:'+'1'*64], env=env, capture_output=True, text=True)
            calls = [json.loads(line) for line in (work / 'calls').read_text().splitlines()]
            return result, calls

    def test_isolation_same_image_only_repository_keys_and_cleanup(self):
        result, calls = self.run_check('')
        self.assertEqual(result.returncode, 0, result.stderr)
        run = next(call for call in calls if call[0] == 'run')
        for option in ['--network=none', '--memory=2g', '--cpus=1', '--user=999:999', '--read-only', '--cap-drop=ALL']:
            self.assertIn(option, run)
        secrets = [arg for arg in run if arg.startswith('--secret=')]
        self.assertEqual(secrets, ['--secret=backup_s3_key,type=mount,uid=999,gid=999,mode=0400',
                                   '--secret=backup_s3_key_secret,type=mount,uid=999,gid=999,mode=0400'])
        self.assertIn('sha256:'+'1'*64, run)
        self.assertEqual(run[-3:], ['--repository-only', '/usr/local/bin/restore-auth.sh', '20270115-020000F'])
        self.assertFalse(any('/data/postgres' in arg or '/data/pgbackrest' in arg for arg in run))
        self.assertEqual(calls[:2], [['rm', '--force', '--ignore', 'trachuan-restore-check'],
                                      ['unshare', 'rm', '-rf', '--', '/srv/trachuan/data/restore-check']])
        self.assertEqual(calls[-2:], calls[:2])

    def test_cleanup_after_container_start_or_recovery_failure(self):
        for fail in ['run', 'wait', 'signal']:
            with self.subTest(fail=fail):
                result, calls = self.run_check(fail)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(calls[-2:], [['rm', '--force', '--ignore', 'trachuan-restore-check'],
                                              ['unshare', 'rm', '-rf', '--', '/srv/trachuan/data/restore-check']])

    def test_restore_arguments_and_peer_only_postgres(self):
        self.assertTrue((HOST / 'restore-auth.sh').exists(), 'restore worker is missing')
        with tempfile.TemporaryDirectory() as work:
            work = Path(work)
            for binary in ['pgbackrest', 'postgres', 'psql']:
                stub = work / binary
                stub.write_text('''#!/usr/bin/env python3
import json, os, pathlib, sys
with pathlib.Path(os.environ['CALLS']).open('a') as out: out.write(json.dumps([pathlib.Path(sys.argv[0]).name, *sys.argv[1:]])+'\\n')
if pathlib.Path(sys.argv[0]).name == 'psql': print('f')
''')
                stub.chmod(0o755)
            env = dict(os.environ, PATH=str(work)+os.pathsep+os.environ['PATH'],
                       CALLS=str(work/'calls'), RESTORE_RUNTIME=str(work/'runtime'), PGDATA=str(work/'data'))
            result = subprocess.run(['bash', str(HOST / 'restore-auth.sh'), '20270115-020000F'],
                                    env=env, capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            calls = [json.loads(line) for line in (work/'calls').read_text().splitlines()]
            restore = next(call for call in calls if call[0] == 'pgbackrest')
            for arg in ['--set=20270115-020000F', '--db-include=auth', '--type=immediate', '--target-action=promote', 'restore']:
                self.assertIn(arg, restore)
            self.assertEqual((work/'runtime/pg_hba.conf').read_text(), 'local all postgres peer\n')
            postgres = next(call for call in calls if call[0] == 'postgres')
            self.assertIn('listen_addresses=', postgres)
            self.assertIn('archive_mode=off', postgres)
            self.assertIn('hba_file='+str(work/'runtime/pg_hba.conf'), postgres)
            smoke = next(call for call in calls if call[0] == 'psql' and 'pg_is_in_recovery' not in ' '.join(call))
            self.assertIn('--dbname=auth', smoke)
            self.assertIn('--no-password', smoke)


if __name__ == '__main__':
    unittest.main()
