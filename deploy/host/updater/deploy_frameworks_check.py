from .deploy_corpus_sql import literal
from .deploy_frameworks_schema import objects
from .deploy_frameworks_sql import LOG_SETTINGS, guard, plain


KINDS = ('view', 'materialized view', 'trigger', 'function', 'procedure', 'aggregate', 'rule', 'policy',
         'event trigger', 'operator', 'cast', 'schema', 'extension', 'type', 'domain', 'setting')
USER = "n.nspname <> 'information_schema' AND pg_catalog.left(n.nspname,3) <> 'pg_'"


def member(catalog, alias):
    return ("NOT EXISTS (SELECT FROM pg_catalog.pg_depend d WHERE d.classid='pg_catalog." + catalog +
            "'::pg_catalog.regclass AND d.objid=" + alias + ".oid AND d.deptype='e')")


SNAPSHOT = f"""SELECT kind, identity, definition FROM (
 SELECT CASE WHEN c.relkind='v' THEN 'view' ELSE 'materialized view' END AS kind,
  n.nspname::text||'.'||c.relname::text AS identity, pg_catalog.pg_get_viewdef(c.oid) AS definition
  FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
  WHERE c.relkind IN ('v','m') AND {USER}
 UNION ALL SELECT 'trigger', n.nspname::text||'.'||c.relname::text||' '||t.tgname::text,
  pg_catalog.pg_get_triggerdef(t.oid)
  FROM pg_catalog.pg_trigger t JOIN pg_catalog.pg_class c ON c.oid=t.tgrelid
  JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE NOT t.tgisinternal
 UNION ALL SELECT CASE WHEN p.prokind='a' THEN 'aggregate' WHEN p.prokind='p' THEN 'procedure' ELSE 'function' END,
  n.nspname::text||'.'||p.proname::text||'('||pg_catalog.pg_get_function_identity_arguments(p.oid)||')',
  CASE WHEN p.prokind='a' THEN '' ELSE pg_catalog.pg_get_functiondef(p.oid) END
  FROM pg_catalog.pg_proc p JOIN pg_catalog.pg_namespace n ON n.oid=p.pronamespace
  WHERE {USER} AND {member('pg_proc', 'p')}
 UNION ALL SELECT 'rule', n.nspname::text||'.'||c.relname::text||' '||r.rulename::text, pg_catalog.pg_get_ruledef(r.oid)
  FROM pg_catalog.pg_rewrite r JOIN pg_catalog.pg_class c ON c.oid=r.ev_class
  JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE r.rulename <> '_RETURN' AND {USER}
 UNION ALL SELECT 'policy', n.nspname::text||'.'||c.relname::text||' '||p.polname::text, ''
  FROM pg_catalog.pg_policy p JOIN pg_catalog.pg_class c ON c.oid=p.polrelid
  JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
 UNION ALL SELECT 'event trigger', e.evtname::text, '' FROM pg_catalog.pg_event_trigger e
 UNION ALL SELECT 'operator', n.nspname::text||'.'||o.oprname::text||'('||pg_catalog.format_type(o.oprleft,NULL)||','||
  pg_catalog.format_type(o.oprright,NULL)||')', ''
  FROM pg_catalog.pg_operator o JOIN pg_catalog.pg_namespace n ON n.oid=o.oprnamespace
  WHERE {USER} AND {member('pg_operator', 'o')}
 UNION ALL SELECT 'cast', pg_catalog.format_type(c.castsource,NULL)||' AS '||pg_catalog.format_type(c.casttarget,NULL), ''
  FROM pg_catalog.pg_cast c WHERE c.oid >= 16384 AND {member('pg_cast', 'c')}
 UNION ALL SELECT 'schema', n.nspname::text, '' FROM pg_catalog.pg_namespace n WHERE {USER} AND n.nspname <> 'public'
 UNION ALL SELECT 'extension', x.extname::text,
  x.extversion||' '||n.nspname::text||' '||CASE WHEN r.rolsuper THEN 'superuser' ELSE 'role' END
  FROM pg_catalog.pg_extension x JOIN pg_catalog.pg_namespace n ON n.oid=x.extnamespace
  JOIN pg_catalog.pg_roles r ON r.oid=x.extowner WHERE x.oid >= 16384
 UNION ALL SELECT CASE WHEN t.typtype='d' THEN 'domain' ELSE 'type' END, n.nspname::text||'.'||t.typname::text, ''
  FROM pg_catalog.pg_type t JOIN pg_catalog.pg_namespace n ON n.oid=t.typnamespace
  WHERE {USER} AND {member('pg_type', 't')}
  AND NOT EXISTS (SELECT FROM pg_catalog.pg_class c WHERE c.oid=t.typrelid AND c.relkind <> 'c')
  AND NOT EXISTS (SELECT FROM pg_catalog.pg_type a WHERE a.typarray=t.oid)
 UNION ALL SELECT 'setting',
  CASE WHEN s.setrole=0::oid THEN 'database' ELSE 'role '||pg_catalog.pg_get_userbyid(s.setrole)::text END,
  pg_catalog.array_to_string(ARRAY(SELECT u.c FROM pg_catalog.unnest(s.setconfig) AS u(c)
   ORDER BY u.c COLLATE pg_catalog."C"),',')
  FROM pg_catalog.pg_db_role_setting s JOIN pg_catalog.pg_database d ON d.oid=s.setdatabase
  WHERE d.datname=pg_catalog.current_database()
) objects"""


def identity(item):
    if item[0] == 'VIEW':
        return 'view', 'public.' + item[1]
    if item[0] == 'FUNCTION':
        return 'function', 'public.' + item[1] + '()'
    return 'trigger', 'public.' + item[1] + ' ' + item[2]


def expected(schema, extensions, activator, reader):
    logs = ','.join(sorted(name + '=' + value for name, value in LOG_SETTINGS))
    rows = [identity(item) + (None,) for item in objects(schema)]
    rows += [('extension', plain(name), version + ' public superuser') for name, version in extensions.items()]
    rows += [('setting', 'database', logs), ('setting', 'role ' + plain(activator), logs),
             ('setting', 'role ' + plain(reader), logs)]
    return ','.join('(' + ','.join(literal(value) for value in row) + ')'
                    for row in sorted(rows, key=lambda row: row[:2]))


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
