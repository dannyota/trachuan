# Membership retention and loss

Reconcile the [support tuples](data.md#scope-workshop-contracts) before
assigning a member's disposition. [Sources](sources.md) owns selection and
relation qualification; [pipeline](pipeline.md#source-ingest-rules) owns stable
listing passes. A removed tuple is audit evidence, not active support.

## Membership predecessor

Every finalizer that reads earlier membership uses one predecessor for the run:
the most recent earlier run of the same country whose `final` phase is frozen.
Order releases by date, then sequence. If none qualifies, use the most recent
earlier selection-version-0 run through the legacy reader; otherwise use none.
Seeds-only and expanded-only snapshots never serve as membership baselines. Read
the chosen snapshot as a whole, not a separate predecessor per document.
[Source execution](source-execution.md#shared-membership-and-artifacts) owns
freezing and verifying this choice. Earlier complete catalog observations used
by the withdrawal safeguard are separate from this membership baseline.

## Supersession

For the same document, a current tuple supersedes every earlier-run tuple with
the same `(rule_kind, rule_value)`, regardless of evidence reference. Do not
record or carry the superseded tuples in the new run. Only rules without a
current tuple carry forward as retained or removed. Keep each independent
current support, including distinct response evidence for the same rule.

Reason: fresh evidence for the same rule is not an independent historical
support. Carrying superseded tuples would grow snapshots on every run. Earlier
runs keep their original evidence unchanged.

## Direct discovery

VBPL seeds come from direct tuples `agency`, `term` and `named`. Once secondary
discovery exists, `secondary_probe` also seeds a proven VBPL match under the
[hand-over contract](sources.md#secondary-discovery-and-hand-over). Relation
tuples never seed. A relation-context member from an earlier run needs current
or retained direct support to become a primary seed.

For each earlier direct tuple not superseded by a current tuple:

- Retain `agency` and `term` tuples when the central listing is incomplete.
- Retain a configured `named` tuple when that entry's lookup is incomplete. An
  ambiguous named lookup also retains its candidates' earlier named tuples.
- Retain `secondary_probe` when that probe's VBPL lookup is incomplete, like
  `named`.
- Otherwise mark the tuple removed. A different rule's incomplete observation
  does not retain this tuple. Removed named entries have no current lookup.

Migration exception: legacy sweep and keyword tuples with query identities
`vbpl:sweep@...` and `vbpl:keyword:...` from a selection-version-0 predecessor
are always marked removed in the first selection-version-1 run, because local
selection replaces them. Superseded tuples are not carried. The member-level
safeguard below still applies when the member has no current support.

Then, for a member with neither a current nor a retained tuple:

1. If present in this run's complete stable central listing, mark
   `scope_removed`.
2. If absent from both this run's and the most recent earlier complete stable
   central listings, mark `withdrawn`.
3. Otherwise, including first absence or incomplete or missing listing evidence,
   retain every earlier tuple of that member and keep it active for fetch.

A member the source list names keeps its last verified acquisition instead of
becoming `withdrawn` under [source withdrawal](source-withdrawal.md).

This member-level fallback is the exception to per-tuple removal. It protects
against a single catalog disappearance. Selection loss is not catalog absence;
losing one agency or term never removes another rule's support. Search back for
the most recent complete stable central listing when intervening observations
are incomplete.

For other queries, withdrawal requires every still-configured query that
previously listed the document to omit it in this run and its most recent
earlier complete stable listing. Incomplete observations retain prior support
for fetch. Removing all supporting queries or rules leaves `scope_removed` only
when no other support remains.

## Relation loss

For context-only membership, keep the target while any active seed points to it.
If every supporting seed leaves scope, record `scope_removed`. Otherwise
consider only currently active supporting seeds: when all have complete
observations and none retains the link, record `withdrawn`. Both are expected
gap items without fetch for the removal run.

A failed or incomplete observation retains that active seed's known targets with
their original evidence. A new failed seed has unknown coverage. A departed seed
cannot prevent removal; a current direct hit or qualifying link always retains
the document. Relation loss needs no two-observation history. Reason: protect
against failed observations without tracking departed seeds or delaying a fully
evidenced removal.

## Fetch drift

Expand freezes active membership, excluding `withdrawn` and `scope_removed`
keys. Compare each primary seed's qualified fresh fetch targets against this
frozen active membership. An absent target, including a retained removal key,
causes non-retryable `scope_changed` failure with seed, target, reason and both
evidence hashes (record no expand hash when its observation was incomplete).
Persist it as a run failure, never an acceptable gap. Start, gap acceptance and
release cannot clear it; a new run is required. Check during run-specific fetch
finalization, never inside cached relation artifacts. Context documents'
second-hop edges remain `unknown_target` gaps without expansion. Reason: the
fetch inventory must not conceal an unfetched target or allow accepted drift.

Keep this failure rule when the seed's expand observation was incomplete,
including a transient expand failure on a new seed followed by successful fetch.
If all fresh qualified targets are already active, incompleteness alone is not
drift; the original source gap remains. Otherwise a new run must re-expand and
plan the missing target before fetch. Fetch never adds it silently or treats its
discovery as an acceptable source gap.

Reason: incomplete expansion cannot prove when an unseen link appeared, and
frozen membership must bound the fetch inventory. Deferring close with a new
retryable expand gap would need a separate protocol to block acceptance and
atomically invalidate downstream work; do not add that exception. The cost is
repeating discovery and downloads in a new run after a transient failure. The
existing retry path can rewind eligible source gaps before drift is committed,
but cannot erase a committed `scope_changed` failure.

## Removal lifetime

Record removal when a member loses its last current or retained support. Carry
the removed-only member and its removed tuples as an expected `withdrawn` or
`scope_removed` gap item until that removal gap was accepted in an earlier run
that recorded or carried it. Acceptance ends the carry: only then may a later
run drop the member and its removed tuples. Until acceptance, the unaccepted
loss stays visible to every later release gate.

Apply this rule before direct retention and relation reconciliation. An
incomplete observation cannot revive a removed-only member as active support.

Do not search older snapshots for excluded members or carry their old supports
back. A fresh direct hit or qualified relation can admit the key again with
current evidence. A later loss after readmission is a new removal event and
needs its own acceptance; acceptance before that readmission does not end the
new carry. Members with active support still carry independent unsuperseded
audit tuples. This lifetime rule concerns `withdrawn` and `scope_removed`;
[source hand-over](sources.md#secondary-discovery-and-hand-over) owns
`source_replaced`.

Every run that recorded or carried the removal keeps its evidence and acceptance
record unchanged. Deliberate scope exclusions still need the owner's
[loss approval](sources.md#local-terms-and-loss-gate). Normal source errors and
named-entry gaps in a later run remain subject to that run's acceptance gate.

Reason: removal records a change in coverage, not a permanent fetch obligation.
Repeating an accepted loss requires acceptance every run without adding
evidence. Keeping unresolved losses visible prevents a new run from bypassing
acceptance; keeping earlier audit records preserves the history after
acceptance. The trade-off is that later inventories alone do not enumerate
historical losses; auditors must follow earlier runs.

Adopt this rule only in new runs with bumped discover and expand recipes after
independent design review and implementation checks. Do not rewrite frozen
snapshots, old releases, artifacts or acceptance records. Existing removed-only
entries in the selected predecessor follow the same acceptance rule. This needs
no new service or network request; bounds remain in
[budgets](budgets.md#pipeline). Owner approval remains required for deliberate
coverage losses and unapproved ceilings.
