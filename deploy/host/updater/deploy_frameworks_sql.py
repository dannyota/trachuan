import re

from . import deploy_corpus_sql as law
from .deploy_corpus_sql import literal


KIND = 'frameworks'


def database(name):
    if not isinstance(name, str) or not re.fullmatch(r'frameworks_[0-9]{8}(?:_[0-9]{1,2})?', name):
        raise ValueError('invalid frameworks database name')
    return '"' + name + '"'


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
