import json
import re

from . import deploy_corpus_sql as sql
from . import deploy_release_catalog as catalog
from .deploy_corpus_kind import LAW
from .deploy_corpus_lock import AdvisoryLock
from .deploy_corpus_verify import toc
from .deploy_host import activator_arguments


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
        return activator_arguments(*args)

    def user(self, *args, **kwargs):
        self.check_lock()
        return self.host.activator(*args, **kwargs)

    def query(self, text, database='auth', options=sql.SCHEMA_PATH):
        self.check_lock()
        return self.host.activator_psql(database, text, options)

    def restored_query(self, text, database):
        return self.query(text, database, sql.CATALOG_PATH)

    def admin(self, database, statements, options=sql.CATALOG_PATH):
        self.check_lock()
        return self.host.psql(database, statements, options).decode().strip()

    def client(self, program, *args):
        return self.user('env', 'PGOPTIONS=' + sql.CATALOG_PATH, program, '--no-password', '-h', '127.0.0.1',
                         '-U', 'activator', *args)

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
            if self.restored_query(sql.SUMMARY_TABLE, name) == 't':
                if self.restored_query(sql.SUMMARY_PROJECTED, name) == 't':
                    return 1
        return 0

    def activate(self, row):
        return json.loads(self.query(self.kind.activation(self, row)))

    def protected(self, name, operation=None):
        self.sql.database(name)
        return self.query(self.sql.protected(name, operation)) == 't'

    def exists(self, name):
        self.sql.database(name)
        return self.query('SELECT EXISTS(SELECT FROM pg_catalog.pg_database WHERE datname=' + sql.literal(name) + ');') == 't'

    def connections(self, name):
        self.sql.database(name)
        return self.query('SELECT EXISTS(SELECT FROM pg_catalog.pg_stat_activity WHERE datname=' + sql.literal(name) + ');') == 't'

    def owned(self, name):
        self.sql.database(name)
        return self.query(sql.database_owner(name)) == 'activator'

    def drop_owned(self, name, operation=None):
        self.sql.database(name)
        if self.protected(name, operation):
            raise ValueError('refusing to drop protected database')
        self.client('dropdb', '--if-exists', name)

    def drop(self, name):
        self.sql.database(name)
        if self.protected(name):
            raise ValueError('refusing to drop protected database')
        self.host.psql('postgres', "SELECT pg_catalog.format('ALTER DATABASE %I OWNER TO activator',datname) "
            'FROM pg_catalog.pg_database WHERE datname=' + sql.literal(name) +
            " AND pg_catalog.pg_get_userbyid(datdba)='postgres' \\gexec\n", sql.CATALOG_PATH)
        self.drop_owned(name)

    def expired(self):
        return json.loads(self.query(self.sql.expired()))

    def forget(self, name):
        self.sql.database(name)
        if self.protected(name) or self.exists(name):
            raise ValueError('cannot forget protected or existing database')
        self.query(self.sql.forget(name))

    def check_extensions(self, manifest):
        installed = json.loads(self.query('SELECT pg_catalog.json_object_agg(name,default_version) '
            "FROM pg_catalog.pg_available_extensions WHERE name IN ('vector','ltree');", 'postgres', sql.CATALOG_PATH))
        if installed != manifest['extensions']:
            raise ValueError('PostgreSQL extension versions differ from corpus manifest')

    def make(self, row):
        name = row['database_name']
        if self.exists(name):
            if not self.owned(name):
                raise ValueError('activation target not owned by activator')
            self.drop_owned(name, row['operation_id'])
        self.client('createdb', '--template=template0', name)
        if not self.owned(name):
            raise ValueError('activation target not owned by activator')

    def create(self, row):
        name, manifest = row['database_name'], row['manifest']
        sql.database(name)
        self.check_extensions(manifest)
        self.make(row)
        outcome = self.admin(name, sql.prepare(name, manifest['extensions']))
        if outcome == 'superuser':
            raise ValueError(sql.SUPERUSER_REFUSED)
        if outcome != 'configured':
            raise ValueError('corpus database not prepared')

    def inspect(self, row, dump, image=None):
        self.check_extensions(row['manifest'])
        self.toc(row, dump)

    def listing(self, dump):
        return self.user('pg_restore', '--list', input_file=dump, timeout=300)

    def list_path(self, operation):
        if not isinstance(operation, str) or not re.fullmatch('[0-9a-f]{32}', operation):
            raise ValueError('invalid activation operation id')
        return '/run/pgbackrest/corpus-' + operation + '.list'

    def save_list(self, row, selected):
        path = self.list_path(row['operation_id'])
        self.host.user('podman', 'exec', '-i', 'trachuan-postgres', 'sh', '-c',
                       'umask 077; cat > "$1"', 'sh', path, input=selected.encode())
        return path

    def remove_list(self, operation):
        self.host.user('podman', 'exec', 'trachuan-postgres', 'rm', '-f', self.list_path(operation))

    def discard_lists(self):
        self.host.user('podman', 'exec', 'trachuan-postgres', 'sh', '-c', 'rm -f /run/pgbackrest/corpus-*.list')

    def toc(self, row, dump):
        return self.save_list(row, toc(self.listing(dump).decode(), self.rules['extensions']))

    def load(self, name, dump, listing):
        try:
            self.user('env', 'PGOPTIONS=-c maintenance_work_mem=2GB -c max_parallel_maintenance_workers=2 '
                      '-c statement_timeout=0 -c lock_timeout=0 -c idle_in_transaction_session_timeout=0 '
                      '-c transaction_timeout=0 ' + sql.CATALOG_PATH,
                      'pg_restore', '--no-password', '-h', '127.0.0.1', '-U', 'activator', '--dbname', name,
                      '--no-owner', '--no-privileges', '--exit-on-error', '--single-transaction',
                      '--use-list', listing, input_file=dump, timeout=3600)
        finally:
            self.host.user('podman', 'exec', 'trachuan-postgres', 'rm', '-f', listing)

    def check_catalog(self, row):
        outcome = self.restored_query(catalog.law(row['manifest']['extensions']), row['database_name'])
        if outcome != 'ok':
            kind = outcome if outcome in catalog.KINDS else 'object'
            raise ValueError('corpus database ' + kind + ' differs from what a release may hold')

    def restore(self, row, dump, image=None):
        name = row['database_name']
        sql.database(name)
        if self.restored_query(sql.RESTORED, name) != 't':
            self.load(name, dump, self.toc(row, dump))
        self.check_catalog(row)
        self.query(sql.grants(name), name, sql.SCHEMA_PATH)

    def validate(self, row):
        raw = self.restored_query(sql.RELEASE_ROW, row['database_name'])
        expected = {key: row['manifest'][key] for key in ('country', 'rel', 'schema_version', 'recipes')}
        if not raw or json.loads(raw) != expected:
            raise ValueError('restored release row differs from manifest')

    def server(self, image, *args, secret=False, **options):
        command = ['podman', 'run', '--rm', '--network=host', '--read-only', '--cap-drop=ALL',
                   '--security-opt=no-new-privileges', '--env-file', str(self.host.paths.host / 'server.env')]
        if secret:
            command += ['--secret', 'server_corpus_database_url,type=env,target=SERVER_CORPUS_DATABASE_URL']
        return self.host.user(*command, image, *args, timeout=150, **options)

    def describe(self, image, database=None, **options):
        args = ['release-check', *self.kind.check]
        if database is not None:
            self.sql.database(database)
            args += ['--database', database]
        return json.loads(self.server(image, *args, secret=database is not None, **options))

    def smoke(self, row, image):
        self.check_lock()
        name = row['database_name']
        if row['mode'] == 'retained':
            self.check_catalog(row)
        self.restored_query(sql.PREWARM, name)
        self.describe(image, name)
