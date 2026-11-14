import os
from pathlib import Path
import subprocess
import tempfile
import unittest

HOST = Path(__file__).resolve().parent


class SecretsTest(unittest.TestCase):
    def run_wrapper(self, missing=None, empty=None, repository_only=False):
        self.assertTrue((HOST / 'pgbackrest-entrypoint.sh').exists(), 'secret wrapper is missing')
        work = Path(self.work.name)
        secrets = work / 'secrets'
        secrets.mkdir()
        values = {'backup_s3_key': 'synthetic-key', 'backup_s3_key_secret': 'synthetic-secret',
                  'postgres_password': 'synthetic:pass\\word'}
        for name, value in values.items():
            if name != missing:
                (secrets / name).write_text('' if name == empty else value)
        upstream = work / 'docker-entrypoint.sh'
        upstream.write_text('#!/bin/bash\nprintf "%s\\n" "$@"\nenv\n')
        upstream.chmod(0o755)
        env = dict(os.environ, PATH=str(work)+os.pathsep+os.environ['PATH'],
                   CREDENTIALS_DIRECTORY=str(secrets),
                   PGBACKREST_CONFIG_INCLUDE_PATH=str(work / 'runtime/conf.d'),
                   PGPASSFILE=str(work / 'runtime/pgpass'))
        args = ['--repository-only'] if repository_only else []
        result = subprocess.run(['bash', str(HOST / 'pgbackrest-entrypoint.sh'), *args, 'postgres', '-c', 'port=5432'],
                                env=env, capture_output=True, text=True)
        for value in values.values():
            self.assertNotIn(value, result.stdout + result.stderr)
        return result

    def setUp(self):
        self.work = tempfile.TemporaryDirectory(prefix='trachuan-secret-test-')
        self.addCleanup(self.work.cleanup)

    def test_runtime_files_private_and_original_arguments_preserved(self):
        result = self.run_wrapper()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(result.stdout.startswith('postgres\n-c\nport=5432\n'))
        root = Path(self.work.name) / 'runtime'
        self.assertEqual((root / 'conf.d/secret.conf').read_text(),
                         '[global]\nrepo1-s3-key=synthetic-key\nrepo1-s3-key-secret=synthetic-secret\n')
        self.assertEqual((root / 'pgpass').read_text(), '*:*:*:postgres:synthetic\\:pass\\\\word\n')
        for file in [root / 'conf.d/secret.conf', root / 'pgpass']:
            self.assertEqual(file.stat().st_mode & 0o777, 0o600)
            self.assertEqual(file.stat().st_uid, os.getuid())

    def test_missing_or_empty_each_secret_fails_closed(self):
        for name in ['backup_s3_key', 'backup_s3_key_secret', 'postgres_password']:
            for mode in ['missing', 'empty']:
                with self.subTest(name=name, mode=mode), tempfile.TemporaryDirectory() as work:
                    previous = self.work
                    self.work = type('Work', (), {'name': work})()
                    result = self.run_wrapper(**{mode: name})
                    self.work = previous
                    self.assertNotEqual(result.returncode, 0)
                    self.assertNotIn('port=5432', result.stdout)
                    self.assertFalse((Path(work) / 'runtime/conf.d/secret.conf').exists())

    def test_restore_only_needs_repository_secrets(self):
        result = self.run_wrapper(missing='postgres_password', repository_only=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse((Path(self.work.name) / 'runtime/pgpass').exists())


if __name__ == '__main__':
    unittest.main()
