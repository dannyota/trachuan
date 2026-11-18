from .deploy_frameworks_schema import objects
from .deploy_frameworks_sql import LOG_SETTINGS, guard
from .deploy_corpus_sql import plain
from .deploy_release_catalog import KINDS, SNAPSHOT, extensions as extension_rows, rows


def identity(item):
    if item[0] == 'VIEW':
        return 'view', 'public.' + item[1]
    if item[0] == 'FUNCTION':
        return 'function', 'public.' + item[1] + '()'
    return 'trigger', 'public.' + item[1] + ' ' + item[2]


def expected(schema, extensions, activator, reader):
    logs = ','.join(sorted(name + '=' + value for name, value in LOG_SETTINGS))
    items = [identity(item) + (None,) for item in objects(schema)] + extension_rows(extensions)
    items += [('setting', 'database', logs), ('setting', 'role ' + plain(activator), logs),
              ('setting', 'role ' + plain(reader), logs)]
    return rows(items)


def executable(schema, extensions, activator='activator', reader='corpus_reader'):
    return ('BEGIN;\n' + guard() +
            'CREATE TEMP TABLE frameworks_before ON COMMIT DROP AS ' + SNAPSHOT + ';\n'
            'SET LOCAL search_path = public;\n' + schema + '\n'
            'SET LOCAL search_path = pg_catalog;\n'
            'CREATE TEMP TABLE frameworks_after ON COMMIT DROP AS ' + SNAPSHOT + ';\n'
            'WITH expected(kind, identity, definition) AS (VALUES ' +
            expected(schema, extensions, activator, reader) + '), differences AS (\n'
            ' (SELECT kind, identity, definition FROM pg_temp.frameworks_before EXCEPT '
            'SELECT kind, identity, definition FROM pg_temp.frameworks_after)\n'
            ' UNION ALL (SELECT kind, identity, definition FROM pg_temp.frameworks_after EXCEPT '
            'SELECT kind, identity, definition FROM pg_temp.frameworks_before)\n'
            ' UNION ALL (SELECT kind, identity, NULL::text FROM pg_temp.frameworks_after EXCEPT '
            'SELECT kind, identity, NULL::text FROM expected)\n'
            ' UNION ALL (SELECT kind, identity, NULL::text FROM expected EXCEPT '
            'SELECT kind, identity, NULL::text FROM pg_temp.frameworks_after)\n'
            ' UNION ALL SELECT a.kind, a.identity, NULL::text FROM pg_temp.frameworks_after a '
            'JOIN expected e ON e.kind=a.kind AND e.identity=a.identity '
            'WHERE e.definition IS NOT NULL AND a.definition IS DISTINCT FROM e.definition)\n'
            "SELECT coalesce((SELECT kind FROM differences ORDER BY kind LIMIT 1),'ok');\n"
            'ROLLBACK;\n')
