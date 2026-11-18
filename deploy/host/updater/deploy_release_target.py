import json
from pathlib import Path
import re

from .deploy_corpus_sql import literal
from .deploy_io import atomic, canonical


MISMATCH = 'target-identity-differs'
REFUSED = 'activation target database identity changed'
BACKENDS_REFUSED = 'another activator session could not be ended'
CLOSED = "('activated','reverted','aborted','pruned')"
STRAYS = f"""WITH protected(name) AS (
 SELECT active_db FROM public.corpus_release
 UNION SELECT active_db FROM public.frameworks_release
 UNION SELECT db_name FROM public.retired_release WHERE retain_until > pg_catalog.now()
 UNION SELECT j.database_name FROM (
  SELECT DISTINCT ON (operation_id) operation_id, database_name, mode, step FROM public.release_journal
  WHERE operation_id IS NOT NULL ORDER BY operation_id, id DESC
 ) j WHERE j.mode <> 'prune' AND j.step NOT IN {CLOSED}
 UNION SELECT 'corpus_'||country||'_'||rel FROM public.release_journal
  WHERE operation_id IS NULL AND step NOT IN {CLOSED}
  AND state NOT IN ('complete','done','activated','reverted','aborted','pruned')
)
SELECT pg_catalog.format('ALTER DATABASE %I IS_TEMPLATE false',d.datname),
 pg_catalog.format('DROP DATABASE %I',d.datname)
FROM pg_catalog.pg_database d WHERE pg_catalog.pg_get_userbyid(d.datdba)=current_user
AND NOT EXISTS (SELECT FROM protected p WHERE p.name=d.datname::pg_catalog.text) \\gexec
"""


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


class Targets:
    def __init__(self, directory):
        self.directory = Path(directory)

    def path(self, operation):
        if not isinstance(operation, str) or not re.fullmatch('[0-9a-f]{32}', operation):
            raise ValueError('invalid activation operation id')
        return self.directory / (operation + '.json')

    def read(self, operation, name):
        try:
            value = json.loads(self.path(operation).read_bytes())
        except FileNotFoundError:
            return None
        except ValueError:
            raise ValueError(REFUSED) from None
        if not isinstance(value, dict) or value.get('database') != name:
            raise ValueError(REFUSED)
        return number(value.get('oid'))

    def write(self, operation, name, oid):
        path = self.path(operation)
        number(oid)
        self.directory.mkdir(parents=True, mode=0o700, exist_ok=True)
        atomic(path, canonical(dict(database=name, oid=oid)))

    def keep(self, operations):
        if not self.directory.is_dir():
            return
        for path in self.directory.glob('*.json'):
            if path.stem not in operations:
                path.unlink()
