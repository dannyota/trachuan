import json
import re

from .deploy_corpus_heading import headings_approved


def literal(value):
    if value is None:
        return 'NULL'
    if not isinstance(value, str) or '\0' in value:
        raise ValueError('invalid SQL literal')
    return "'" + value.replace("'", "''") + "'"


def plain(name):
    if not isinstance(name, str) or not re.fullmatch('[a-z_][a-z0-9_]{0,62}', name):
        raise ValueError('invalid role or extension name')
    return name


def database(name):
    if not isinstance(name, str) or not re.fullmatch(r'corpus_[a-z]{2}_[0-9]{8}(?:_[1-9][0-9]*)?', name):
        raise ValueError('invalid corpus database name')
    return '"' + name + '"'


CATALOG_PATH = '-c search_path=pg_catalog'
SCHEMA_PATH = '-c search_path=pg_catalog,public'
SUPERUSER_REFUSED = 'a role other than postgres is a superuser or a member of one'
SUPERUSER_ROLES = ('SELECT EXISTS(SELECT FROM pg_catalog.pg_roles r WHERE r.rolname <> {owner} AND (r.rolsuper OR '
                   'EXISTS(SELECT FROM pg_catalog.pg_roles s WHERE s.rolsuper AND '
                   "pg_catalog.pg_has_role(r.oid,s.oid,'MEMBER'))))")
SUPERUSER_CHECK = SUPERUSER_ROLES.format(owner='current_user') + ' AS release_superuser \\gset\n'
RESTORED = "SELECT pg_catalog.to_regclass('public.release') IS NOT NULL;"
RELEASE_ROW = ("SELECT pg_catalog.json_build_object('country',country,'rel',rel,'schema_version',schema_version,"
               "'recipes',pg_catalog.json_build_object('dense',dense_recipe,'chunk',chunk_recipe,"
               "'lexical',lexical_recipe,'retrieval',retrieval_recipe)) FROM public.release WHERE singleton;")
SUMMARY_TABLE = "SELECT pg_catalog.to_regclass('public.document_summary') IS NOT NULL;"
SUMMARY_PROJECTED = 'SELECT NOT EXISTS(SELECT FROM public.document_summary WHERE projection <> 1);'
PREWARM_SKIPPED = 'skipped'
PREWARM = ('SET statement_timeout=0;\nSET lock_timeout=0;\nSET idle_in_transaction_session_timeout=0;\n'
           'SET transaction_timeout=0;\n'
           'SELECT EXISTS(SELECT FROM pg_catalog.pg_proc p JOIN pg_catalog.pg_namespace n ON n.oid=p.pronamespace '
           "JOIN pg_catalog.pg_depend d ON d.classid='pg_catalog.pg_proc'::pg_catalog.regclass AND d.objid=p.oid "
           "AND d.deptype='e' JOIN pg_catalog.pg_extension x ON x.oid=d.refobjid "
           "JOIN pg_catalog.pg_roles r ON r.oid=x.extowner WHERE n.nspname='public' AND p.proname='pg_prewarm' "
           "AND x.extname='pg_prewarm' AND r.rolsuper) AS corpus_prewarm \\gset\n"
           '\\if :corpus_prewarm\n'
           'SELECT pg_catalog.sum(public.pg_prewarm(c.oid::pg_catalog.regclass)) FROM pg_catalog.pg_class c '
           'JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace '
           "WHERE n.nspname IN ('public','pg_toast') AND c.relkind IN ('r','i','t') "
           "AND pg_catalog.has_table_privilege(c.oid,'SELECT');\n"
           '\\else\n'
           "SELECT '" + PREWARM_SKIPPED + "';\n"
           '\\endif\n')


def other_superuser(owner='postgres'):
    return SUPERUSER_ROLES.format(owner=literal(owner)) + ';'


def database_owner(name):
    return ('SELECT pg_catalog.pg_get_userbyid(datdba) FROM pg_catalog.pg_database WHERE datname=' +
            literal(name) + ';')


def create_extensions(extensions):
    return ''.join('CREATE EXTENSION IF NOT EXISTS ' + plain(extension) + ' WITH SCHEMA public VERSION ' +
                   literal(version) + ';\n' for extension, version in extensions.items())


def prepare(name, extensions):
    return (SUPERUSER_CHECK +
            '\\if :release_superuser\n'
            "SELECT 'superuser';\n"
            '\\else\n'
            'BEGIN;\n'
            'REVOKE ALL ON DATABASE ' + database(name) + ' FROM PUBLIC;\n' + create_extensions(extensions) +
            'CREATE EXTENSION IF NOT EXISTS pg_prewarm WITH SCHEMA public;\n'
            'COMMIT;\n'
            "SELECT 'configured';\n"
            '\\endif\n')


def owner_grants(target, reader, guard=''):
    role = '"' + plain(reader) + '"'
    return ('BEGIN;\n' + guard + 'REVOKE ALL ON DATABASE ' + target + ' FROM PUBLIC;\n'
            'GRANT CONNECT ON DATABASE ' + target + ' TO ' + role + ';\n'
            'REVOKE CREATE ON SCHEMA public FROM PUBLIC;\n'
            'GRANT USAGE ON SCHEMA public TO ' + role + ';\n'
            'GRANT SELECT ON ALL TABLES IN SCHEMA public TO ' + role + ';\n'
            'GRANT SELECT ON ALL SEQUENCES IN SCHEMA public TO ' + role + ';\n'
            'COMMIT;\n')


