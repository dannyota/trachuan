# Activation record

The server can write every table in `auth`
([the updater and `auth`](corpus-release-activation.md#the-updater-and-auth)),
so the host keeps its own account of what it activated and what it is doing.
Commands are built from this record and the manifest verified on the host, never
from a table row.

## The file

One file per kind, `/var/lib/trachuan-deploy/<kind>-activation.json`, beside
`state.json` and root-only like it, written atomically as canonical JSON. It is
a separate file because older updaters refuse unknown keys in `state.json`,
which stays the authority for each manifest's status.

```json
{
  "version": 1,
  "active": {
    "database": "corpus_vn_20261118_1",
    "oid": 41001,
    "manifest_hash": "<64 hex>",
    "dump_sha256": "<64 hex>"
  },
  "retired": [
    {
      "database": "corpus_vn_20261114_1",
      "oid": 39000,
      "dump_sha256": "<64 hex>",
      "retired_at": "2026-11-18T01:00:00Z"
    }
  ],
  "operation": {
    "id": "<32 hex>",
    "mode": "new",
    "database": "corpus_vn_20261120_1",
    "manifest_hash": "<64 hex>",
    "dump_sha256": "<64 hex>",
    "expected": "corpus_vn_20261118_1",
    "oid": null,
    "step": "started"
  },
  "alerts": { "pointer_changed": "2026-11-18T01:05:00Z" }
}
```

`active` and `operation` may be null. `step` is one of `started`, `creating`,
`created`, `restoring`, `restored`, `validated`, `checked`, `swapping`,
`activated` and `aborted`. The file is unreadable when it fails to parse, has
another version or key set, or holds a value of the wrong type or a database
name invalid for the kind. It replaces the `<kind>-targets/` files, which the
updater deletes.

The order for each change is: record, then tables, then `state.json`. The step
moves in the record before the matching table write, and `swapping` is written
before the pointer transaction. `swapping` has no journal row, because older
updaters refuse unknown steps; its table write is the `activated` row.

## Reconciling

At the start of each phase, under the lock, the updater brings the file into
line with `state.json` and the catalog. These reads cannot be forged by the
server.

1. A missing or unreadable file is rebuilt, with a warning when it was
   unreadable: `active` from the host's active manifest, `retired` from its
   retained manifests, each only if its database exists and `activator` owns it,
   and no operation. A retained database another role owns, such as one restored
   by hand, gets no entry, so nothing here touches it. This is also how the
   first run after this change seeds the file.
2. When `state.json` names another active manifest, because an older updater
   activated one during a rollback, the old `active` moves to `retired` if its
   database still has that OID, and `active` is rebuilt. This rule does not
   apply while the operation is at `activated`, when the file is one write ahead
   of `state.json`.
3. An operation whose manifest is no longer pending is closed and never resumed.
4. A `retired` entry is removed when it is the file's `active`, or when it is
   younger than seven days and its database is gone or has another OID or owner.
   Older entries stay for prune.
5. An `active` whose database is gone, has another OID or is not owned by
   `activator` becomes null, with an alarm. A restore of the cluster to an
   earlier time lands here, and re-signing the release then restores it as
   `new`. `active` stays null until an adoption or activation sets it.

New entries take the time of the write as `retired_at`. In `verify` mode the
updater reconciles the file and raises [alerts](#alerts) but starts, resumes,
adopts and finishes nothing. A missing file for the other kind counts as empty;
an unreadable one is rebuilt by the running phase before any drop.

Losing all of `/var/lib/trachuan-deploy` is the
[lost-state case](pull-deployment.md#bootstrap): nothing changes until
bootstrap.

## Mode

`retained` needs a `retired` entry no older than seven days with the manifest's
database and dump digest, and that OID in the catalog; the operation starts with
that OID. Otherwise the operation is `new`. It requires `previous` to equal the
pointer and, when `active` exists, `active` too, and it needs a target that is
neither `active` nor in `retired`. When these do not hold the phase waits and
spends nothing, because a forged pointer can cause them. A rollback to a
database with no entry, such as one restored by hand, is the owner's manual
swap. Only the newest accepted manifest starts an operation.

## Recovery

The target is _as recorded_ when it exists with the recorded OID, or when no OID
is recorded yet, since a `new` operation records it after creating the database
and re-creates whatever it finds before that. A row with an operation applies
while its manifest is pending; the `none` rows apply to the newest pending
manifest:

| Step              | Target             | Pointer        | What the updater does                                          |
| ----------------- | ------------------ | -------------- | -------------------------------------------------------------- |
| before `swapping` | as recorded        | not the target | resumes from the step                                          |
| before `swapping` | missing, other OID | not the target | closes it and starts a new operation                           |
| before `swapping` | any                | the target     | adopts, below                                                  |
| `swapping`        | as recorded        | the target     | finishes                                                       |
| `swapping`        | as recorded        | `expected`     | runs the swap again, then finishes                             |
| `swapping`        | missing, other OID | `expected`     | closes it and starts a new operation                           |
| `swapping`        | missing, other OID | the target     | waits and alarms: the serving database is gone                 |
| `swapping`        | any                | anything else  | waits with `pointer changed`, alarms                           |
| `aborted`         | any                | any            | drops its target if allowed, marks the manifest failed, closes |
| `activated`       | any                | any            | settles `state.json`, closes                                   |
| none              | any                | not the target | starts a new operation                                         |
| none              | any                | the target     | adopts, below                                                  |

A lost record therefore costs a repeat restore. No row re-creates a database the
pointer names, and no row spends the manifest on what a table says.

**Finishing** is one idempotent routine. Step 4's transaction accepts a pointer
already at the target and still writes everything else: the retirement of the
previous database, the removal of a `retained` target from `retired_release`,
the summary fields and the `activated` row. The previous database is the
record's `active`, never the pointer's value; with no `active`, the table row
names `expected` with a NULL digest, as for a database restored by hand, and the
host records nothing. When `active` is already the target, as for a re-signed
active release, nothing is retired. One atomic write of the file then moves
`active` to `retired`, sets the new `active` and sets step `activated`; the
updater settles `state.json` and clears the operation.

**Adopting** covers a pointer that reached the target without a completed
record: a re-signed active release, a crash combined with a lost file, or a
forgery. The database must exist, belong to `activator`, have the recorded OID
if one exists, and pass the release row check, the catalog check and the smoke
test, all read-only. It is recorded as a `retained` operation and then finishes.
Anything else waits and alarms, and the manifest stays pending.

**Leaving a wait.** Setting the pointer back to `expected` resumes the
operation. A newer accepted manifest replaces it: the phase still fetches while
waiting, the operation is cleared and its target goes with the next sweep. The
older manifest stays pending until the newer one becomes active, as in
[acceptance](pull-deployment.md#acceptance). An owner keeping a manual pointer
first sets deploy mode to `verify`, and then either puts the pointer back or
re-signs the release it serves, which the host adopts in `activate` mode. After
a manual swap to a pipeline release, re-sign it the same day, or its `retired`
entry ages out and only the tables protect it.

## Drops

A database is dropped only when all of these hold:

- every read below succeeded;
- it is not the host's active database or in `retired`, for either kind, whether
  or not the policy has a `frameworks` block, except that prune may drop an
  entry older than seven days;
- it is not the pointer's database or otherwise protected by the tables;
- it is not the target of an open operation, except that an operation may drop
  its own target to re-create it or after aborting.

The leftover sweep applies this to `activator`-owned databases, whatever their
names. Prune handles only `retired` entries older than seven days, and drops a
database only when its OID and `activator` ownership still match, so no
`postgres` session takes part and a forged row cannot hand it a database. An
entry whose database is gone is just finished; one with another OID or owner is
removed without a drop. It keeps no operation: it journals `pruning`, drops the
database, deletes the table row, journals `pruned` and removes the entry last,
so repeating it after a crash is harmless. A database retired with no host entry
is left to the owner.

## Alerts

A wait here may be a compromise, so these do not use the 24-hour release-waiting
alarm. Each code is recorded in `alerts` with the time it was last seen and
stays in the kind's result file, as the list `integrity`, for 24 hours after
that, so a one-off event cannot be missed between runs.

| Code                    | Raised when                                                                                                        |
| ----------------------- | ------------------------------------------------------------------------------------------------------------------ |
| `pointer_changed`       | on any run the pointer differs from a non-null `active`, other than by a `swapping` operation; or Mode waits on it |
| `serving_database_gone` | the database `active` or the pointer's target names is missing or has another OID while it serves                  |
| `active_cleared`        | reconciling rule 5 cleared `active`                                                                                |
| `adoption_failed`       | a database failed an adoption check                                                                                |
| `record_rebuilt`        | an unreadable file was rebuilt; seeding a missing file raises nothing                                              |
| `journal_row_refused`   | a journal row failed its shape check, or `RETURNING` differed from the insert                                      |

The metrics timer publishes `release_integrity` with the `kind` dimension, 1
while the list is not empty. The OCI alarm `release_integrity[1m].max() > 0` is
critical, pends one minute and repeats every four hours. The Telegram watch has
signals `release_integrity` and `frameworks_integrity`, which fire on first
sight and carry only the codes. With the five-minute updater timer, both reach
the owner within about ten minutes. Ordinary waits, such as a recipe the server
does not support yet, keep the 24-hour alarm. After a rollback an older watch
discards state holding the new signals and starts fresh. The response is in the
[alarms runbook](../runbooks/alarms.md#release-integrity-alarm).

## The journal

`release_journal` is a published copy for operators and older updaters. The
updater appends the row for each step and any row the record is ahead of. An
open row with no matching operation is closed with `aborted`, by a table-only
statement, before any re-creation, because it would otherwise protect the
target. No row resumes, finishes or aborts an operation, names a database or
spends a manifest. An older updater running after a rollback trusts the journal
as it always did; the release floor ends that window.

An operation open when this change ships has no record, so it is closed and its
release restarts as a new operation.
