import json
import os
from pathlib import Path
import re
import stat
import subprocess
import tempfile
import unittest

HOST = Path(__file__).resolve().parent
UNQUALIFIED = re.compile(r'(?<![\w.])(format|to_regclass|unnest|pg_roles|pg_database|pg_db_role_setting)\b')
SESSION = re.compile(r"PGOPTIONS='([^']*)' \\\n(.*?) <<'SQL'\n(.*?)\nSQL\n", re.S)
AUTH_LIMITS = ('statement_timeout=30s', 'lock_timeout=5s', 'idle_in_transaction_session_timeout=30s',
               'client_connection_check_interval=2s')
RECORDER = '''#!/usr/bin/env python3
import json, os, sys
keys = ('PGPASSWORD', 'PGUSER', 'PGPASSFILE', 'PGSERVICEFILE', 'PGCONNECT_TIMEOUT', 'PGOPTIONS', 'ACTIVATOR_DB_PASSWORD')
with open(os.environ['SESSIONS'], 'a') as out:
    out.write(json.dumps(dict(args=sys.argv[1:], sql=sys.stdin.read(), env={k: os.environ.get(k) for k in keys})) + '\\n')
'''
PASS_THROUGH = '#!/usr/bin/env bash\nshift 3\nexec "$@"\n'
ADMIN = '#!/usr/bin/env bash\nexport PGPASSWORD=synthetic-admin PGUSER=postgres PGPASSFILE=/dev/null\nexec "$@"\n'


def sessions(name):
    return [dict(options=(' ' + found[1]).split(' -c ')[1:], command=found[2], sql=found[3])
            for found in SESSION.finditer((HOST / name).read_text())]


def option(session, name):
    return [item.strip() for item in session['options'] if item.strip().startswith(name + '=')]


class SuperuserSessionTest(unittest.TestCase):
    def test_superuser_sessions_search_only_the_catalog(self):
        for name in ('provision-activator.sh', 'bootstrap.sh'):
            with self.subTest(name=name):
                self.assertEqual(option(sessions(name)[0], 'search_path'), ['search_path=pg_catalog'])

    def test_catalog_names_are_schema_qualified(self):
        for name in ('provision-activator.sh', 'bootstrap.sh'):
            for session in sessions(name):
                with self.subTest(name=name):
                    self.assertEqual(UNQUALIFIED.findall(session['sql']), [])

    def test_provisioner_superuser_session_stays_in_database_postgres(self):
        admin, *others = sessions('provision-activator.sh')
        self.assertEqual(len(others), 1)
        self.assertIn('postgres-admin.sh psql', admin['command'])
        self.assertIn('--username postgres --dbname postgres', admin['command'])
        self.assertEqual(option(admin, 'password_encryption'), ['password_encryption=scram-sha-256'])
        self.assertEqual(option(admin, 'log_statement'), ['log_statement=none'])
        self.assertEqual(option(admin, 'log_min_error_statement'), ['log_min_error_statement=panic'])
        self.assertIn('ALTER ROLE activator PASSWORD', admin['sql'])
        self.assertIn('ALTER ROLE activator RESET ALL;', admin['sql'])
        self.assertNotIn('GRANT', admin['sql'].replace("'GRANT", ''))
        self.assertNotIn('--dbname auth', (HOST / 'provision-activator.sh').read_text().split('<<')[0])

    def test_provisioner_grants_run_as_the_owner_role_with_limits(self):
        grants = sessions('provision-activator.sh')[1]
        self.assertNotIn('postgres-admin.sh', grants['command'])
        for word in ('timeout --signal=TERM --kill-after=5s 60s psql', '--no-password', '--host 127.0.0.1',
                     '--username auth_owner', '--dbname auth'):
            self.assertIn(word, grants['command'])
        self.assertIn("PGPASSWORD=$auth_password PGPASSFILE=/dev/null PGSERVICEFILE=/dev/null PGCONNECT_TIMEOUT=5 \\\n"
                      "PGOPTIONS='-c search_path=pg_catalog ", (HOST / 'provision-activator.sh').read_text())
        self.assertEqual(option(grants, 'search_path'), ['search_path=pg_catalog'])
        for limit in AUTH_LIMITS:
            self.assertIn(limit, [item.strip() for item in grants['options']])
        for statement in ('GRANT CONNECT ON DATABASE auth TO activator;', 'GRANT USAGE ON SCHEMA public TO activator;',
                          "'GRANT SELECT, INSERT, UPDATE, DELETE ON public.%I TO activator'",
                          "pg_catalog.to_regclass(pg_catalog.format('public.%I', name))",
                          "'GRANT USAGE, SELECT ON SEQUENCE public.release_journal_id_seq TO activator'",
                          "'GRANT SELECT, INSERT, UPDATE ON public.frameworks_release TO activator'"):
            self.assertIn(statement, grants['sql'])
        self.assertNotIn('PASSWORD', grants['sql'])

    def test_provisioner_reads_and_writes_no_relation_outside_the_catalog(self):
        for session in sessions('provision-activator.sh'):
            self.assertEqual(re.findall(r'\b(?:FROM|JOIN|INTO|UPDATE)\s+(?!pg_catalog\.)[a-z_."]+', session['sql']), [])


