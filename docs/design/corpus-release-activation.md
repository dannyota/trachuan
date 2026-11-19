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
- Nothing a dump left behind reaches the next activation. Before any operation
  begins, before a target is created or a `retained` database is reactivated,
  and after an operation ends, the updater ends every other `activator` backend
  and drops every `activator`-owned database that no kind protects, whatever its
  name; one it cannot clear makes the phase wait. It keeps a new target's
  `pg_database` OID in the [activation record](activation-record.md), and every
  later step, the `postgres` one first, refuses a database with another OID
  under that name.
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
  another role on the database, its relations or its schema and no
  default-privilege row. A `retained` database is checked with its reader grants
  allowed. Any difference aborts the activation, naming only the kind of object.
- The updater feeds the provisioner from the activated bundle over stdin,
  because a running container keeps the old file behind its bind mount. Before
  each activation its role statements run as `postgres` in database `postgres`,
  resetting `activator`'s role-wide settings and its settings in every database
  but the frameworks ones, whose four log settings the
  [frameworks check](frameworks-release-restore.md) verifies.

Before an operation starts, the corpus phase refuses to proceed while a role
other than `postgres` is a superuser or a member of one: the result is
`waiting`, so no manifest is spent. The same check runs again in the `postgres`
step.

The [manual restore](../runbooks/corpus-restore-manual.md) is for a dump the
owner already trusts and is outside these rules.

## The updater and `auth`

`auth` belongs to `auth_owner`, the role the internet-facing server connects as
([database roles](system.md#database-roles)), so its tables, and anything that
fires on them, are the server's. The updater works from that assumption.

**Sessions.** It reads and writes the release tables (`corpus_release`,
`frameworks_release`, `release_journal`, `retired_release`) as `auth_owner`, so
a planted trigger, view or policy runs with rights the server already has. The
pointer swap of step 4, with its retirement and journal rows, is one transaction
there. These sessions send SQL on stdin, bound their output, and fix in the
startup options `search_path`, statement, lock and idle-transaction timeouts,
`default_transaction_read_only`, `standard_conforming_strings` and
`client_encoding`, so a planted function or a database default can delay a step
but not hold the updater's locks. In `auth`, `activator` only holds the
`activation` advisory lock, with `idle_session_timeout` off in its startup
options; its catalog reads, the ending of other backends and its drops run from
database `postgres`, which the server does not own. No `activator` statement in
`auth` names an object outside `pg_catalog`, and `search_path` is `pg_catalog`
from startup: that is the control. A lock that another role holds, or a refused
connection to `auth`, makes the phase wait and raises an
[alert](activation-record.md#alerts); a lock still held by a leftover
`activator` session only makes it wait. A refused or lost `activator` session to
`postgres` or `auth` also waits; a failed session to the target database or a
failed `pg_restore` aborts, because a signed dump can limit connections to its
own database and would otherwise stall the phase forever. Its table, sequence
and schema rights are revoked as well, so an honest missed path fails at once,
but `auth_owner` can grant them back and nothing relies on the revoke.

**The host holds the operation.** Commands are built from the
[activation record](activation-record.md) and the manifest verified on the host.
`release_journal` is a published copy. Of the tables, only the current pointer
and protection still count, and neither chooses what is activated: that is the
newest manifest the host verified and accepted. A table that disagrees or cannot
be read makes the phase wait without spending the manifest, as does a busy
advisory lock. A server that locks the updater out of `auth` (its own password,
`ALTER DATABASE auth` limits or defaults) stops deploys and raises the alarm;
the owner [repairs it](../runbooks/auth-access.md).

**Rollout.** The new updater passes `--no-auth-tables` on every provisioner
call, and the installed script then revokes the rights; without the flag the
script grants them as before. This release has no auth migration, because one
would block rollback.

| Updater                                 | Script it runs                         | Result  |
| --------------------------------------- | -------------------------------------- | ------- |
| before PR #425                          | the container's mounted copy, no flag  | grants  |
| from #425                               | the installed file over stdin, no flag | grants  |
| this one                                | installed new script, flag             | revokes |
| this one, recovering to an older bundle | installed old script, flag ignored     | grants  |

- **The deploy that ships this.** The old process installs the bundle,
  provisions without the flag and finishes its run with its rights. The next run
  to reach a corpus phase is the new updater, which revokes them.
- **A crash after install.** The next run is new code, recovers to the old
  bundle and its script grants, so the run after, old code, has its rights.
- **A rollback.** The older updater provisions before its first `activator`
  session, which restores the rights. It reads the journal as before, and the
  new updater keeps writing every row, so it sees complete history. Rolling
  forward revokes again.
- **The lock** is the same key in the same database in every version, so two
  versions never overlap. The release after this one raises `release_floor` to
  this sequence and deletes the grant branch, which ends rollback across it.

## Steps

The host activator lives under `deploy/host/updater/` and works through
`podman exec` into `trachuan-postgres`. Privileged preparation runs as
`postgres` through `postgres-admin.sh`; work on corpus databases runs as
`activator` (ADR 0003; [system](system.md#database-roles)), which has
`CREATEDB`, owns the corpus databases it creates, and in `auth` has only what
[the updater and `auth`](#the-updater-and-auth) leaves it. Before its first
activator connection, the corpus phase requires the installed
`provision-activator.sh` on the host, and `postgres-admin.sh` and
`/run/secrets/activator_db_password` readable in the running PostgreSQL
container; otherwise it waits with reason `activator not provisioned`. It runs
the idempotent provisioner before connecting and before each activation: as
`postgres` in database `postgres` it creates the role with `CREATEDB` when
missing and sets its password from the Vault secret `activator_db_password`; as
`auth_owner` it sets the role's rights in `auth` as
[above](#the-updater-and-auth), even when the auth migrations ran before the
role existed.

0. Take the `activation` advisory lock in `auth`; write the operation to the
   [activation record](activation-record.md); append `release_journal`
   (`started`, mode, target database, manifest hash, dump SHA-256, expected
   current `active_db`). Every later step does both, in that order.
1. `new` only:
   - List the TOC. Refuse functions, event triggers, and extensions other than
     `vector` and `ltree` at the versions in the manifest, which must equal the
     versions the PostgreSQL image provides.
   - As `activator`: create the database. A target that already exists when this
     step starts is dropped and created again if `activator` owns it and the
     [drop rule](activation-record.md#drops) allows it, so nothing dump SQL made
     earlier survives (a target with open connections waits); a target with
     another owner is refused and left alone.
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
   `corpus_release.active_db` from the operation's expected value, require
   exactly one updated row, insert the old database into `retired_release` with
   its dump SHA-256 and `retain_until` seven days ahead, delete the target from
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
manifest, or NULL when restored by hand; such a row stays protected for seven
days and cannot be reactivated by the pipeline, so rollback within that window
is the owner's manual compare-and-swap of `corpus_release.active_db`. A database
is protected in the tables when it is `active_db`, the target of an open
activation journal, or in `retired_release` with `retain_until` in the future.
Recovery, the daily prune and every drop follow the
[activation record](activation-record.md).
