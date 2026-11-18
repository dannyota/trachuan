from pathlib import Path
import re
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).with_name('provision-activator.sh')
ADMIN = '#!/usr/bin/env bash\nprintf "PGOPTIONS=%s\\n" "${PGOPTIONS-}"\nprintf "ARG=%s\\n" "$@"\ncat\n'


def session():
    text = SCRIPT.read_text()
    return re.search(r"PGOPTIONS='([^']*)'", text)[1], text.split("<<'SQL'\n", 1)[1].split('\nSQL\n', 1)[0]


class ProvisionSearchPathTest(unittest.TestCase):
    def run_script(self, piped):
        with tempfile.TemporaryDirectory() as work:
            root = Path(work)
            (root / 'secret').write_text('synthetic')
            (root / 'admin').write_text(ADMIN)
            (root / 'admin').chmod(0o755)
            text = SCRIPT.read_text().replace('/run/secrets/activator_db_password', str(root / 'secret'))
            text = text.replace('/usr/local/bin/postgres-admin.sh', str(root / 'admin'))
            (root / 'script').write_text(text)
            args, stdin = (['bash', '-s'], text) if piped else (['bash', str(root / 'script')], None)
            return subprocess.run(args, input=stdin, capture_output=True, text=True, timeout=10)

    def test_session_pins_the_catalog_as_file_and_piped(self):
        for piped in (False, True):
            with self.subTest(piped=piped):
                result = self.run_script(piped)
                self.assertEqual(result.returncode, 0, result.stderr)
                lines = result.stdout.splitlines()
                self.assertEqual(lines[0].split(' -c ')[-1], 'search_path=pg_catalog')
                self.assertIn('-c log_statement=none', lines[0])
                self.assertIn('-c password_encryption=scram-sha-256', lines[0])
                self.assertIn('ARG=auth', lines)
                self.assertIn('\\getenv activator_password ACTIVATOR_DB_PASSWORD', lines)
                self.assertEqual(lines[-1], "WHERE pg_catalog.to_regclass('public.frameworks_release') IS NOT NULL \\gexec")

    def test_every_function_and_catalog_relation_is_qualified(self):
        code = re.sub(r":?'(?:[^']|'')*'", "''", session()[1])
        self.assertEqual(re.findall(r'(?<![\w.])([a-z_]+)\(', code), [])
        self.assertEqual(re.findall(r'\b(?:FROM|JOIN)\s+(?!pg_catalog\.)[a-z_."]+', code), [])

    def test_every_granted_relation_names_its_schema(self):
        body = session()[1]
        for text in ("'GRANT SELECT, INSERT, UPDATE, DELETE ON public.%I TO activator'",
                     "pg_catalog.to_regclass(pg_catalog.format('public.%I', name))",
                     "'GRANT USAGE, SELECT ON SEQUENCE public.release_journal_id_seq TO activator'",
                     "pg_catalog.to_regclass('public.release_journal_id_seq')",
                     "'GRANT SELECT, INSERT, UPDATE ON public.frameworks_release TO activator'"):
            with self.subTest(text=text):
                self.assertIn(text, body)


if __name__ == '__main__':
    unittest.main()
