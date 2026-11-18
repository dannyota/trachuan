from . import deploy_corpus_sql as law
from .deploy_corpus_sql import CATALOG_PATH, SCHEMA_PATH, literal, plain


KIND = 'frameworks'
COUNTRY = 'vn'
RELEASE = r'[0-9]{8}(?:_[0-9]{1,2})?'
LOG_SETTINGS = (('log_min_messages', 'panic'), ('log_min_error_statement', 'panic'),
                ('log_min_duration_statement', '-1'), ('log_statement', 'none'))
LOG_OPTIONS = ' '.join('-c ' + name + '=' + value for name, value in LOG_SETTINGS)
EFFECTIVE = ','.join(value for _, value in LOG_SETTINGS)
ADMIN_OPTIONS = LOG_OPTIONS + ' ' + CATALOG_PATH


def database(name):
    if not law.shaped('frameworks_' + RELEASE, name):
        raise ValueError('invalid frameworks database name')
    return '"' + name + '"'


def journal_row(row):
    return law.journal_row(row, database, COUNTRY, RELEASE)


def activation_values(row):
    return law.activation_values(row, database, COUNTRY)


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
    return ('SELECT EXISTS(SELECT FROM pg_catalog.pg_parameter_acl a CROSS JOIN pg_catalog.aclexplode(a.paracl) x '
            'LEFT JOIN pg_catalog.pg_roles r ON r.oid=x.grantee WHERE a.parname=ANY(' + parameters + ") "
            "AND x.privilege_type='SET' AND r.rolsuper IS NOT TRUE) "
            'OR EXISTS(SELECT FROM pg_catalog.pg_roles r CROSS JOIN pg_catalog.unnest(' + parameters + ') p '
            "WHERE NOT r.rolsuper AND pg_catalog.has_parameter_privilege(r.oid,p,'SET')) "
            'AS frameworks_log_granted \\gset\n' + law.SUPERUSER_CHECK +
            '\\if :release_superuser\n'
            "SELECT 'superuser';\n"
            '\\elif :frameworks_log_granted\n'
            "SELECT 'granted';\n"
            '\\else\n'
            'BEGIN;\n'
            'REVOKE ALL ON DATABASE ' + target + ' FROM PUBLIC;\n' + settings + law.create_extensions(extensions) +
            'COMMIT;\n'
            "SELECT 'configured';\n"
            '\\endif\n')


def apply(schema):
    return 'BEGIN;\n' + guard() + 'SET LOCAL search_path = public;\n' + schema + '\nCOMMIT;\n'


def grants(name, reader='corpus_reader'):
    return law.owner_grants(database(name), reader, guard())


def release_row():
    return guard() + ("SELECT pg_catalog.json_build_object('kind',kind,'rel',rel,'schema_version',schema_version,"
                      "'counts',pg_catalog.json_build_object('versions',version_count,'controls',control_count),"
                      "'recipes',pg_catalog.json_build_object('dense',dense_recipe,'chunk',chunk_recipe,"
                      "'lexical',lexical_recipe,'retrieval',retrieval_recipe)) FROM public.release WHERE singleton;")


def restored():
    return guard() + "SELECT pg_catalog.to_regclass('public.release') IS NOT NULL;"


def current(country):
    return 'SELECT active_db FROM public.frameworks_release WHERE country=' + literal(country) + ';'


def operations(own=None):
    return law.operations(KIND, own)


def forget(name):
    return law.forget(name, KIND)


def journal(row, step):
    return law.journal(row, step, KIND, KIND)


def close_open(keep=None):
    return law.close_open(keep, KIND, KIND)


def activate(row, retire=None):
    country, target, expected = activation_values(row)
    retained = ("DELETE FROM public.retired_release WHERE corpus_kind='frameworks' AND country=" + country +
                ' AND db_name=' + target + ';' if row['mode'] == 'retained' else '')
    retirement = law.retirement(country, retire, database, ',corpus_kind', ",'frameworks'")
    return f"""BEGIN;
DO $activation$
DECLARE current_db text; changed bigint;
BEGIN
  SELECT active_db INTO current_db FROM public.frameworks_release WHERE country={country} FOR UPDATE;
  IF current_db IS DISTINCT FROM {target} AND current_db IS DISTINCT FROM {expected} THEN
    RAISE EXCEPTION 'active frameworks changed';
  END IF;
  IF current_db IS NULL THEN
    INSERT INTO public.frameworks_release (country,active_db,activated_at,activated_by)
      VALUES ({country},{target},now(),'activator') ON CONFLICT (country) DO NOTHING;
  ELSE
    UPDATE public.frameworks_release SET active_db={target},activated_at=now(),activated_by='activator'
      WHERE country={country} AND (active_db={target} OR active_db IS NOT DISTINCT FROM {expected});
  END IF;
  GET DIAGNOSTICS changed = ROW_COUNT;
  IF changed <> 1 THEN
    RAISE EXCEPTION 'active frameworks update must affect exactly one row';
  END IF;
  {retirement}
  {retained}
END;
$activation$;
{journal(row, 'activated')}
COMMIT;
"""
