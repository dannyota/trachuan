import json
import re

from .deploy_corpus_heading import headings_approved


COUNTRY = '[a-z]{2}'
RELEASE = r'[0-9]{8}(?:_[1-9][0-9]*)?'
OPERATION = '[0-9a-f]{32}'
DIGEST = '[0-9a-f]{64}'
MODES = ('new', 'retained', 'prune')
STEPS = ('started', 'creating', 'created', 'restoring', 'restored', 'validated', 'checked',
         'activated', 'reverted', 'aborted', 'pruning', 'pruned')
ROW_FIELDS = ('operation_id', 'country', 'rel', 'mode', 'database_name', 'manifest_hash',
              'dump_sha256', 'expected_active_db', 'manifest')
REFUSED = 'invalid release journal row'


class JournalRefused(ValueError):
    pass


def literal(value):
    if value is None:
        return 'NULL'
    if not isinstance(value, str) or '\0' in value:
        raise ValueError('invalid SQL literal')
    return "'" + value.replace("'", "''") + "'"


def dollar_body(value):
    if value is not None and (not isinstance(value, str) or '$' in value or '\\' in value):
        raise ValueError('invalid activation value')
    return literal(value)


def shaped(pattern, value):
    return isinstance(value, str) and re.fullmatch(pattern, value) is not None


def database(name):
    if not shaped('corpus_' + COUNTRY + '_' + RELEASE, name):
        raise ValueError('invalid corpus database name')
    return '"' + name + '"'


def named(check, value):
    try:
        check(value)
    except ValueError:
        return False
    return True


def journal_row(row, check=database, country=COUNTRY, release=RELEASE):
    if not isinstance(row, dict):
        raise JournalRefused(REFUSED)
    prune = row.get('mode') == 'prune'
    expected = row.get('expected_active_db')
    if not (shaped(OPERATION, row.get('operation_id')) and shaped(country, row.get('country'))
            and shaped(release, row.get('rel')) and row.get('mode') in MODES
            and named(check, row.get('database_name')) and (expected is None or named(check, expected))
            and row.get('step') in STEPS
            and all(shaped(DIGEST, row.get(field)) or (prune and row.get(field) is None)
                    for field in ('manifest_hash', 'dump_sha256'))
            and (isinstance(row.get('manifest'), dict) or (prune and row.get('manifest') is None))):
        raise JournalRefused(REFUSED)
    return row


def journal_json(raw):
    try:
        return json.loads(raw)
    except (ValueError, RecursionError):
        raise JournalRefused(REFUSED) from None


def same_row(returned, inserted):
    if any(returned.get(field) != inserted.get(field)
           for field in ROW_FIELDS + ('step', 'state', 'message', 'corpus_kind')):
        raise JournalRefused(REFUSED)


def activation_values(row, check=database, country=COUNTRY):
    if not shaped(country, row['country']):
        raise ValueError('invalid activation country')
    check(row['database_name'])
    if row['expected_active_db'] is not None:
        check(row['expected_active_db'])
    return tuple(dollar_body(row[field]) for field in ('country', 'database_name', 'expected_active_db'))


PREWARM = ('SET statement_timeout=0; SET lock_timeout=0; SET idle_in_transaction_session_timeout=0; '
           'SET transaction_timeout=0; BEGIN; CREATE EXTENSION IF NOT EXISTS pg_prewarm; '
           "SELECT sum(pg_prewarm(c.oid)) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace "
           "WHERE n.nspname IN ('public','pg_toast') AND c.relkind IN ('r','i','t'); "
           'DROP EXTENSION pg_prewarm; COMMIT;')


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


def written(step, label='corpus'):
    terminal = step in ('activated', 'reverted', 'aborted', 'pruned')
    return dict(state='complete' if terminal else 'open', message=label + ' ' + step)


def journal(row, step, corpus='law', label='corpus'):
    fields = ROW_FIELDS + ('step', 'state', 'message')
    value = dict(row, step=step, **written(step, label))
    value['manifest'] = json.dumps(row['manifest'], separators=(',', ':')) if row['manifest'] else None
    if corpus != 'law':
        fields += ('corpus_kind',)
        value['corpus_kind'] = corpus
    return ('INSERT INTO release_journal (' + ','.join(fields) + ') VALUES (' +
            ','.join(literal(value[f]) for f in fields) + ') RETURNING row_to_json(release_journal);')


def activate(row, projection=0):
    if type(projection) is not int or projection not in (0, 1):
        raise ValueError('invalid activation projection')
    approved = 'true' if projection == 1 and headings_approved(row['manifest']) else 'false'
    country, target, expected = activation_values(row)
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


def protected(name):
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
) j WHERE database_name={target} AND mode <> 'prune'
  AND step NOT IN ('activated','reverted','aborted','pruned'));
"""
