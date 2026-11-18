import re

from . import deploy_frameworks_check as check
from . import deploy_frameworks_sql as sql
from .deploy_frameworks_schema import objects, toc
from .deploy_host import SCRAM_ONLY
from .deploy_io import decode
from .deploy_verify import positive


STEPS = ('configuration', 'create reader pool', 'connect reader', 'log settings', 'release row',
         'release identity or schema', 'create smoke key', 'open engine', 'engine release identity', 'search',
         'search hits', 'read', 'read result')
SAFE = re.compile(r'server: (frameworks release-check (?:' + '|'.join(STEPS) + r'): '
                  r'(?:SQLSTATE [0-9A-Z]{5}|Go type [A-Za-z0-9_.*\[\]]{1,120}))')


def text(raw, what):
    try:
        return raw.decode()
    except UnicodeDecodeError:
        raise ValueError('invalid ' + what) from None


class Steps:
    def __init__(self, database):
        self.db, self.schemas = database, {}

    def schema(self, image, version):
        positive(version, 'frameworks schema version')
        key = (image, version)
        if key not in self.schemas:
            output = text(self.db.server(image, 'frameworks-schema', '--version', str(version)),
                          'frameworks schema output')
            objects(output)
            self.schemas[key] = output
        return self.schemas[key]

    def select(self, row, dump, image):
        schema = self.schema(image, row['manifest']['schema_version'])
        return toc(text(self.db.listing(dump), 'dump TOC'), self.db.rules['extensions'], schema)

    def admin(self, database, statements):
        sql.database(database)
        self.db.check_lock()
        return self.db.host.user('podman', 'exec', '-i', '--env', 'PGOPTIONS=' + sql.ADMIN_OPTIONS, *SCRAM_ONLY,
                                 'trachuan-postgres',
                                 '/usr/local/bin/postgres-admin.sh', 'psql', '-XqAt', '-h', '127.0.0.1', '-U',
                                 'postgres', '-d', database, '-v', 'ON_ERROR_STOP=1',
                                 input=statements.encode()).decode().strip()

    def session(self, statements, name, path=sql.CATALOG_PATH):
        return self.db.query(statements, self.db.target(name), path)

    def settings(self, name):
        if self.session(sql.effective(), name) != sql.EFFECTIVE:
            raise ValueError('frameworks log settings refused')

    def inspect(self, row, dump, image):
        self.db.check_extensions(row['manifest'])
        self.select(row, dump, image)

    def create(self, row):
        name, manifest = row['database_name'], row['manifest']
        sql.database(name)
        self.db.check_extensions(manifest)
        self.db.make(name)
        outcome = self.admin(name, sql.configure(name, manifest['extensions']))
        if outcome == 'granted':
            raise ValueError('frameworks log parameter SET granted to a role other than a superuser')
        if outcome != 'configured':
            raise ValueError('frameworks log settings not configured')
        self.settings(name)

    def restore(self, row, dump, image):
        name = row['database_name']
        sql.database(name)
        self.settings(name)
        if self.session(sql.restored(), name) != 't':
            self.db.load(name, dump, self.db.save_list(row, self.select(row, dump, image)), ' ' + sql.CATALOG_PATH)
        schema = self.schema(image, row['manifest']['schema_version'])
        self.session(sql.apply(schema), name, sql.SCHEMA_PATH)
        outcome = self.session(check.executable(schema, row['manifest']['extensions']), name)
        if outcome != 'ok':
            kind = outcome if outcome in check.KINDS else 'object'
            raise ValueError('frameworks database ' + kind + ' differs from image schema version')
        self.session(sql.grants(name), name, sql.SCHEMA_PATH)

    def validate(self, row):
        name = row['database_name']
        sql.database(name)
        self.settings(name)
        raw = self.session(sql.release_row(), name)
        expected = dict({key: row['manifest'][key] for key in ('rel', 'schema_version', 'recipes', 'counts')},
                        kind='frameworks')
        if not raw or decode(raw, 'frameworks release row') != expected:
            raise ValueError('restored release row differs from manifest')

    def smoke(self, row, image):
        name = row['database_name']
        sql.database(name)
        self.db.check_lock()
        report = self.db.describe(image, name, safe=SAFE)
        if not isinstance(report, dict) or report.get('status') != 'ok':
            raise ValueError('frameworks release-check did not report ok')
