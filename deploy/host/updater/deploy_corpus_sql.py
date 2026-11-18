import json
import re

from .deploy_corpus_heading import headings_approved


def literal(value):
    if value is None:
        return 'NULL'
    if not isinstance(value, str) or '\0' in value:
        raise ValueError('invalid SQL literal')
    return "'" + value.replace("'", "''") + "'"


def database(name):
    if not isinstance(name, str) or not re.fullmatch(r'corpus_[a-z]{2}_[0-9]{8}(?:_[1-9][0-9]*)?', name):
        raise ValueError('invalid corpus database name')
    return '"' + name + '"'


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
