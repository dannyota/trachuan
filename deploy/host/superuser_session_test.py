from pathlib import Path
import re
import unittest

HOST = Path(__file__).resolve().parent
UNQUALIFIED = re.compile(r'(?<![\w.])(format|to_regclass|unnest|pg_roles|pg_database|pg_db_role_setting)\b')


def session(name):
    text = (HOST / name).read_text()
    options = re.search(r"PGOPTIONS='([^']*)'", text)[1]
    body = text.split("<<'SQL'\n", 1)[1].split('\nSQL\n', 1)[0]
    return options, body


class SuperuserSessionTest(unittest.TestCase):
    def test_sessions_that_reach_auth_search_only_the_catalog(self):
        for name in ('provision-activator.sh', 'bootstrap.sh'):
            with self.subTest(name=name):
                self.assertEqual(session(name)[0].split(' -c ')[-1], 'search_path=pg_catalog')

    def test_catalog_names_are_schema_qualified(self):
        for name in ('provision-activator.sh', 'bootstrap.sh'):
            with self.subTest(name=name):
                self.assertEqual(UNQUALIFIED.findall(session(name)[1]), [])

    def test_provisioner_names_the_schema_of_every_granted_relation(self):
        body = session('provision-activator.sh')[1]
        self.assertIn("'GRANT SELECT, INSERT, UPDATE, DELETE ON public.%I TO activator'", body)
        self.assertIn("pg_catalog.to_regclass(pg_catalog.format('public.%I', name))", body)
        self.assertIn("'GRANT USAGE, SELECT ON SEQUENCE public.release_journal_id_seq TO activator'", body)
        self.assertIn("'GRANT SELECT, INSERT, UPDATE ON public.frameworks_release TO activator'", body)

    def test_provisioner_reads_and_writes_no_relation_outside_the_catalog(self):
        body = session('provision-activator.sh')[1]
        self.assertEqual(re.findall(r'\b(?:FROM|JOIN|INTO|UPDATE)\s+(?!pg_catalog\.)[a-z_."]+', body), [])


if __name__ == '__main__':
    unittest.main()
