import json
import re


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


def journal(row, step):
    terminal = step in ('activated', 'reverted', 'aborted', 'pruned')
    fields = ('operation_id', 'country', 'rel', 'mode', 'database_name', 'manifest_hash',
              'dump_sha256', 'expected_active_db', 'manifest', 'step', 'state', 'message')
    value = dict(row, step=step, state='complete' if terminal else 'open', message='corpus ' + step)
    value['manifest'] = json.dumps(row['manifest'], separators=(',', ':')) if row['manifest'] else None
    return ('INSERT INTO release_journal (' + ','.join(fields) + ') VALUES (' +
            ','.join(literal(value[f]) for f in fields) + ') RETURNING row_to_json(release_journal);')


def activate(row):
    country, target, expected = (literal(row[k]) for k in ('country', 'database_name', 'expected_active_db'))
    retained = ('DELETE FROM retired_release WHERE country=' + country + ' AND db_name=' + target + ';'
                if row['mode'] == 'retained' else '')
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
  UPDATE corpus_release SET active_db={target},activated_at=now(),activated_by='activator'
    WHERE country={country} AND active_db IS NOT DISTINCT FROM {expected};
  GET DIAGNOSTICS changed = ROW_COUNT;
  IF changed <> 1 THEN
    RAISE EXCEPTION 'active corpus update must affect exactly one row';
  END IF;
  SELECT dump_sha256 INTO old_digest FROM release_journal
    WHERE country={country} AND database_name=previous AND step='activated'
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
OR EXISTS(SELECT FROM retired_release WHERE db_name={target} AND retain_until > now())
OR EXISTS(SELECT FROM release_journal WHERE operation_id IS NULL
  AND 'corpus_' || country || '_' || rel = {target}
  AND step NOT IN ('activated','reverted','aborted','pruned')
  AND state NOT IN ('complete','done','activated','reverted','aborted','pruned'))
OR EXISTS(SELECT FROM (
  SELECT DISTINCT ON (operation_id) * FROM release_journal
  WHERE operation_id IS NOT NULL ORDER BY operation_id,id DESC
) j WHERE database_name={target} AND mode <> 'prune'
  AND step NOT IN ('activated','reverted','aborted','pruned'));
"""
