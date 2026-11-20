import os
from pathlib import Path
import re
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).with_name('provision-activator.sh')
ADMIN = '#!/usr/bin/env bash\nprintf "PGOPTIONS=%s\\n" "${PGOPTIONS-}"\nprintf "ARG=%s\\n" "$@"\ncat\n'
LIMIT = '#!/usr/bin/env bash\nshift 3\nexec "$@"\n'


def sessions():
    found = re.findall(r"PGOPTIONS='([^']*)' \\\n.*? <<'SQL'\n(.*?)\nSQL\n", SCRIPT.read_text(), re.S)
    return [(options, body) for options, body in found]


class ProvisionSearchPathTest(unittest.TestCase):
    def run_script(self, piped):
        with tempfile.TemporaryDirectory() as work:
            root = Path(work)
            (root / 'secret').write_text('synthetic')
            for name, body in (('admin', ADMIN), ('psql', ADMIN), ('timeout', LIMIT)):
                (root / name).write_text(body)
                (root / name).chmod(0o755)
            text = SCRIPT.read_text().replace('/run/secrets/activator_db_password', str(root / 'secret'))
            text = text.replace('/run/secrets/auth_db_password', str(root / 'secret'))
            text = text.replace('/usr/local/bin/postgres-admin.sh', str(root / 'admin'))
            (root / 'script').write_text(text)
            args, stdin = (['bash', '-s'], text) if piped else (['bash', str(root / 'script')], None)
            env = dict(os.environ, PATH=str(root) + os.pathsep + os.environ['PATH'])
            return subprocess.run(args, input=stdin, capture_output=True, text=True, timeout=10, env=env)

    def test_session_pins_the_catalog_as_file_and_piped(self):
        for piped in (False, True):
            with self.subTest(piped=piped):
                result = self.run_script(piped)
                self.assertEqual(result.returncode, 0, result.stderr)
                lines = result.stdout.splitlines()
                self.assertEqual(lines[0].split(' -c ')[-1], 'search_path=pg_catalog')
                self.assertIn('-c log_statement=none', lines[0])
                self.assertIn('-c password_encryption=scram-sha-256', lines[0])
                first = lines[:lines.index('\\getenv activator_password ACTIVATOR_DB_PASSWORD')]
                self.assertIn('ARG=postgres', first)
                self.assertNotIn('ARG=auth', first)
                self.assertIn('ARG=auth_owner', lines)
                self.assertIn('ARG=auth', lines)
                self.assertIn('\\getenv activator_password ACTIVATOR_DB_PASSWORD', lines)
                self.assertIn("WHERE pg_catalog.to_regclass('public.frameworks_release') IS NOT NULL \\gexec", lines)
                self.assertEqual(lines[-1], '\\endif')

    def test_both_sessions_pin_the_catalog_and_scram(self):
        found = sessions()
        self.assertEqual(len(found), 2)
        for options, _ in found:
            self.assertIn('-c search_path=pg_catalog', options.replace('search_path=pg_catalog,', 'refused '))
            self.assertIn('-c password_encryption=scram-sha-256', options)

    def test_every_function_catalog_relation_and_operator_is_qualified(self):
        for _, body in sessions():
            code = re.sub(r":?'(?:[^']|'')*'", "''", body)
            self.assertEqual(re.findall(r'(?<![\w.])([a-z_]+)\(', code), [])
            self.assertEqual(re.findall(r'\b(?:FROM|JOIN)\s+(?!pg_catalog\.)[a-z_."]+', code), [])
            self.assertEqual(re.findall(r'(?<![.(])(?:!~|=)(?!\))', re.sub(r'OPERATOR\(pg_catalog\.[^)]*\)', '', code)), [])

    def test_every_granted_relation_names_its_schema(self):
        body = sessions()[1][1]
        for text in ("'GRANT SELECT, INSERT, UPDATE, DELETE ON public.%I TO activator'",
                     "pg_catalog.to_regclass(pg_catalog.format('public.%I', name))",
                     "'GRANT USAGE, SELECT ON SEQUENCE public.release_journal_id_seq TO activator'",
                     "pg_catalog.to_regclass('public.release_journal_id_seq')",
                     "'GRANT SELECT, INSERT, UPDATE ON public.frameworks_release TO activator'"):
            with self.subTest(text=text):
                self.assertIn(text, body)


if __name__ == '__main__':
    unittest.main()
