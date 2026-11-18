import re

from . import deploy_corpus_sql as law
from .deploy_corpus_sql import literal
from .deploy_frameworks_schema import objects


KIND = 'frameworks'
LOG_SETTINGS = (('log_min_messages', 'panic'), ('log_min_error_statement', 'panic'),
                ('log_min_duration_statement', '-1'), ('log_statement', 'none'))
LOG_OPTIONS = ' '.join('-c ' + name + '=' + value for name, value in LOG_SETTINGS)
EFFECTIVE = ','.join(value for _, value in LOG_SETTINGS)
ROLES = ('activator', 'corpus_reader')
KINDS = ('view', 'materialized view', 'trigger', 'function', 'procedure', 'aggregate', 'rule', 'policy',
         'event trigger', 'operator', 'cast', 'schema')
SYSTEM = "('pg_catalog','information_schema')"
SNAPSHOT = f"""SELECT kind, identity, definition FROM (
 SELECT CASE WHEN c.relkind='v' THEN 'view' ELSE 'materialized view' END AS kind,
  n.nspname::text||'.'||c.relname::text AS identity, pg_get_viewdef(c.oid) AS definition
  FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.relkind IN ('v','m') AND n.nspname NOT IN {SYSTEM}
 UNION ALL SELECT 'trigger', n.nspname::text||'.'||c.relname::text||' '||t.tgname::text, pg_get_triggerdef(t.oid)
  FROM pg_trigger t JOIN pg_class c ON c.oid=t.tgrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE NOT t.tgisinternal
 UNION ALL SELECT CASE WHEN p.prokind='a' THEN 'aggregate' WHEN p.prokind='p' THEN 'procedure' ELSE 'function' END,
  n.nspname::text||'.'||p.proname::text||'('||pg_get_function_identity_arguments(p.oid)||')',
  CASE WHEN p.prokind='a' THEN '' ELSE pg_get_functiondef(p.oid) END
  FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname NOT IN {SYSTEM}
  AND NOT EXISTS (SELECT FROM pg_depend d WHERE d.classid='pg_proc'::regclass AND d.objid=p.oid AND d.deptype='e')
 UNION ALL SELECT 'rule', n.nspname::text||'.'||c.relname::text||' '||r.rulename::text, pg_get_ruledef(r.oid)
  FROM pg_rewrite r JOIN pg_class c ON c.oid=r.ev_class JOIN pg_namespace n ON n.oid=c.relnamespace
  WHERE r.rulename <> '_RETURN' AND n.nspname NOT IN {SYSTEM}
 UNION ALL SELECT 'policy', n.nspname::text||'.'||c.relname::text||' '||p.polname::text, ''
  FROM pg_policy p JOIN pg_class c ON c.oid=p.polrelid JOIN pg_namespace n ON n.oid=c.relnamespace
 UNION ALL SELECT 'event trigger', e.evtname::text, '' FROM pg_event_trigger e
 UNION ALL SELECT 'operator', n.nspname::text||'.'||o.oprname::text||'('||format_type(o.oprleft,NULL)||','||
  format_type(o.oprright,NULL)||')', ''
  FROM pg_operator o JOIN pg_namespace n ON n.oid=o.oprnamespace WHERE n.nspname NOT IN {SYSTEM}
  AND NOT EXISTS (SELECT FROM pg_depend d WHERE d.classid='pg_operator'::regclass AND d.objid=o.oid AND d.deptype='e')
 UNION ALL SELECT 'cast', format_type(c.castsource,NULL)||' AS '||format_type(c.casttarget,NULL), '' FROM pg_cast c
  WHERE c.oid >= 16384
  AND NOT EXISTS (SELECT FROM pg_depend d WHERE d.classid='pg_cast'::regclass AND d.objid=c.oid AND d.deptype='e')
 UNION ALL SELECT 'schema', n.nspname::text, '' FROM pg_namespace n
  WHERE left(n.nspname,3) <> 'pg_' AND n.nspname NOT IN ('information_schema','public')
) objects"""


