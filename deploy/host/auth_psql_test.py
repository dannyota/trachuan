import json
import os
from pathlib import Path
import pty
import re
import subprocess
import tempfile
import unittest

HOST = Path(__file__).resolve().parent
SCRIPT = HOST / 'auth-psql.sh'
OPTIONS = ('PGOPTIONS=-c search_path=pg_catalog,public -c statement_timeout=30s -c lock_timeout=5s '
           '-c idle_in_transaction_session_timeout=30s -c client_connection_check_interval=2s')
WRAPPER = ('set +x; set -eu; export PGPASSWORD; PGPASSWORD=$(cat /run/secrets/auth_db_password); '
           '[ -n "$PGPASSWORD" ]; exec "$@"')
FIXED = ['timeout', '--signal=TERM', '--kill-after=5s', '60s', 'env', 'PGCONNECT_TIMEOUT=5', OPTIONS,
         'psql', '--no-password', '-XqAt']
TAIL = ['-h', '127.0.0.1', '-U', 'auth_owner', '-d', 'auth', '-v', 'ON_ERROR_STOP=1']
ACTIVE = "SELECT active_db FROM public.corpus_release WHERE country = 'vn';"
PODMAN = '''#!/usr/bin/env python3
import json, os, sys
with open(os.environ['CALLS'], 'a') as out:
    out.write(json.dumps(sys.argv[1:]) + '\\n')
sys.stdout.buffer.write(os.environ.get('ANSWER', '').encode().decode('unicode_escape').encode('latin-1'))
sys.exit(int(os.environ.get('EXIT', '0')))
'''


class AuthPsqlTest(unittest.TestCase):
    def run_script(self, *args, stdin=b'', answer='', code=0, terminal=False):
        with tempfile.TemporaryDirectory() as directory:
            work = Path(directory)
            (work / 'podman').write_text(PODMAN)
            (work / 'podman').chmod(0o755)
            env = dict(os.environ, PATH=str(work) + os.pathsep + os.environ['PATH'], CALLS=str(work / 'calls'),
                       ANSWER=answer, EXIT=str(code))
            options = dict(input=stdin)
            if terminal:
                primary, secondary = pty.openpty()
                self.addCleanup(os.close, primary)
                self.addCleanup(os.close, secondary)
                options = dict(stdin=secondary)
            result = subprocess.run(['bash', str(SCRIPT), *args], env=env, capture_output=True, **options)
            calls = (work / 'calls').read_text().splitlines() if (work / 'calls').exists() else []
            return result, [json.loads(line) for line in calls]

    def expected(self, attach, *args):
        return ['exec', *attach, 'trachuan-postgres', 'sh', '-c', WRAPPER, 'sh', *FIXED, *args, *TAIL]

    def test_piped_stdin_is_attached(self):
        result, calls = self.run_script('-v', 'db=corpus_vn_20990101', stdin=b'SELECT 1;\n')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, [self.expected(['-i'], '-v', 'db=corpus_vn_20990101')])

    def test_a_terminal_is_not_attached_so_pasted_lines_stay_with_the_shell(self):
        result, calls = self.run_script('-c', 'SELECT 1;', terminal=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, [self.expected([], '-c', 'SELECT 1;')])

    def test_a_terminal_without_a_command_is_refused(self):
        result, calls = self.run_script(terminal=True)
        self.assertEqual(result.returncode, 2)
        self.assertEqual(calls, [])

    def test_long_forms_are_accepted(self):
        result, calls = self.run_script('--set=a=1', '--variable=b=2', '--command=SELECT 1;')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls[0][-len(TAIL) - 3:-len(TAIL)], ['--set=a=1', '--variable=b=2', '--command=SELECT 1;'])

    def test_arguments_that_could_change_the_connection_are_refused(self):
        for args in (['-d', 'postgres'], ['--dbname=postgres'], ['-dpostgres'], ['-h', '192.0.2.1'], ['--host=192.0.2.1'],
                     ['-U', 'postgres'], ['--username=postgres'], ['-p', '5999'], ['--port=5999'],
                     ['host=192.0.2.1 dbname=auth'], ['postgresql://192.0.2.1/auth'], ['auth'], ['-f', '/tmp/x'],
                     ['-c'], ['-v'], ['-cSELECT 1'], ['-X'], ['--']):
            with self.subTest(args=args):
                result, calls = self.run_script(*args, stdin=b'')
                self.assertEqual(result.returncode, 2)
                self.assertEqual(calls, [])

    def test_failure_of_the_session_is_returned(self):
        result, _ = self.run_script('-c', 'SELECT 1;', code=3)
        self.assertEqual(result.returncode, 3)

    def test_active_db_prints_a_valid_law_database(self):
        for name in ('corpus_vn_20990101', 'corpus_vn_20990101_12'):
            with self.subTest(name=name):
                result, calls = self.run_script('active-db', answer=name + '\\n')
                self.assertEqual((result.returncode, result.stdout), (0, name.encode() + b'\n'), result.stderr)
                self.assertEqual(calls, [self.expected(['-i'], '-c', ACTIVE)])

    def test_active_db_refuses_anything_else(self):
        for answer in ('', '\\n', 'auth\\n', 'corpus_vn_20990101 \\n', ' corpus_vn_20990101\\n', 'corpus_vn_2099010\\n',
                       'corpus_VN_20990101\\n', 'corpus_vn_20990101_0\\n', 'corpus_vn_20990101_\\n',
                       'corpus_vn_20990101\\ncorpus_vn_20990102\\n', 'frameworks_20990101\\n',
                       'dbname=corpus_vn_20990101\\n', 'corpus_vn_20990101_' + '1' * 60 + '\\n',
                       'corpus_vn_20990101_' + '1' * 200 + '\\n'):
            with self.subTest(answer=answer):
                result, _ = self.run_script('active-db', answer=answer)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(result.stdout, b'')

    def test_active_db_takes_no_arguments_and_reports_a_failed_read(self):
        self.assertEqual(self.run_script('active-db', '-c', 'SELECT 1;')[0].returncode, 2)
        result, _ = self.run_script('active-db', answer='corpus_vn_20990101\\n', code=2)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(result.stdout, b'')

    def test_active_query_is_the_one_the_updater_runs(self):
        from updater.deploy_corpus_sql import AUTH_ACTIVE_DATABASE
        self.assertEqual(ACTIVE, AUTH_ACTIVE_DATABASE)

    def test_script_is_installed_with_the_bundle(self):
        listing = re.search(r'for script in (bootstrap\.sh.*?); do', (HOST / 'install.sh').read_text(), re.S)[1]
        self.assertIn('auth-psql.sh', listing.split())
        self.assertTrue(os.access(SCRIPT, os.X_OK))


if __name__ == '__main__':
    unittest.main()