class ProvisionerRunTest(unittest.TestCase):
    def executable(self, path, text):
        path.write_text(text)
        path.chmod(path.stat().st_mode | stat.S_IXUSR)

    def run_script(self, how):
        with tempfile.TemporaryDirectory() as directory:
            work = Path(directory)
            (work / 'bin').mkdir()
            self.executable(work / 'bin/psql', RECORDER)
            self.executable(work / 'bin/timeout', PASS_THROUGH)
            self.executable(work / 'admin', ADMIN)
            (work / 'activator').write_text('synthetic-activator\n')
            (work / 'auth').write_text('synthetic-auth\n')
            text = (HOST / 'provision-activator.sh').read_text()
            for old, new in (('/usr/local/bin/postgres-admin.sh', work / 'admin'),
                             ('/run/secrets/activator_db_password', work / 'activator'),
                             ('/run/secrets/auth_db_password', work / 'auth')):
                text = text.replace(old, str(new))
            script = work / 'provision.sh'
            script.write_text(text)
            env = dict(os.environ, PATH=str(work / 'bin') + os.pathsep + os.environ['PATH'],
                       SESSIONS=str(work / 'sessions'), PGPASSWORD='synthetic-admin', PGUSER='postgres')
            args, stdin = (['bash', '-s'], text) if how == 'stdin' else (['bash', str(script)], '')
            result = subprocess.run(args, input=stdin, env=env, capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            return [json.loads(line) for line in (work / 'sessions').read_text().splitlines()]

    def test_both_sessions_run_over_stdin_and_from_the_mounted_file(self):
        for how in ('stdin', 'file'):
            with self.subTest(how=how):
                admin, grants = self.run_script(how)
                self.assertEqual(admin['args'][-4:], ['--username', 'postgres', '--dbname', 'postgres'])
                self.assertEqual(admin['env']['PGPASSWORD'], 'synthetic-admin')
                self.assertEqual(admin['env']['ACTIVATOR_DB_PASSWORD'], 'synthetic-activator')
                self.assertIn('ALTER ROLE activator PASSWORD', admin['sql'])
                self.assertEqual(grants['args'][-4:], ['--username', 'auth_owner', '--dbname', 'auth'])
                self.assertEqual(grants['env']['PGPASSWORD'], 'synthetic-auth')
                self.assertEqual(grants['env']['PGPASSFILE'], '/dev/null')
                self.assertEqual(grants['env']['PGSERVICEFILE'], '/dev/null')
                self.assertEqual(grants['env']['PGCONNECT_TIMEOUT'], '5')
                self.assertIsNone(grants['env']['ACTIVATOR_DB_PASSWORD'])
                self.assertIn('GRANT CONNECT ON DATABASE auth TO activator;', grants['sql'])
                self.assertFalse(any('synthetic' in arg for session in (admin, grants) for arg in session['args']))


if __name__ == '__main__':
    unittest.main()
