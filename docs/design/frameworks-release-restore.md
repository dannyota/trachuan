# Control frameworks: restore

The host restore of a frameworks release, between creating `frameworks_<rel>`
and the pointer swap of [frameworks release](frameworks-release.md#host). It
follows the law [activation steps](corpus-release-activation.md#steps) except
where this file says otherwise.

## Trust in the dump

`pg_restore` runs the SQL stored in the archive as written, whatever the TOC
list keeps, so every statement in a frameworks dump runs with `activator`'s
privileges. The signed manifest is the boundary; the rules below keep that SQL
from reaching more privilege or changing what other sessions run:

- Every `postgres` step (refusing any superuser but `postgres` or a member of
  one, log settings, extensions, revoking `PUBLIC`'s rights on the database)
  runs before the first byte of the dump. Once the restore starts, no session
  with more privilege than `activator` connects to the database: the owner
  grants the corpus reader, and there is no prewarm (the engine loads its
  partitions when it opens the release).
- Every updater session in the restored database starts with `search_path` set
  to `pg_catalog` first (`pg_catalog, public` where the image's SQL names its
  tables), so no planted object shadows a built-in, and the check names catalog
  objects with their schema.
- After the image's executable SQL is applied, one check compares the database
  with the image's schema version: schemas (only `public`), extensions (exactly
  `vector` and `ltree`, owned by `postgres`), views, functions, procedures,
  aggregates, triggers, rules, policies, operators, casts, types and domains,
  and the database's role settings (exactly the four log settings, on the
  database and for `activator` and `corpus_reader`). Any difference fails the
  activation, naming only the kind of object.
- `activator` owns the law databases too, so a hostile signed dump can still act
  on them (settings, connection limits, dropping retired ones). This is accepted
  as it is for law dumps, since signing is the boundary for both.

## Logs

No session in a frameworks database may write an error, statement or parameter
to the PostgreSQL log. Right after creating the database, as `postgres`, set
`log_min_messages` and `log_min_error_statement` to `panic`,
`log_min_duration_statement` to `-1` and `log_statement` to `none` on the
database and, with `ALTER ROLE ... IN DATABASE`, for `activator` and
`corpus_reader` in it, which takes precedence over any role-wide setting. The
same step refuses when `pg_parameter_acl` grants `SET` on any of them to a role
other than a superuser; without that grant no other role can change them in a
session. Every session that reads a frameworks database (restore, smoke, and
each connection the server's frameworks pool opens) reads the four effective
values first and stops, or is discarded, unless they are those. Superusers are
the trusted boundary: the host's `postgres` role is the only one, and its own
sessions set the values before touching the database. The updater and the server
log only safe causes (SQLSTATE, step, object names).

## Restore

The TOC list keeps the law allowlist and also omits every view, function and
trigger entry, refusing the dump when such an entry is not one the image's
schema version defines, any other executable object (rule, policy, aggregate,
operator, cast, event trigger) and any schema but `public`. This shapes an
honest dump; it is not the control for a hostile one (above). After the data,
`server frameworks-schema --version <n>` prints the image's own views, functions
and triggers for that schema version, applied as `activator`; a check constraint
or default that needs a function the dump omitted fails the restore. Then the
check above, the owner's grants to the corpus reader, and validation of the
`release` row (`kind`, `rel`, schema version, recipes, version and control
counts) against the manifest.

## Smoke

`server release-check --kind frameworks --database frameworks_<rel>` opens the
frameworks engine with a key made in memory for that run and runs one search and
one read as a caller who may read every version, printing status and counts
only. Only a stderr line naming one of release-check's fixed steps and a
SQLSTATE or Go type is kept as the cause.
