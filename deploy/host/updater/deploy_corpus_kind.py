import re
import shutil

from . import deploy_corpus_sql, deploy_frameworks_sql
from . import deploy_corpus_verify as law_verify
from . import deploy_frameworks_verify as frameworks_verify
from .deploy_frameworks_steps import Steps


class Law:
    name, key, label, sql = 'law', 'corpus', 'corpus', deploy_corpus_sql
    tag, result, dump = 'vn-production', 'release', 'corpus.dump'
    artifacts, marker = 'corpus-artifacts', 'corpus-prune.json'
    check, bootstrap = (), False
    unpublished = 'no corpus release published'
    waiting = 'recipe pair activation is not implemented'

    def settings(self, rules):
        return rules['corpus_repository'], rules['release_namespace'], rules['release_bucket']

    def ready(self, state, rules):
        return True

    def verify(self, directory, rules, runner):
        return law_verify.verify(directory, rules, runner)

    def compatible(self, manifest, report):
        return law_verify.compatible(manifest, report)

    def country(self, manifest):
        return manifest['country']

    def staging(self, manifest):
        return manifest['country'] + '-' + manifest['rel']

    def previous(self, manifest):
        return 'corpus_' + manifest['country'] + '_' + manifest['previous']

    def parse(self, name):
        _, country, rel = name.split('_', 2)
        return country, rel

    def steps(self, database):
        return database

    def activation(self, database, row):
        return self.sql.activate(row, database.projection(row))

    def cause(self, detail):
        return detail

    def discard(self, releases):
        pass


class Frameworks:
    name, key, label, sql = 'frameworks', 'frameworks', 'frameworks', deploy_frameworks_sql
    tag, result, dump = 'frameworks-production', 'release-frameworks', 'frameworks.dump'
    artifacts, marker = 'frameworks-artifacts', 'frameworks-prune.json'
    check, bootstrap = ('--kind', 'frameworks'), True
    unpublished = 'no frameworks release published'
    waiting = 'frameworks schema or recipe not served by the image'

    def settings(self, rules):
        block = rules['frameworks']
        return block['repository'], block['namespace'], block['bucket']

    def ready(self, state, rules):
        return state.data.get('release_floor', 0) >= rules['frameworks']['min_release_floor']

    def verify(self, directory, rules, runner):
        return frameworks_verify.verify(directory, rules, runner)

    def compatible(self, manifest, report):
        return frameworks_verify.compatible(manifest, report)

    def country(self, manifest):
        return 'vn'

    def staging(self, manifest):
        return 'frameworks-' + manifest['rel']

    def previous(self, manifest):
        return None if manifest['previous'] is None else 'frameworks_' + manifest['previous']

    def parse(self, name):
        self.sql.database(name)
        return 'vn', name[len('frameworks_'):]

    def steps(self, database):
        return Steps(database)

    def activation(self, database, row):
        return self.sql.activate(row)

    def cause(self, detail):
        found = re.match(r'[^:]*: exit -?[0-9]+', detail)
        return found[0] if found else None

    def discard(self, releases):
        if not releases.is_dir():
            return
        for path in releases.iterdir():
            if (re.fullmatch('frameworks-' + frameworks_verify.RELEASE, path.name)
                    and path.is_dir() and not path.is_symlink()):
                shutil.rmtree(path)


LAW, FRAMEWORKS = Law(), Frameworks()
