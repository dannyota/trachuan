# ADR 0016: Control frameworks beside the law corpus, licensed text by allowlist

Status: proposed 2026-11-15.

## Context

trachuan serves Vietnamese law. People who work with that law also work with
voluntary and contractual control frameworks: NIST, CIS, CSA, PCI DSS, the ISO
family, the AICPA criteria, COBIT and Vietnamese national standards (TCVN).
compliary, the owner's earlier project (archived 2026-11-01), held 13 framework
versions with 3,943 controls and 4,501 mapping edges and served them over MCP.
Its documents live in a private repository, `compliary-private`, because most of
them are bought or licensed for internal use.

On 2026-11-15 the owner decided to build that part inside trachuan: anyone with
an account searches the freely usable knowledge, and licensed text is served
only to accounts on an allowlist. The allowlist is server data, not a Vault
secret. An admin portal behind Cloudflare One comes later. The feature is in
development, so the review of each publisher's licence terms waits.

compliary had no per-account access: one operator password opened everything,
and its per-document gate was stored but not enforced when serving
([survey](../research/compliary-survey-2026-11.md)).

## Decision

1. **A second corpus kind.** Frameworks are a corpus kind of their own,
   `frameworks`, with no country: its release database, schema, release channel
   and evaluation are separate from `corpus_<cc>_<rel>`. The law dump therefore
   never holds licensed text and stays handable to other projects.
2. **Data from compliary, not its code.** trachuan reuses compliary's registry
   seeds, curated mapping edges, paraphrased titles, golden cases and recorded
   lessons. Parsers are written for trachuan's pipeline; a compliary parser may
   be ported with its tests when the owner owns both sides.
3. **Every framework version has an access class.** `free`: text served to every
   signed-in account. `licensed`: text served only under an entitlement. A new
   version is `licensed` until a reviewed registry entry says `free`, so
   forgetting to classify fails closed.
4. **Licensed text is served by entitlement.** An entitlement names a verified
   email and a scope (every licensed framework, or one framework). It is a row
   in the auth database, written by a server admin command and later by the
   admin portal, with an audit event in the same transaction. It belongs to the
   email, not to an account or an OAuth grant. The check runs on every request,
   in one place, before any cache; a revocation takes effect within the session
   revocation delay; a restore from backup revokes every entitlement until the
   operator allows it again.
5. **Without an entitlement a licensed control is a reduced record.** The caller
   gets the framework, version, citation, trachuan's own paraphrased title,
   mapping edges and version lineage; never the publisher's title or body text.
   One invariant governs search, read, mappings and caches: text a caller may
   not read cannot change anything that caller observes.
6. **Licensed material has a closed list of places.** Source files and every
   derivative (extracted and OCR text, check results, reviewer readings,
   indexes, dumps) exist only in the places the
   [access note](../design/frameworks-access.md#licensed-material-and-where-it-may-be)
   lists; that table is the only list. One publication exception: the signed
   manifest's integrity values (dump digest and size, report and build artifact
   digests), which show that a release changed and nothing about what. Tests use
   synthetic documents only.
7. **Scans use the OCR path, as an exception to ADR 0014.** A source without a
   text layer goes through the Vision OCR and cross-check of
   [ADR 0014](0014-ocr-on-the-write-path.md), which otherwise sends only public
   legal text to the providers. The owner decided on 2026-11-15 to OCR his
   licensed scans this way. A scan is sent only after the providers' data-use
   terms are recorded in the registry; reviewer readings stay outside Git.
8. **One served model.** Frameworks use the law corpus's dense recipe and the
   one query embedder on the host. A different recipe needs measured evidence
   that this one misses the floors on the framework golden cases.
9. **Admin portal later, on its own boundary.** It will be a separate hostname
   behind Cloudflare Access, the server verifying the Access token; it adds no
   capability to the public site or to `/mcp`.
10. **Licence terms are deferred, exposure is not.** During development every
    held document may be ingested. Until the terms review lands, only the
    owner's email holds an entitlement and no framework is `free` except those
    whose publisher states public-domain or open terms in the registry.

## Consequences

- The server resolves two kinds of release and keeps two engine sets; the host
  memory budget grows by the framework index.
- Access becomes data that changes at run time, so entitlements need audit rows
  and the admin command needs the same care as account blocking.
- The signed-manifest and private-bucket path carries the framework release. Its
  full dump is licensed material and has no hand-over path; a reduced export
  (public fields, free text, edges, lineage) and a change list per release are
  the hand-over form for the owner's other products, which keep their private
  mappings keyed by this corpus's identities.
- Links from controls to Vietnamese legal provisions become possible in one
  product, as reviewed mapping edges; the plan carries them after serving.

## Alternatives

- **Revive compliary as its own service.** Rejected: a second host, login,
  deploy path and embedder for one owner, and no account model to build on.
- **Frameworks as a pseudo-country in the law corpus.** Rejected: one dump would
  mix licensed text with public law, and document, validity and taxonomy rules
  for law do not fit controls.
- **Allowlist in Vault, like the retired sign-up list.** Rejected by the owner:
  entitlements change often, need audit and an admin interface, and are not
  secrets.
- **Hide licensed frameworks entirely from accounts without an entitlement.**
  Rejected: citations, mapping edges and our own titles carry no licensed text
  and are what makes the free part useful.

## History

- 2026-11-15: proposed after the owner's decision and the survey of compliary
  and `compliary-private`.
- 2026-11-15: revised after independent review (A-01 to A-03, S-01 to S-04):
  entitlements instead of grants, the access invariant, the closed list of
  places for licensed material, corpus kind in release identity, restore rule.
- 2026-11-15: second review round (A-04, S-05, S-06): text-dependent diagnostics
  withheld from callers without access, complete public manifest field list,
  host staging and server memory in the list of places. Added the downstream
  contract after the owner named sc365's private asset mappings.
- 2026-11-15: third review round (S-05 to S-07): the manifest's integrity values
  named as the one publication exception, the access note's table as the only
  list of places, a broken link.
