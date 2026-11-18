import re

from . import deploy_corpus_sql as law
from . import deploy_frameworks_check as check
from . import deploy_frameworks_sql as sql
from .deploy_frameworks_schema import objects, toc
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

    def session(self, statements, row, path=sql.CATALOG_PATH):
        return self.db.on_target(row, statements, path)

    def settings(self, row):
        if self.session(sql.effective(), row) != sql.EFFECTIVE:
            raise ValueError('frameworks log settings refused')

    def inspect(self, row, dump, image):
        self.db.check_extensions(row['manifest'])
        self.select(row, dump, image)

    def create(self, row):
        name, manifest = row['database_name'], row['manifest']
        statements = sql.configure(name, manifest['extensions'])
        self.db.check_extensions(manifest)
        self.db.make(row)
        outcome = self.db.target_admin(row, statements, sql.ADMIN_OPTIONS)
        if outcome == 'superuser':
            raise ValueError(law.SUPERUSER_REFUSED)
        if outcome == 'granted':
            raise ValueError('frameworks log parameter SET granted to a role other than a superuser')
        if outcome != 'configured':
            raise ValueError('frameworks log settings not configured')
        self.settings(row)

    def restore(self, row, dump, image):
        name = row['database_name']
        sql.database(name)
        self.settings(row)
        fresh = self.session(sql.restored(), row) != 't'
        if fresh:
            self.db.load(row, dump, self.db.save_list(row, self.select(row, dump, image)))
        self.session(sql.apply(self.schema(image, row['manifest']['schema_version'])), row, sql.SCHEMA_PATH)
        self.check_catalog(row, image, not fresh)
        self.session(sql.grants(name), row, sql.SCHEMA_PATH)

    def check_catalog(self, row, image, granted):
        schema = self.schema(image, row['manifest']['schema_version'])
        statements = check.executable(schema, row['manifest']['extensions'], granted=granted,
                                      retained=row['mode'] == 'retained')
        outcome = self.session(statements, row)
        if outcome != 'ok':
            kind = outcome if outcome in check.KINDS else 'object'
            raise ValueError('frameworks database ' + kind + ' differs from image schema version')

    def validate(self, row):
        sql.database(row['database_name'])
        self.settings(row)
        raw = self.session(sql.release_row(), row)
        expected = dict({key: row['manifest'][key] for key in ('rel', 'schema_version', 'recipes', 'counts')},
                        kind='frameworks')
        if not raw or decode(raw, 'frameworks release row') != expected:
            raise ValueError('restored release row differs from manifest')

    def smoke(self, row, image):
        name = row['database_name']
        sql.database(name)
        self.db.check_lock()
        self.check_catalog(row, image, True)
        self.db.verify_target(row)
        report = self.db.describe(image, name, safe=SAFE)
        self.db.verify_target(row)
        if not isinstance(report, dict) or report.get('status') != 'ok':
            raise ValueError('frameworks release-check did not report ok')
