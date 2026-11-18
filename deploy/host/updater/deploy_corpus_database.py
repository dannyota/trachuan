import json

from . import deploy_corpus_sql as sql
from .deploy_corpus_kind import LAW
from .deploy_corpus_lock import AdvisoryLock
from .deploy_corpus_verify import toc


class Database:
    def __init__(self, host, rules, kind=LAW):
        self.host, self.rules, self.holder = host, rules, None
        self.kind, self.sql = kind, kind.sql

    def lock(self):
        return AdvisoryLock(self)

    def check_lock(self):
        if self.holder is None or self.holder.poll() is not None:
            raise ValueError('activation advisory lock lost')

    def arguments(self, *args):
        return ['podman', 'exec', '-i', 'trachuan-postgres', 'sh', '-c',
                'set +x; set -eu; export PGPASSWORD; PGPASSWORD=$(cat /run/secrets/activator_db_password); '
                '[ -n "$PGPASSWORD" ]; exec "$@"', 'sh', *args]

    def user(self, *args, **kwargs):
        self.check_lock()
        return self.host.user(*self.arguments(*args), **kwargs)

    def query(self, text, database='auth'):
        return self.user('psql', '--no-password', '-XqAt', '-h', '127.0.0.1', '-U', 'activator',
                         '-v', 'ON_ERROR_STOP=1', '-d', database,
                         input=('SET standard_conforming_strings=on;\n' + text).encode()).decode().strip()

    def current(self, country):
        value = self.query(self.sql.current(country))
        if value:
            self.sql.database(value)
        return value or None

    def retirement(self, name):
        self.sql.database(name)
        raw = self.query(self.sql.retirement(name))
        return json.loads(raw) if raw else None

    def operations(self):
        return json.loads(self.query(self.sql.operations()))

    def append(self, row, step):
        return json.loads(self.query(self.sql.journal(row, step)))

    def projection(self, row):
        if row['manifest']['schema_version'] >= 8:
            name = row['database_name']
            if self.query("SELECT to_regclass('public.document_summary') IS NOT NULL;", name) == 't':
                if self.query('SELECT NOT EXISTS(SELECT FROM document_summary WHERE projection <> 1);', name) == 't':
                    return 1
        return 0

    def activate(self, row):
        return json.loads(self.query(self.kind.activation(self, row)))

    def protected(self, name):
        self.sql.database(name)
        return self.query(self.sql.protected(name)) == 't'

    def exists(self, name):
        self.sql.database(name)
        return self.query('SELECT EXISTS(SELECT FROM pg_database WHERE datname=' + sql.literal(name) + ');') == 't'

    def connections(self, name):
        self.sql.database(name)
        return self.query('SELECT EXISTS(SELECT FROM pg_stat_activity WHERE datname=' + sql.literal(name) + ');') == 't'

    def drop(self, name):
        self.sql.database(name)
        if self.protected(name):
            raise ValueError('refusing to drop protected database')
        self.host.psql('postgres', "SELECT format('ALTER DATABASE %I OWNER TO activator',datname) "
            'FROM pg_database WHERE datname=' + sql.literal(name) +
            " AND pg_get_userbyid(datdba)='postgres' \\gexec\n")
        if self.protected(name):
            raise ValueError('refusing to drop protected database')
        self.user('dropdb', '--no-password', '-h', '127.0.0.1', '-U', 'activator', '--if-exists', name)

    def expired(self):
        return json.loads(self.query(self.sql.expired()))

    def forget(self, name):
        self.sql.database(name)
        if self.protected(name) or self.exists(name):
            raise ValueError('cannot forget protected or existing database')
        self.query(self.sql.forget(name))

    def check_extensions(self, manifest):
        installed = json.loads(self.query("SELECT json_object_agg(name,default_version) "
            "FROM pg_available_extensions WHERE name IN ('vector','ltree');", 'postgres'))
        if installed != manifest['extensions']:
            raise ValueError('PostgreSQL extension versions differ from corpus manifest')

    def create(self, row):
        name, manifest = row['database_name'], row['manifest']
        sql.database(name)
        self.check_extensions(manifest)
        if not self.exists(name):
            self.user('createdb', '--no-password', '-h', '127.0.0.1', '-U', 'activator', '--template=template0', name)
        owner = self.query('SELECT pg_get_userbyid(datdba) FROM pg_database WHERE datname=' + sql.literal(name) + ';')
        if owner != 'activator':
            raise ValueError('activation target not owned by activator')
        self.host.psql(name, ';'.join('CREATE EXTENSION IF NOT EXISTS ' + ext + ' VERSION ' + sql.literal(version)
                                      for ext, version in manifest['extensions'].items()) + ';')

    def inspect(self, row, dump):
        self.check_extensions(row['manifest'])
        self.toc(row, dump)

    def toc(self, row, dump):
        raw = self.user('pg_restore', '--list', input_file=dump, timeout=300)
        selected = toc(raw.decode(), self.rules['extensions'])
        path = '/run/pgbackrest/corpus-' + row['operation_id'] + '.list'
        self.host.user('podman', 'exec', '-i', 'trachuan-postgres', 'sh', '-c',
                       'umask 077; cat > "$1"', 'sh', path, input=selected.encode())
        return path

    def restore(self, row, dump):
        name = row['database_name']
        sql.database(name)
        restored = self.query("SELECT to_regclass('public.release') IS NOT NULL;", name) == 't'
        if not restored:
            listing = self.toc(row, dump)
            try:
                self.user('env', 'PGOPTIONS=-c maintenance_work_mem=2GB -c max_parallel_maintenance_workers=2 '
                          '-c statement_timeout=0 -c lock_timeout=0 -c idle_in_transaction_session_timeout=0 '
                          '-c transaction_timeout=0',
                          'pg_restore', '--no-password', '-h', '127.0.0.1', '-U', 'activator', '--dbname', name,
                          '--no-owner', '--no-privileges', '--exit-on-error', '--single-transaction',
                          '--use-list', listing, input_file=dump, timeout=3600)
            finally:
                self.host.user('podman', 'exec', 'trachuan-postgres', 'rm', '-f', listing)
        self.host.user('podman', 'exec', 'trachuan-postgres', 'bash', '/usr/local/bin/grant-corpus.sh', name)

    def validate(self, row):
        raw = self.query("SELECT json_build_object('country',country,'rel',rel,'schema_version',schema_version,"
            "'recipes',json_build_object('dense',dense_recipe,'chunk',chunk_recipe,"
            "'lexical',lexical_recipe,'retrieval',retrieval_recipe)) FROM release WHERE singleton;",
            row['database_name'])
        expected = {key: row['manifest'][key] for key in ('country', 'rel', 'schema_version', 'recipes')}
        if not raw or json.loads(raw) != expected:
            raise ValueError('restored release row differs from manifest')

    def describe(self, image, database=None):
        args = ['podman', 'run', '--rm', '--network=host', '--read-only', '--cap-drop=ALL',
                '--security-opt=no-new-privileges', '--env-file', str(self.host.paths.host / 'server.env')]
        if database is not None:
            sql.database(database)
            args += ['--secret', 'server_corpus_database_url,type=env,target=SERVER_CORPUS_DATABASE_URL']
        args += [image, 'release-check', *self.kind.check]
        if database is not None:
            args += ['--database', database]
        return json.loads(self.host.user(*args, timeout=150))

    def smoke(self, row, image):
        self.check_lock()
        self.host.psql(row['database_name'], 'SET statement_timeout=0; SET lock_timeout=0; SET idle_in_transaction_session_timeout=0; '
            'SET transaction_timeout=0; BEGIN; CREATE EXTENSION IF NOT EXISTS pg_prewarm; '
            "SELECT sum(pg_prewarm(c.oid)) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace "
            "WHERE n.nspname IN ('public','pg_toast') AND c.relkind IN ('r','i','t'); "
            'DROP EXTENSION pg_prewarm; COMMIT;')
        self.describe(image, row['database_name'])
