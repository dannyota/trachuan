import json
import re

from . import deploy_corpus_sql as sql
from . import deploy_release_catalog as catalog
from . import deploy_release_target as target
from .deploy_corpus_kind import LAW
from .deploy_corpus_lock import AdvisoryLock
from .deploy_corpus_verify import toc
from .deploy_host import activator_arguments
from .deploy_io import CommandError, interrupted


class Database:
    def __init__(self, host, rules, kind=LAW):
        self.host, self.rules, self.holder, self.backend = host, rules, None, None
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

    def admin(self, database, statements, options=sql.CATALOG_PATH):
        self.check_lock()
        return self.host.psql(database, statements, options).decode().strip()

    def targets(self):
        return target.Targets(self.host.paths.state / (self.kind.key + '-targets'))

    def forget_targets(self, operations):
        self.targets().keep(operations)

    def remember_target(self, row):
        name = row['database_name']
        oid = self.query(target.target_oid(name))
        self.targets().write(row['operation_id'], name, oid)
        return oid

    def target_oid(self, row):
        if row['mode'] != 'new':
            return None
        stored = self.targets().read(row['operation_id'], row['database_name'])
        if stored:
            return stored
        self.clear_leftovers()
        return self.remember_target(row)

    def guarded(self, row, statements):
        oid = self.target_oid(row)
        return statements if oid is None else target.guard(oid, statements)

    def identified(self, output):
        if output == target.MISMATCH:
            raise ValueError(target.REFUSED)
        return output

    def on_target(self, row, statements, options=sql.CATALOG_PATH):
        return self.identified(self.query(self.guarded(row, statements), row['database_name'], options))

    def target_admin(self, row, statements, options=sql.CATALOG_PATH):
        return self.identified(self.admin(row['database_name'], self.guarded(row, statements), options))

    def verify_target(self, row):
        if self.target_oid(row) is not None:
            self.on_target(row, '')

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
            if self.on_target(row, sql.SUMMARY_TABLE) == 't':
                if self.on_target(row, sql.SUMMARY_PROJECTED) == 't':
                    return 1
        return 0

    def activate(self, row):
        self.verify_target(row)
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

    def other_superuser(self):
        return self.query(sql.other_superuser()) == 't'

    def clear_leftovers(self):
        if self.backend is None:
            raise ValueError('activation advisory lock lost')
        lines = self.query(target.end_backends(self.backend)).splitlines()
        if not lines or lines[-1] != 'f':
            raise target.Leftover(target.BACKEND_LEFT)
        if self.query(target.STRAYS) != 'f':
            raise target.Leftover(target.DATABASE_LEFT)

    def tidy(self):
        try:
            self.clear_leftovers()
        except target.Leftover as error:
            self.host.note('leftovers', 'leftover sweep deferred: ' + str(error))
        except (CommandError, ValueError) as error:
            if interrupted(error):
                raise
            self.host.note('leftovers', 'leftover sweep deferred: command failed')

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
        exists = self.exists(name)
        if exists and not self.owned(name):
            raise ValueError('activation target not owned by activator')
        self.clear_leftovers()
        if exists:
            self.drop_owned(name, row['operation_id'])
        self.client('createdb', '--template=template0', name)
        if not self.owned(name):
            raise ValueError('activation target not owned by activator')
        self.remember_target(row)

    def create(self, row):
        name, manifest = row['database_name'], row['manifest']
        sql.database(name)
        self.check_extensions(manifest)
        self.make(row)
        outcome = self.target_admin(row, sql.prepare(name, manifest['extensions']))
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

    def load(self, row, dump, listing):
        self.verify_target(row)
        try:
            self.user('env', 'PGOPTIONS=-c maintenance_work_mem=2GB -c max_parallel_maintenance_workers=2 '
                      '-c statement_timeout=0 -c lock_timeout=0 -c idle_in_transaction_session_timeout=0 '
                      '-c transaction_timeout=0 ' + sql.CATALOG_PATH,
                      'pg_restore', '--no-password', '-h', '127.0.0.1', '-U', 'activator',
                      '--dbname', row['database_name'], '--no-owner', '--no-privileges', '--exit-on-error',
                      '--single-transaction', '--use-list', listing, input_file=dump, timeout=3600)
        finally:
            self.host.user('podman', 'exec', 'trachuan-postgres', 'rm', '-f', listing)
        self.verify_target(row)

    def check_catalog(self, row, granted):
        outcome = self.on_target(row, catalog.law(row['manifest']['extensions'], granted=granted,
                                                  retained=row['mode'] == 'retained'))
        if outcome != 'ok':
            kind = outcome if outcome in catalog.KINDS else 'object'
            raise ValueError('corpus database ' + kind + ' differs from what a release may hold')

    def restore(self, row, dump, image=None):
        name = row['database_name']
        sql.database(name)
        fresh = self.on_target(row, sql.RESTORED) != 't'
        if fresh:
            self.load(row, dump, self.toc(row, dump))
        self.check_catalog(row, not fresh)
        self.on_target(row, sql.grants(name), sql.SCHEMA_PATH)

    def validate(self, row):
        raw = self.on_target(row, sql.RELEASE_ROW)
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
        self.check_catalog(row, True)
        self.on_target(row, sql.PREWARM)
        self.verify_target(row)
        self.describe(image, row['database_name'])
        self.verify_target(row)
