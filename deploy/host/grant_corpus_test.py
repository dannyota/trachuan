from pathlib import Path
import subprocess
import unittest


class GrantCorpusTest(unittest.TestCase):
    def grant(self, database):
        script = Path(__file__).with_name('grant-corpus.sh').read_text()
        script = script.replace('/usr/local/bin/postgres-admin.sh', 'postgres_admin')
        fake = 'postgres_admin() { printf "PGOPTIONS=%s\\n" "${PGOPTIONS-}"; printf "%s\\n" "$@"; cat; }\n'
        return subprocess.run(['bash', '-c', fake + script, 'grant-corpus.sh', database],
                              capture_output=True, text=True)

    def test_frameworks_and_invalid_names_never_call_postgres(self):
        for database in ('frameworks_20990101', 'frameworks_20990101_1', 'frameworks_20990101_99',
                         'frameworks_2099010', 'frameworks_209901011', 'frameworks_20990101_',
                         'frameworks_20990101_123', 'frameworks_vn_20990101',
                         'frameworks_20990101;SELECT 1', 'frameworks_20990101\n', 'corpus_vn_20990101\n'):
            with self.subTest(database=database):
                result = self.grant(database)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(result.stdout, '')
                self.assertIn('invalid corpus database name', result.stderr)

    def test_law_names_keep_existing_suffix_and_country_rules(self):
        for database in ('corpus_vn_20990101', 'corpus_vn_20990101_123', 'corpus_us_20990101_1'):
            with self.subTest(database=database):
                self.assertEqual(self.grant(database).returncode, 0)

    def test_law_reader_grants_and_session_log_values(self):
        options = ('-c log_min_messages=panic -c log_min_error_statement=panic '
                   '-c log_min_duration_statement=-1 -c log_statement=none')
        database = 'corpus_vn_20990101_1'
        result = self.grant(database)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('GRANT SELECT ON ALL TABLES IN SCHEMA public TO corpus_reader;', result.stdout)
        self.assertIn('PGOPTIONS=' + options + '\npsql\n-X\n-q\n--set\nON_ERROR_STOP=1\n'
                      '--username\npostgres\n--dbname\n' + database + '\n', result.stdout)