def grants(name, reader='corpus_reader'):
    return owner_grants(database(name), reader)


def kind(name):
    return 'corpus_kind=' + literal(name)


def current(country):
    return 'SELECT active_db FROM corpus_release WHERE country=' + literal(country) + ';'


def retirement(name, corpus='law'):
    return ("SELECT json_build_object('dump_sha256',dump_sha256,'live',retain_until > now()) "
            'FROM retired_release WHERE ' + kind(corpus) + ' AND db_name=' + literal(name) + ';')


def operations(corpus='law'):
    return ("SELECT coalesce(json_agg(j ORDER BY id),'[]') FROM ("
            'SELECT DISTINCT ON (operation_id) * FROM release_journal WHERE ' + kind(corpus) +
            ' AND operation_id IS NOT NULL ORDER BY operation_id,id DESC) j;')


def expired(corpus='law'):
    return ("SELECT coalesce(json_agg(db_name),'[]') FROM retired_release "
            'WHERE ' + kind(corpus) + ' AND retain_until <= now();')


def forget(name, corpus='law'):
    return 'DELETE FROM retired_release WHERE ' + kind(corpus) + ' AND db_name=' + literal(name) + ';'


def journal(row, step, corpus='law', label='corpus'):
    terminal = step in ('activated', 'reverted', 'aborted', 'pruned')
    fields = ('operation_id', 'country', 'rel', 'mode', 'database_name', 'manifest_hash',
              'dump_sha256', 'expected_active_db', 'manifest', 'step', 'state', 'message')
    value = dict(row, step=step, state='complete' if terminal else 'open', message=label + ' ' + step)
    value['manifest'] = json.dumps(row['manifest'], separators=(',', ':')) if row['manifest'] else None
    if corpus != 'law':
        fields += ('corpus_kind',)
        value['corpus_kind'] = corpus
    return ('INSERT INTO release_journal (' + ','.join(fields) + ') VALUES (' +
            ','.join(literal(value[f]) for f in fields) + ') RETURNING row_to_json(release_journal);')


def activate(row, projection=0):
    approved = 'true' if projection == 1 and headings_approved(row['manifest']) else 'false'
    country, target, expected = (literal(row[k]) for k in ('country', 'database_name', 'expected_active_db'))
    retained = ("DELETE FROM retired_release WHERE corpus_kind='law' AND country=" + country +
                ' AND db_name=' + target + ';' if row['mode'] == 'retained' else '')
    return f"""BEGIN;
DO $activation$
DECLARE previous text; old_digest text; changed bigint;
BEGIN
  SELECT active_db INTO previous FROM corpus_release WHERE country={country} FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION 'active corpus pointer missing';
  END IF;
  IF previous IS DISTINCT FROM {expected} THEN
    RAISE EXCEPTION 'active corpus changed';
  END IF;
  UPDATE corpus_release SET active_db={target},activated_at=now(),activated_by='activator',
    summary_database={target},summary_projection={projection},headings_approved={approved}
    WHERE country={country} AND active_db IS NOT DISTINCT FROM {expected};
  GET DIAGNOSTICS changed = ROW_COUNT;
  IF changed <> 1 THEN
    RAISE EXCEPTION 'active corpus update must affect exactly one row';
  END IF;
  SELECT dump_sha256 INTO old_digest FROM release_journal
    WHERE corpus_kind='law' AND country={country} AND database_name=previous AND step='activated'
    ORDER BY id DESC LIMIT 1;
  IF previous IS NOT NULL AND previous <> {target} THEN
    INSERT INTO retired_release (country,db_name,retired_at,retain_until,dropped_at,dump_sha256)
      VALUES ({country},previous,now(),now()+interval '7 days',NULL,old_digest)
      ON CONFLICT (country,db_name) DO UPDATE SET retired_at=EXCLUDED.retired_at,
        retain_until=EXCLUDED.retain_until,dropped_at=NULL,dump_sha256=EXCLUDED.dump_sha256;
  END IF;
  {retained}
END;
$activation$;
{journal(row, 'activated')}
COMMIT;
"""


def other_operation(operation):
    return '' if operation is None else ' AND operation_id <> ' + literal(operation)


def protected(name, operation=None):
    target = literal(name)
    return f"""SELECT EXISTS(SELECT FROM corpus_release WHERE active_db={target})
OR EXISTS(SELECT FROM retired_release WHERE corpus_kind='law' AND db_name={target} AND retain_until > now())
OR EXISTS(SELECT FROM release_journal WHERE corpus_kind='law' AND operation_id IS NULL
  AND 'corpus_' || country || '_' || rel = {target}
  AND step NOT IN ('activated','reverted','aborted','pruned')
  AND state NOT IN ('complete','done','activated','reverted','aborted','pruned'))
OR EXISTS(SELECT FROM (
  SELECT DISTINCT ON (operation_id) * FROM release_journal
  WHERE corpus_kind='law' AND operation_id IS NOT NULL ORDER BY operation_id,id DESC
) j WHERE database_name={target} AND mode <> 'prune'{other_operation(operation)}
  AND step NOT IN ('activated','reverted','aborted','pruned'));
"""
