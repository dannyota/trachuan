from .deploy_corpus_sql import literal, plain


KINDS = ('view', 'materialized view', 'trigger', 'function', 'procedure', 'aggregate', 'rule', 'policy',
         'event trigger', 'operator', 'cast', 'schema', 'extension', 'type', 'domain', 'setting', 'access')
USER = "n.nspname <> 'information_schema' AND pg_catalog.left(n.nspname,3) <> 'pg_'"


def member(catalog, alias):
    return ("NOT EXISTS (SELECT FROM pg_catalog.pg_depend d WHERE d.classid='pg_catalog." + catalog +
            "'::pg_catalog.regclass AND d.objid=" + alias + ".oid AND d.deptype='e')")


ENTRY = ("CASE WHEN x.grantee=0::oid THEN 'public' ELSE pg_catalog.pg_get_userbyid(x.grantee)::text END"
         "||' '||x.privilege_type||CASE WHEN x.is_grantable THEN ' grantable' ELSE '' END")
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
 UNION ALL SELECT 'access', 'database '||d.datname::text, 'database '||{ENTRY}
  FROM pg_catalog.pg_database d
  CROSS JOIN LATERAL pg_catalog.aclexplode(coalesce(d.datacl,pg_catalog.acldefault('d',d.datdba))) x
  WHERE d.datname=pg_catalog.current_database() AND x.grantee<>d.datdba
 UNION ALL SELECT 'access', 'relation '||n.nspname::text||'.'||c.relname::text, 'relation '||{ENTRY}
  FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
  CROSS JOIN LATERAL pg_catalog.aclexplode(c.relacl) x WHERE x.grantee<>c.relowner AND {USER}
 UNION ALL SELECT 'access', 'column '||n.nspname::text||'.'||c.relname::text||'.'||a.attname::text, 'column '||{ENTRY}
  FROM pg_catalog.pg_attribute a JOIN pg_catalog.pg_class c ON c.oid=a.attrelid
  JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
  CROSS JOIN LATERAL pg_catalog.aclexplode(a.attacl) x WHERE x.grantee<>c.relowner AND {USER}
 UNION ALL SELECT 'access', 'schema '||n.nspname::text, 'schema '||{ENTRY}
  FROM pg_catalog.pg_namespace n CROSS JOIN LATERAL pg_catalog.aclexplode(n.nspacl) x
  WHERE {USER} AND x.grantee<>n.nspowner
  AND NOT (x.grantee=0::oid AND x.privilege_type='USAGE' AND NOT x.is_grantable)
 UNION ALL SELECT 'access', 'default '||d.oid::text,
  'default '||CASE WHEN r.rolsuper THEN 'superuser' ELSE 'role' END||' '||coalesce(n.nspname::text,'')||' '||
  d.defaclobjtype::text||' '||pg_catalog.array_to_string(ARRAY(SELECT {ENTRY}
   FROM pg_catalog.aclexplode(d.defaclacl) x ORDER BY 1),',')
  FROM pg_catalog.pg_default_acl d JOIN pg_catalog.pg_roles r ON r.oid=d.defaclrole
  LEFT JOIN pg_catalog.pg_namespace n ON n.oid=d.defaclnamespace
) objects"""


def rows(items):
    return ','.join('(' + ','.join(literal(value) for value in row) + ')'
                    for row in sorted(items, key=lambda row: row[:2]))


def extensions(versions, retained=False):
    return [('extension', plain(name), None if retained else version + ' public superuser')
            for name, version in versions.items()]


def access_allowed(alias, reader, granted):
    role = plain(reader)
    if not granted:
        return 'FALSE'
    allowed = ('database ' + role + ' CONNECT', 'relation ' + role + ' SELECT', 'schema ' + role + ' USAGE',
               'default superuser public r ' + role + ' SELECT', 'default superuser public S ' + role + ' SELECT')
    return ('(' + alias + ".kind='access' AND " + alias + '.definition IN (' +
            ','.join(literal(value) for value in allowed) + '))')


def law(versions, reader='corpus_reader', granted=False, retained=False):
    return ('WITH found AS (' + SNAPSHOT + '),\nexpected(kind, identity, definition) AS (VALUES ' +
            rows(extensions(versions, retained)) + '), differences AS (\n'
            ' SELECT f.kind FROM found f WHERE NOT EXISTS (SELECT FROM expected e WHERE e.kind=f.kind '
            'AND e.identity=f.identity AND (e.definition IS NULL OR e.definition=f.definition))\n'
            " AND NOT (f.kind='extension' AND f.identity='pg_prewarm' AND f.definition LIKE '% public superuser')\n"
            ' AND NOT ' + access_allowed('f', reader, granted) + '\n'
            " UNION ALL SELECT f.kind FROM found f WHERE f.kind='extension' AND f.definition NOT LIKE '% public superuser'\n"
            ' UNION ALL SELECT e.kind FROM expected e WHERE NOT EXISTS (SELECT FROM found f WHERE f.kind=e.kind '
            'AND f.identity=e.identity AND (e.definition IS NULL OR f.definition=e.definition)))\n'
            "SELECT coalesce((SELECT kind FROM differences ORDER BY kind LIMIT 1),'ok');\n")
