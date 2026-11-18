import json
import re

from . import deploy_corpus_sql as sql
from . import deploy_release_catalog as catalog
from . import deploy_release_target as target
from .deploy_corpus_kind import LAW
from .deploy_corpus_lock import AdvisoryLock
from .deploy_corpus_verify import toc
from .deploy_host import activator_arguments, activator_client
from .deploy_io import CommandError, interrupted
from .deploy_release_tables import Tables


CATALOG = ("SELECT coalesce(pg_catalog.json_object_agg(d.datname::pg_catalog.text,pg_catalog.json_build_object("
           "'oid',d.oid::pg_catalog.int8,'owner',pg_catalog.pg_get_userbyid(d.datdba)::pg_catalog.text)),'{}') "
           'FROM pg_catalog.pg_database d;')


class NoRecords:
    def names(self, catalog, own=None, pruning=()):
        return set()


class Database:
    def __init__(self, host, rules, kind=LAW):
        self.host, self.rules, self.holder, self.backend = host, rules, None, None
        self.kind, self.sql = kind, kind.sql
        self.tables = Tables(host, kind)
        self.guard = NoRecords()
        self.remember = lambda operation, oid: None

    def lock(self):
        return AdvisoryLock(self)

    def check_lock(self):
        if self.holder is None or self.holder.poll() is not None:
            raise target.LockLost('activation advisory lock lost')

    def arguments(self, *args):
        return activator_arguments(*args)

    def target(self, name, *shared):
        if name not in shared:
            self.sql.database(name)
        return name

    def session(self, database, options=sql.CATALOG_PATH):
        return activator_client(self.target(database, 'auth', 'postgres'), options)

    def user(self, *args, **kwargs):
        self.check_lock()
        return self.host.activator(*args, **kwargs)

    def query(self, text, database='postgres', options=sql.CATALOG_PATH):
        self.check_lock()
        name = self.target(database, 'postgres')
        try:
            return self.host.activator_psql(name, text, options)
        except CommandError as error:
            if name == 'postgres' and error.returncode == 2 and not interrupted(error):
                raise target.Unreachable('activator session unavailable') from error
            raise

    def admin(self, database, statements, options=sql.CATALOG_PATH):
        self.check_lock()
        return self.host.psql(self.target(database), statements, options).decode().strip()

    def catalog(self):
        value = json.loads(self.query(CATALOG))
        if not isinstance(value, dict):
            raise ValueError('invalid database catalog')
        return {name: dict(oid=int(item['oid']), owner=item['owner']) for name, item in value.items()}

    def remember_target(self, row):
        name = self.target(row['database_name'])
        oid = target.number(self.query(target.target_oid(name)))
        row['oid'] = int(oid)
        self.remember(row['operation_id'], row['oid'])
        return row['oid']

    def target_oid(self, row):
        self.target(row['database_name'])
        oid = row.get('oid')
        if oid is None:
            raise ValueError(target.REFUSED)
        return str(oid)

    def guarded(self, row, statements):
        return target.guard(self.target_oid(row), statements)

    def identified(self, output):
        if output == target.MISMATCH:
            raise ValueError(target.REFUSED)
        return output

    def on_target(self, row, statements, options=sql.CATALOG_PATH):
        return self.identified(self.query(self.guarded(row, statements), row['database_name'], options))

    def target_admin(self, row, statements, options=sql.CATALOG_PATH):
        return self.identified(self.admin(row['database_name'], self.guarded(row, statements), options))

    def verify_target(self, row):
        self.on_target(row, '')

    def projection(self, row):
        if row['manifest']['schema_version'] >= 8:
            if self.on_target(row, sql.SUMMARY_TABLE) == 't':
                if self.on_target(row, sql.SUMMARY_PROJECTED) == 't':
                    return 1
        return 0

    def exists(self, name):
        self.sql.database(name)
        return self.query('SELECT EXISTS(SELECT FROM pg_catalog.pg_database WHERE datname=' + sql.literal(name) + ');') == 't'

    def connections(self, name):
        self.sql.database(name)
        return self.query('SELECT EXISTS(SELECT FROM pg_catalog.pg_stat_activity WHERE datname=' + sql.literal(name) + ');') == 't'

    def owned(self, name):
        self.sql.database(name)
        return self.query(sql.database_owner(name)) == 'activator'

    def protected_names(self, operation=None, pruning=()):
        names = self.tables.protected(operation)
        return names | self.guard.names(self.catalog(), own=operation, pruning=pruning)

    def droppable(self, name, operation=None, pruning=()):
        self.sql.database(name)
        return name not in self.protected_names(operation, pruning) and not self.connections(name)

    def drop_database(self, name, oid=None):
        self.query(target.drop(self.target(name), oid))
        if self.exists(name):
            raise target.Waiting(target.DATABASE_LEFT)

    def drop_target(self, row):
        name = self.target(row['database_name'])
        if name in self.protected_names(row['operation_id']):
            raise target.Waiting('activation target is protected')
        if self.connections(name):
            raise target.Waiting('activation target has connections')
        self.drop_database(name, row.get('oid'))

    def other_superuser(self):
        return self.query(sql.other_superuser()) == 't'

    def clear_leftovers(self):
        if self.backend is None:
            raise target.LockLost('activation advisory lock lost')
        lines = self.query(target.end_backends(self.backend)).splitlines()
        if not lines or lines[-1] != 'f':
            raise target.Leftover(target.BACKEND_LEFT)
        if self.query(target.strays(self.protected_names())) != 'f':
            raise target.Leftover(target.DATABASE_LEFT)

    def tidy(self):
        try:
            self.clear_leftovers()
        except target.Waiting as error:
            self.host.note('leftovers', 'leftover sweep deferred: ' + str(error))
        except (CommandError, ValueError) as error:
            if interrupted(error):
                raise
            self.host.note('leftovers', 'leftover sweep deferred: command failed')

    def check_extensions(self, manifest):
        installed = json.loads(self.query('SELECT pg_catalog.json_object_agg(name,default_version) '
            "FROM pg_catalog.pg_available_extensions WHERE name IN ('vector','ltree');", 'postgres', sql.CATALOG_PATH))
        if installed != manifest['extensions']:
            raise ValueError('PostgreSQL extension versions differ from corpus manifest')

    def make(self, row):
        name = self.target(row['database_name'])
        exists = self.exists(name)
        if exists and not self.owned(name):
            raise ValueError('activation target not owned by activator')
        self.clear_leftovers()
        if exists:
            self.drop_target(row)
        self.query('CREATE DATABASE ' + self.sql.database(name) + ' TEMPLATE template0;')
        if not self.owned(name):
            raise ValueError('activation target not owned by activator')
        self.remember_target(row)

    def create(self, row):
        name, manifest = row['database_name'], row['manifest']
        sql.database(name)
        statements = sql.prepare(name, manifest['extensions'])
        self.check_extensions(manifest)
        self.make(row)
        outcome = self.target_admin(row, statements)
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
        name = self.target(row['database_name'])
        self.verify_target(row)
        try:
            self.user('env', 'PGOPTIONS=-c maintenance_work_mem=2GB -c max_parallel_maintenance_workers=2 '
                      '-c statement_timeout=0 -c lock_timeout=0 -c idle_in_transaction_session_timeout=0 '
                      '-c transaction_timeout=0 ' + sql.CATALOG_PATH,
                      'pg_restore', '--no-password', '-h', '127.0.0.1', '-U', 'activator',
                      '--dbname', name, '--no-owner', '--no-privileges', '--exit-on-error',
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
        self.target(row['database_name'])
        self.check_lock()
        self.check_catalog(row, True)
        self.on_target(row, sql.PREWARM)
        self.verify_target(row)
        self.describe(image, row['database_name'])
        self.verify_target(row)
