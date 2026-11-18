import re

from . import deploy_corpus_sql as law
from .deploy_corpus_sql import literal


KIND = 'frameworks'
LOG_SETTINGS = (('log_min_messages', 'panic'), ('log_min_error_statement', 'panic'),
                ('log_min_duration_statement', '-1'), ('log_statement', 'none'))
LOG_OPTIONS = ' '.join('-c ' + name + '=' + value for name, value in LOG_SETTINGS)
EFFECTIVE = ','.join(value for _, value in LOG_SETTINGS)
ADMIN_OPTIONS = LOG_OPTIONS + ' -c search_path=pg_catalog'
CATALOG_PATH = '-c search_path=pg_catalog'
SCHEMA_PATH = '-c search_path=pg_catalog,public'


def database(name):
    if not isinstance(name, str) or not re.fullmatch(r'frameworks_[0-9]{8}(?:_[0-9]{1,2})?', name):
        raise ValueError('invalid frameworks database name')
    return '"' + name + '"'


def plain(name):
    if not isinstance(name, str) or not re.fullmatch('[a-z_][a-z0-9_]{0,62}', name):
        raise ValueError('invalid frameworks role or extension name')
    return name


def effective():
    return ("SELECT pg_catalog.concat_ws(','," +
            ','.join("pg_catalog.current_setting('" + name + "')" for name, _ in LOG_SETTINGS) + ');')


def guard():
    differs = ' OR '.join("pg_catalog.current_setting('" + name + "') OPERATOR(pg_catalog.<>) '" + value + "'"
                          for name, value in LOG_SETTINGS)
    return ('DO $guard$ BEGIN IF ' + differs +
            " THEN RAISE EXCEPTION 'frameworks log settings refused'; END IF; END $guard$;\n")


def configure(name, extensions, activator='activator', reader='corpus_reader'):
    target = database(name)
    scopes = ['DATABASE ' + target] + ['ROLE "' + plain(role) + '" IN DATABASE ' + target for role in (activator, reader)]
    parameters = 'ARRAY[' + ','.join(literal(setting) for setting, _ in LOG_SETTINGS) + ']'
    settings = ''.join('ALTER ' + scope + ' SET ' + setting + ' = ' + literal(value) + ';\n'
                       for scope in scopes for setting, value in LOG_SETTINGS)
    created = ''.join('CREATE EXTENSION IF NOT EXISTS ' + plain(extension) + ' WITH SCHEMA public VERSION ' +
                      literal(version) + ';\n' for extension, version in extensions.items())
    return ('SELECT EXISTS(SELECT FROM pg_catalog.pg_parameter_acl a CROSS JOIN pg_catalog.aclexplode(a.paracl) x '
            'LEFT JOIN pg_catalog.pg_roles r ON r.oid=x.grantee WHERE a.parname=ANY(' + parameters + ") "
            "AND x.privilege_type='SET' AND r.rolsuper IS NOT TRUE) "
            'OR EXISTS(SELECT FROM pg_catalog.pg_roles r CROSS JOIN pg_catalog.unnest(' + parameters + ') p '
            "WHERE NOT r.rolsuper AND pg_catalog.has_parameter_privilege(r.oid,p,'SET')) "
            'AS frameworks_log_granted \\gset\n'
            '\\if :frameworks_log_granted\n'
            "SELECT 'granted';\n"
            '\\else\n'
            'BEGIN;\n'
            'REVOKE ALL ON DATABASE ' + target + ' FROM PUBLIC;\n'
            'GRANT CONNECT ON DATABASE ' + target + ' TO "' + plain(reader) + '";\n' + settings + created +
            'COMMIT;\n'
            "SELECT 'configured';\n"
            '\\endif\n')


def apply(schema):
    return 'BEGIN;\n' + guard() + 'SET LOCAL search_path = public;\n' + schema + '\nCOMMIT;\n'


def grants(reader='corpus_reader'):
    role = '"' + plain(reader) + '"'
    return ('BEGIN;\n' + guard() + 'REVOKE CREATE ON SCHEMA public FROM PUBLIC;\n'
            'GRANT USAGE ON SCHEMA public TO ' + role + ';\n'
            'GRANT SELECT ON ALL TABLES IN SCHEMA public TO ' + role + ';\n'
            'GRANT SELECT ON ALL SEQUENCES IN SCHEMA public TO ' + role + ';\n'
            'COMMIT;\n')


def release_row():
    return guard() + ("SELECT pg_catalog.json_build_object('kind',kind,'rel',rel,'schema_version',schema_version,"
                      "'counts',pg_catalog.json_build_object('versions',version_count,'controls',control_count),"
                      "'recipes',pg_catalog.json_build_object('dense',dense_recipe,'chunk',chunk_recipe,"
                      "'lexical',lexical_recipe,'retrieval',retrieval_recipe)) FROM public.release WHERE singleton;")


def restored():
    return guard() + "SELECT pg_catalog.to_regclass('public.release') IS NOT NULL;"


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