def database(name):
    if not isinstance(name, str) or not re.fullmatch(r'frameworks_[0-9]{8}(?:_[0-9]{1,2})?', name):
        raise ValueError('invalid frameworks database name')
    return '"' + name + '"'


def plain(name):
    if not isinstance(name, str) or not re.fullmatch('[a-z_][a-z0-9_]{0,62}', name):
        raise ValueError('invalid frameworks role or extension name')
    return name


def effective():
    return "SELECT concat_ws(','," + ','.join("current_setting('" + name + "')" for name, _ in LOG_SETTINGS) + ');'


def guard():
    differs = ' OR '.join("current_setting('" + name + "') <> '" + value + "'" for name, value in LOG_SETTINGS)
    return ('DO $guard$ BEGIN IF ' + differs +
            " THEN RAISE EXCEPTION 'frameworks log settings refused'; END IF; END $guard$;\n")


def configure(name, extensions, roles=ROLES):
    target = database(name)
    scopes = ['DATABASE ' + target] + ['ROLE "' + plain(role) + '" IN DATABASE ' + target for role in roles]
    parameters = 'ARRAY[' + ','.join(literal(setting) for setting, _ in LOG_SETTINGS) + ']'
    settings = ''.join('ALTER ' + scope + ' SET ' + setting + ' = ' + literal(value) + ';\n'
                       for scope in scopes for setting, value in LOG_SETTINGS)
    created = ''.join('CREATE EXTENSION IF NOT EXISTS ' + plain(extension) + ' VERSION ' + literal(version) + ';\n'
                      for extension, version in extensions.items())
    return ('SELECT EXISTS(SELECT FROM pg_parameter_acl a CROSS JOIN aclexplode(a.paracl) x '
            'LEFT JOIN pg_roles r ON r.oid=x.grantee WHERE a.parname=ANY(' + parameters + ") "
            "AND x.privilege_type='SET' AND r.rolsuper IS NOT TRUE) "
            'OR EXISTS(SELECT FROM pg_roles r CROSS JOIN unnest(' + parameters + ') p '
            "WHERE NOT r.rolsuper AND has_parameter_privilege(r.oid,p,'SET')) AS frameworks_log_granted \\gset\n"
            '\\if :frameworks_log_granted\n'
            "SELECT 'granted';\n"
            '\\else\n'
            'BEGIN;\n' + settings + created + 'COMMIT;\n'
            "SELECT 'configured';\n"
            '\\endif\n')


def apply(schema):
    return 'BEGIN;\n' + guard() + schema + '\nCOMMIT;\n'


def release_row():
    return guard() + ("SELECT json_build_object('kind',kind,'rel',rel,'schema_version',schema_version,"
                      "'counts',json_build_object('versions',version_count,'controls',control_count),'recipes',"
                      "json_build_object('dense',dense_recipe,'chunk',chunk_recipe,'lexical',lexical_recipe,"
                      "'retrieval',retrieval_recipe)) FROM release WHERE singleton;")


def identity(item):
    if item[0] == 'VIEW':
        return 'view', 'public.' + item[1]
    if item[0] == 'FUNCTION':
        return 'function', 'public.' + item[1] + '()'
    return 'trigger', 'public.' + item[1] + ' ' + item[2]


def executable(schema):
    expected = sorted(identity(item) for item in objects(schema))
    rows = ('SELECT kind, identity FROM (VALUES ' +
            ','.join('(' + literal(kind) + ',' + literal(name) + ')' for kind, name in expected) +
            ') e(kind, identity)' if expected else 'SELECT NULL::text AS kind, NULL::text AS identity WHERE false')
    return ('BEGIN;\n' + guard() +
            'CREATE TEMP TABLE frameworks_before ON COMMIT DROP AS ' + SNAPSHOT + ';\n' + schema + '\n' +
            'CREATE TEMP TABLE frameworks_after ON COMMIT DROP AS ' + SNAPSHOT + ';\n'
            'WITH expected AS (' + rows + '), differences AS (\n'
            ' (SELECT kind, identity, definition FROM frameworks_before EXCEPT '
            'SELECT kind, identity, definition FROM frameworks_after)\n'
            ' UNION ALL (SELECT kind, identity, definition FROM frameworks_after EXCEPT '
            'SELECT kind, identity, definition FROM frameworks_before)\n'
            ' UNION ALL (SELECT kind, identity, NULL::text FROM frameworks_after EXCEPT '
            'SELECT kind, identity, NULL::text FROM expected)\n'
            ' UNION ALL (SELECT kind, identity, NULL::text FROM expected EXCEPT '
            'SELECT kind, identity, NULL::text FROM frameworks_after))\n'
            "SELECT coalesce((SELECT kind FROM differences ORDER BY kind LIMIT 1),'ok');\n"
            'ROLLBACK;\n')


