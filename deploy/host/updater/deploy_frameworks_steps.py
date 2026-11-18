import re

from . import deploy_frameworks_sql as sql
from .deploy_corpus_sql import PREWARM
from .deploy_frameworks_schema import objects, toc
from .deploy_io import decode


SAFE = re.compile(r'server: (frameworks release-check [a-z]{1,20}(?: [a-z]{1,20}){0,5}: '
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
        self.db.check_lock()
        return self.db.host.user('podman', 'exec', '-i', '--env', 'PGOPTIONS=' + sql.LOG_OPTIONS, 'trachuan-postgres',
                                 '/usr/local/bin/postgres-admin.sh', 'psql', '-XqAt', '-h', '127.0.0.1', '-U',
                                 'postgres', '-d', database, '-v', 'ON_ERROR_STOP=1',
                                 input=statements.encode()).decode().strip()

    def settings(self, name):
        if self.db.query(sql.effective(), name) != sql.EFFECTIVE:
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
        if self.db.query(sql.restored(), name) != 't':
            self.db.load(name, dump, self.db.save_list(row, self.select(row, dump, image)))
        self.db.query(sql.apply(self.schema(image, row['manifest']['schema_version'])), name)
        self.db.grant(name)

    def validate(self, row):
        name = row['database_name']
        sql.database(name)
        self.settings(name)
        raw = self.db.query(sql.release_row(), name)
        expected = {key: row['manifest'][key] for key in ('rel', 'schema_version', 'recipes')}
        if not raw or decode(raw, 'frameworks release row') != expected:
            raise ValueError('restored release row differs from manifest')

    def smoke(self, row, image):
        name = row['database_name']
        sql.database(name)
        self.admin(name, sql.guard() + PREWARM)
        report = self.db.describe(image, name, safe=SAFE)
        if not isinstance(report, dict) or report.get('status') != 'ok':
            raise ValueError('frameworks release-check did not report ok')
