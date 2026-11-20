import json

from . import deploy_corpus_sql as law
from .deploy_io import CommandError
from .deploy_release_target import Waiting


class TableFailure(Waiting):
    pass


class PointerRefused(Waiting):
    pass


class Tables:
    def __init__(self, host, kind):
        self.host, self.kind, self.sql = host, kind, kind.sql

    def read(self, statement):
        return self.call(statement, False)

    def write(self, statement):
        return self.call(statement, True)

    def call(self, statement, write):
        try:
            return self.host.release_sql(statement, write)
        except (CommandError, UnicodeDecodeError) as error:
            raise TableFailure('auth tables unavailable') from error

    def pointer(self):
        value = self.read(self.sql.current(self.kind.pointer_country))
        if not value:
            return None
        try:
            self.sql.database(value)
        except ValueError:
            raise PointerRefused('active pointer is not a release database') from None
        return value

    def protected(self, operation=None):
        try:
            names = json.loads(self.read(law.protected_names(operation)))
        except ValueError:
            raise TableFailure('auth tables unreadable') from None
        if not isinstance(names, list) or not all(isinstance(name, str) for name in names):
            raise TableFailure('auth tables unreadable')
        return set(names)

    def operations(self, own=None):
        rows = law.journal_json(self.read(self.sql.operations(own=own)))
        if not isinstance(rows, list):
            raise law.JournalRefused(law.REFUSED)
        return [self.sql.journal_row(row) for row in rows]

    def recorded(self, raw, row, step):
        inserted = dict(row, step=step)
        expected = dict(inserted, corpus_kind=self.kind.name, **law.written(step, self.kind.label))
        law.same_row(self.sql.journal_row(law.journal_json(raw)), expected)
        return inserted

    def append(self, row, step):
        return self.recorded(self.write(self.sql.journal(row, step)), row, step)

    def close_open(self, keep=None):
        self.write(self.sql.close_open(keep))

    def swap(self, statement, row):
        return self.recorded(self.write(statement), row, 'activated')

    def forget(self, name):
        self.sql.database(name)
        self.write(self.sql.forget(name))