def restored():
    return guard() + "SELECT to_regclass('public.release') IS NOT NULL;"


def current(country):
    return 'SELECT active_db FROM frameworks_release WHERE country=' + literal(country) + ';'


def retirement(name):
    return law.retirement(name, KIND)


def operations():
    return law.operations(KIND)


def expired():
    return law.expired(KIND)


def forget(name):
    return law.forget(name, KIND)


def journal(row, step):
    return law.journal(row, step, KIND, KIND)


def activate(row):
    country, target, expected = (literal(row[k]) for k in ('country', 'database_name', 'expected_active_db'))
    retained = ("DELETE FROM retired_release WHERE corpus_kind='frameworks' AND country=" + country +
                ' AND db_name=' + target + ';' if row['mode'] == 'retained' else '')
    return f"""BEGIN;
DO $activation$
DECLARE previous text; old_digest text; changed bigint;
BEGIN
  SELECT active_db INTO previous FROM frameworks_release WHERE country={country} FOR UPDATE;
  IF previous IS DISTINCT FROM {expected} THEN
    RAISE EXCEPTION 'active frameworks changed';
  END IF;
  IF previous IS NULL THEN
    INSERT INTO frameworks_release (country,active_db,activated_at,activated_by)
      VALUES ({country},{target},now(),'activator') ON CONFLICT (country) DO NOTHING;
  ELSE
    UPDATE frameworks_release SET active_db={target},activated_at=now(),activated_by='activator'
      WHERE country={country} AND active_db={expected};
  END IF;
  GET DIAGNOSTICS changed = ROW_COUNT;
  IF changed <> 1 THEN
    RAISE EXCEPTION 'active frameworks update must affect exactly one row';
  END IF;
  SELECT dump_sha256 INTO old_digest FROM release_journal
    WHERE corpus_kind='frameworks' AND country={country} AND database_name=previous AND step='activated'
    ORDER BY id DESC LIMIT 1;
  IF previous IS NOT NULL AND previous <> {target} THEN
    INSERT INTO retired_release (country,db_name,retired_at,retain_until,dropped_at,dump_sha256,corpus_kind)
      VALUES ({country},previous,now(),now()+interval '7 days',NULL,old_digest,'frameworks')
      ON CONFLICT (country,db_name) DO UPDATE SET retired_at=EXCLUDED.retired_at,
        retain_until=EXCLUDED.retain_until,dropped_at=NULL,dump_sha256=EXCLUDED.dump_sha256,
        corpus_kind=EXCLUDED.corpus_kind;
  END IF;
  {retained}
END;
$activation$;
{journal(row, 'activated')}
COMMIT;
"""


def protected(name):
    target = literal(name)
    return f"""SELECT EXISTS(SELECT FROM frameworks_release WHERE active_db={target})
OR EXISTS(SELECT FROM retired_release WHERE corpus_kind='frameworks' AND db_name={target} AND retain_until > now())
OR EXISTS(SELECT FROM (
  SELECT DISTINCT ON (operation_id) * FROM release_journal
  WHERE corpus_kind='frameworks' AND operation_id IS NOT NULL ORDER BY operation_id,id DESC
) j WHERE database_name={target} AND mode <> 'prune'
  AND step NOT IN ('activated','reverted','aborted','pruned'));
"""
