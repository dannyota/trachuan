import unittest

from updater import activation_test as fixture
from updater.deploy_corpus_sql import AUTH_ACTIVE_DATABASE, AUTH_SCHEMA_VERSION
from updater.deploy_host import auth_arguments, AUTH_OUTPUT_LIMIT
from updater.deploy_io import CommandError


SCRIPT = ('set +x; set -eu; export PGPASSWORD; PGPASSWORD=$(cat /run/secrets/auth_db_password); '
          '[ -n "$PGPASSWORD" ]; exec "$@"')


class AuthReadTest(fixture.ActivationFixture, unittest.TestCase):
    def setUp(self):
        super().setUp()
        self.limits, self.answers = [], {}
        self.host.runner = self.auth_runner

    def auth_runner(self, args, input=None, **kwargs):
        text = fixture.sql_text(args, input)
        for marker, answer in self.answers.items():
            if marker in text:
                self.calls.append((args, input))
                self.limits.append(kwargs.get('stdout_limit'))
                if isinstance(answer, Exception):
                    raise answer
                return answer
        return self.runner(args, input=input, **kwargs)

    def expected(self, sql):
        return ['podman', 'exec', '-i', 'trachuan-postgres', 'sh', '-c', SCRIPT, 'sh',
                'env', 'PGOPTIONS=-c search_path=pg_catalog,public', 'psql', '--no-password', '-XqAt',
                '-h', '127.0.0.1', '-U', 'auth_owner', '-v', 'ON_ERROR_STOP=1', '-d', 'auth', '-c', sql]

    def test_arguments_name_the_owner_role_and_keep_the_password_in_the_container(self):
        self.assertEqual(auth_arguments('SELECT 1;'), self.expected('SELECT 1;'))
        self.assertFalse(any(arg.startswith('PGPASSWORD=') for arg in auth_arguments('SELECT 1;')))

    def test_queries_name_their_schema(self):
        self.assertEqual(AUTH_SCHEMA_VERSION,
                         'SELECT coalesce(pg_catalog.max(version_id),0) FROM public.goose_db_version;')
        self.assertEqual(AUTH_ACTIVE_DATABASE, "SELECT active_db FROM public.corpus_release WHERE country = 'vn';")

    def test_schema_reads_as_the_owner_role_with_bounded_output(self):
        self.answers[b'max(version_id)'] = b'41\n'
        self.assertEqual(self.host.schema(), 41)
        self.assertEqual(self.calls[-1], (self.expected(AUTH_SCHEMA_VERSION), None))
        self.assertEqual(self.limits, [AUTH_OUTPUT_LIMIT])

    def test_schema_refuses_anything_but_an_integer(self):
        for value in (b'', b'-1\n', b'7 8\n', b'7\n8\n', b'seven\n', b'\xd9\xa1\n', b'1' * 20 + b'\n'):
            with self.subTest(value=value):
                self.answers[b'max(version_id)'] = value
                with self.assertRaisesRegex(ValueError, 'invalid auth schema version'):
                    self.host.schema()

    def test_schema_keeps_the_largest_goose_version(self):
        self.answers[b'max(version_id)'] = b'9223372036854775807\n'
        self.assertEqual(self.host.schema(), 9223372036854775807)

    def test_a_failed_read_is_not_retried_as_another_role(self):
        self.answers[b'max(version_id)'] = CommandError('command failed', detail='psql: exit 2')
        with self.assertRaises(CommandError):
            self.host.schema()
        self.assertEqual(len(self.limits), 1)
        self.assertFalse(any('postgres-admin.sh' in ' '.join(args) for args, _ in self.calls))

    def test_active_database_reads_as_the_owner_role(self):
        self.answers[b'active_db'] = b'corpus_vn_20990101_1\n'
        self.host.postgres()
        self.assertIn((self.expected(AUTH_ACTIVE_DATABASE), None), self.calls)
        self.assertEqual(self.limits, [AUTH_OUTPUT_LIMIT])

    def test_active_database_refuses_names_outside_the_corpus_shape(self):
        for value in (b'', b'auth\n', b'corpus_vn_20990101\ncorpus_vn_20990102\n', b'corpus_vn_20990101;drop\n'):
            with self.subTest(value=value):
                self.answers[b'active_db'] = value
                with self.assertRaisesRegex(ValueError, 'invalid corpus database name'):
                    self.host.postgres()

    def test_superuser_helper_refuses_auth_and_defaults_to_the_catalog_path(self):
        self.calls.clear()
        with self.assertRaisesRegex(ValueError, 'superuser session refused on auth'):
            self.host.psql('auth', 'SELECT 1;')
        self.assertEqual(self.calls, [])
        self.host.psql('postgres', 'SELECT 1;')
        self.assertIn('PGOPTIONS=-c search_path=pg_catalog', self.calls[-1][0])


if __name__ == '__main__':
    unittest.main()
