# Corpus activation

The activation steps of [corpus release](corpus-release.md#host-activation) for
the law corpus. The frameworks kind follows them with the differences in
[frameworks restore](frameworks-release-restore.md).

## Trust in the dump

`pg_restore` runs the SQL stored in the archive as written, whatever the TOC
list keeps, so every statement in a dump runs with `activator`'s privileges. The
signed manifest is the boundary; these rules keep that SQL from reaching more
privilege:

- Every `postgres` step on a target runs before the first byte of its dump. Once
  the restore starts, no session with more privilege than `activator` connects
  to that database, and none connects to it later: the owner grants the corpus
  reader and runs every prewarm, including the one after a PostgreSQL restart.
  That prewarm skips a database that `activator` does not own or that has no
  `pg_prewarm` (one activated before this rule); it warms from traffic and the
  operating system's page cache until its successor is activated.
- Nothing a dump left behind reaches the next activation. Before a target is
  created, the updater ends every other `activator` backend and drops every
  `activator`-owned database that no kind protects, whatever its name. It
  records the new target's `pg_database` OID and every later step, the
  `postgres` one first, refuses a database with another OID under that name.
- Every updater session pins `search_path` at connection start (`pg_catalog`, or
  `pg_catalog, public` where its SQL names corpus tables) and names catalog
  objects with their schema.
- Before the reader gets access, and before a `retained` database becomes active
  again, a check under `search_path=pg_catalog` compares the database with what
  a law release may hold: only the `public` schema; exactly the template's
  extensions plus `vector` and `ltree`, and `pg_prewarm` if present (a database
  restored before this rule has none), all owned by `postgres`, at the
  manifest's versions for a new database (a `retained` one is compared by name,
  schema and owner, since the image may have moved on); no view, function,
  procedure, aggregate, trigger, rule, policy, operator, cast, event trigger,
  explicit type or domain outside those extensions; no `pg_db_role_setting` row
  for the database; and, before the owner's grants, no access-control entry for
  another role on the database's relations or schema and no default-privilege
  row. A `retained` database is checked with its reader grants allowed. Any
  difference aborts the activation, naming only the kind of object.
- The updater feeds the provisioner from the activated bundle over stdin,
  because a running container keeps the old file behind its bind mount. It runs
  as `postgres` before each activation and resets `activator`'s role-wide
  settings and its settings in every database but the frameworks ones, whose
  four log settings the [frameworks check](frameworks-release-restore.md)
  verifies.

Before an operation starts, the corpus phase refuses to proceed while a role
other than `postgres` is a superuser or a member of one: the result is
`waiting`, so no manifest is spent. The same check runs again in the `postgres`
step.

The [manual restore](../runbooks/release-restore.md#restore-a-corpus-manually)
is for a dump the owner already trusts and is outside these rules.

## Steps

The host activator lives under `deploy/host/updater/` and works through
`podman exec` into `trachuan-postgres`. Privileged preparation runs as
`postgres` through `postgres-admin.sh`; everything else runs as `activator` (ADR
0003; [system](system.md#database-roles)), which has `CREATEDB`, owns the corpus
databases it creates, and may write only `corpus_release`, `release_journal` and
`retired_release` in `auth`. Before its first activator connection, the corpus
phase requires both `/usr/local/bin/provision-activator.sh` and
`/run/secrets/activator_db_password` to be readable in the running PostgreSQL
container; otherwise it waits with reason `activator not provisioned`. It runs
the idempotent provisioner as `postgres` before connecting and before each
activation: it creates the role with `CREATEDB` when missing, sets its password
from the Vault secret `activator_db_password`, and reapplies the table grants
even when the auth migrations ran before the role existed.

0. Take the `activation` advisory lock in `auth`; append `release_journal`
   (`started`, mode, target database, manifest hash, dump SHA-256, expected
   current `active_db`). Every later step appends its own row.
1. `new` only:
   - List the TOC. Refuse functions, event triggers, and extensions other than
     `vector` and `ltree` at the versions in the manifest, which must equal the
     versions the PostgreSQL image provides.
   - As `activator`: create the database. A target that already exists when this
     step starts is dropped and created again if `activator` owns it and the
     protection rule below allows it, so nothing dump SQL made earlier survives
     (a target with open connections waits); a target with another owner is
     refused and left alone.
   - As `postgres`, before any byte of the dump: refuse when a role other than
     `postgres` is a superuser or a member of one, create `vector`, `ltree` and
     `pg_prewarm` in `public`, and revoke `PUBLIC`'s rights on the database.
   - As `activator`: `pg_restore --no-owner --no-privileges --exit-on-error`
     with a TOC list (`-L`) that omits every extension entry and every entry
     that belongs to an extension, including `COMMENT ON EXTENSION`, with
     `maintenance_work_mem=2GB` and two maintenance workers.
   - As `activator`: the catalog check, then the owner's grants to
     `corpus_reader` (`CONNECT`, `USAGE` on `public`, `SELECT` on its tables and
     sequences).
2. Require the restored `release` row to equal the manifest's `rel`,
   `schema_version` and recipes; otherwise abort.
3. As `activator`, prewarm the target's tables, indexes and TOAST relations with
   a schema-qualified `public.pg_prewarm`, skipping relations it may not read; a
   `retained` database without the extension is not warmed. Then
   `server release-check --database <target>` runs one search and one read
   against it with the server code that will serve it, under the evaluation
   profile: no candidate cache or postings and the 60-second deadline, since the
   smoke checks correctness, not serving latency. The restore and prewarm
   sessions set every PostgreSQL timeout to zero.
4. In one transaction: require the country's pointer row, compare-and-swap
   `corpus_release.active_db` from the journaled expected value, require exactly
   one updated row, insert the old database into `retired_release` with its dump
   SHA-256 and `retain_until` seven days ahead, delete the target from
   `retired_release` in `retained` mode, and append `activated`.
5. The server resolves the release once per request, so traffic moves without a
   restart; the postings for the new release load on first use under the
   server's existing gate ([sparse postings](sparse-postings.md)). The public
   summary pool of the new release opens and the old one retires within 5
   seconds ([serving public summaries](web-public-documents-serving.md)).
   `headings_approved` is set for the release only when the manifest's
   `heading_review` equals its `heading_evidence` field by field (projection,
   scan output hash, exclusions hash), both signed; otherwise summaries serve
   labels only.

`release_journal` and `retired_release` (with `dump_sha256`) are auth
migrations. The old active database gets its dump digest from its active corpus
manifest, or NULL when restored by hand; that row stays protected for seven days
and cannot be reactivated by the pipeline, so rollback within that window is the
owner's manual compare-and-swap of `corpus_release.active_db`. Two rules close
recovery and cleanup:

- A journal whose last row is `activated` or `reverted` is complete; the next
  run only finishes local bookkeeping.
- A database is protected when it is `active_db`, the target of an open
  activation journal, or in `retired_release` with `retain_until` in the future.
  No step, abort or prune ever drops a protected database, whatever the journal
  says.

After a crash, an open journal continues from its last step when the guard holds
(`new`: the target is neither active nor retired; `retained`: it is retired and
not active). Otherwise the journal is closed as aborted, and a `new` target that
`activator` owns is dropped subject to the rule above. A daily `prune` under the
same lock handles each unprotected database in `retired_release` without
connections: it journals `pruning`, drops the database if it exists, deletes the
row, and journals `pruned`. An open `pruning` journal is finished the same way
on the next run.
