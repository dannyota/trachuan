from pathlib import Path
import subprocess
import unittest


class GrantCorpusTest(unittest.TestCase):
    def grant(self, database):
        script = Path(__file__).with_name('grant-corpus.sh').read_text()
        script = script.replace('/usr/local/bin/postgres-admin.sh', 'postgres_admin')
        fake = 'postgres_admin() { printf "%s\\n" "$@"; cat; }\n'
        return subprocess.run(['bash', '-c', fake + script, 'grant-corpus.sh', database],
                              capture_output=True, text=True)

    def test_frameworks_receives_same_reader_grants_as_law(self):
        law = self.grant('corpus_vn_20990101_1')
        self.assertEqual(law.returncode, 0, law.stderr)
        for database in ('frameworks_20990101', 'frameworks_20990101_1', 'frameworks_20990101_99'):
            with self.subTest(database=database):
                result = self.grant(database)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(result.stdout.replace(database, 'corpus_vn_20990101_1'), law.stdout)
                self.assertIn('GRANT SELECT ON ALL TABLES IN SCHEMA public TO corpus_reader;', result.stdout)

    def test_invalid_frameworks_names_never_call_postgres(self):
        for database in ('frameworks_2099010', 'frameworks_209901011', 'frameworks_20990101_',
                         'frameworks_20990101_123', 'frameworks_vn_20990101',
                         'frameworks_20990101;SELECT 1', 'frameworks_20990101\n'):
            with self.subTest(database=database):
                result = self.grant(database)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(result.stdout, '')
                self.assertIn('invalid corpus database name', result.stderr)

    def test_law_names_keep_existing_suffix_and_country_rules(self):
        for database in ('corpus_vn_20990101', 'corpus_vn_20990101_123', 'corpus_us_20990101_1'):
            with self.subTest(database=database):
                self.assertEqual(self.grant(database).returncode, 0)
