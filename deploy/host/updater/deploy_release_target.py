import re

from .deploy_corpus_sql import literal


MISMATCH = 'target-identity-differs'
REFUSED = 'activation target database identity changed'
BACKEND_LEFT = 'leftover backend'
DATABASE_LEFT = 'leftover database in use'
OWNED = 'pg_catalog.pg_get_userbyid(d.datdba)=current_user'
DROP = ("SELECT pg_catalog.format('ALTER DATABASE %I IS_TEMPLATE false',s.datname),"
        "pg_catalog.format('DROP DATABASE %I',s.datname) FROM ({source}) s \\gexec\n")


class Waiting(Exception):
    pass


class Leftover(Waiting):
    pass


class Held(Waiting):
    pass


class LockBusy(Waiting):
    pass


class LockUnavailable(Waiting):
    pass


class LockLost(Waiting):
    pass


class Unreachable(Waiting):
    pass


def number(value):
    if not isinstance(value, str) or not re.fullmatch('[1-9][0-9]*', value):
        raise ValueError(REFUSED)
    return value


def end_backends(keep):
    if type(keep) is not int or keep < 0:
        raise ValueError('invalid activation lock backend')
    others = ('FROM pg_catalog.pg_stat_activity a WHERE a.usename=current_user '
              'AND a.pid NOT IN (pg_catalog.pg_backend_pid(),' + str(keep) + ')')
    return ('SELECT pg_catalog.count(pg_catalog.pg_terminate_backend(a.pid,5000)) ' + others + ';\n'
            'SELECT EXISTS(SELECT ' + others + ');\n')


def unprotected(protected):
    names = ','.join(literal(name) for name in sorted(protected))
    return ('SELECT d.datname FROM pg_catalog.pg_database d WHERE ' + OWNED +
            ' AND d.datname::pg_catalog.text <> ALL(ARRAY[' + names + ']::pg_catalog.text[])')


def strays(protected):
    source = unprotected(protected)
    return ('\\set ON_ERROR_STOP off\n' + DROP.format(source=source) +
            '\\set ON_ERROR_STOP on\n'
            'SELECT EXISTS(' + source + ');\n')


def drop(name, oid=None):
    source = 'SELECT d.datname FROM pg_catalog.pg_database d WHERE d.datname=' + literal(name) + ' AND ' + OWNED
    if oid is not None:
        source += ' AND d.oid=' + number(str(oid)) + '::pg_catalog.oid'
    return '\\set ON_ERROR_STOP off\n' + DROP.format(source=source) + '\\set ON_ERROR_STOP on\n'


def target_oid(name):
    return 'SELECT oid AS target_oid FROM pg_catalog.pg_database WHERE datname=' + literal(name) + ';'


def guard(oid, statements=''):
    body = statements if not statements or statements.endswith('\n') else statements + '\n'
    return ("SELECT d.oid::pg_catalog.text='" + number(oid) + "' AS release_target FROM pg_catalog.pg_database d "
            'WHERE d.datname=pg_catalog.current_database() \\gset\n'
            '\\if :release_target\n' + body +
            '\\else\n'
            "SELECT '" + MISMATCH + "';\n"
            '\\endif\n')
