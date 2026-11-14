# Email accounts and TOTP

Decision: [ADR 0015](../adr/0015-email-password-accounts-and-totp.md). This
document is the contract for email and password accounts, the optional TOTP
second factor, recovery codes, the key ring and the mail outbox. Sessions, CSRF
and route conventions are in [web API](web-api.md); tokens and revocation in
[auth](auth.md); consent in [OAuth](oauth.md). Numbers are in
[budgets](budgets.md#accounts).

## Email address

One parser serves every path: trim, then accept only an ASCII addr-spec (local
part of 1 to 64 bytes, one `@`, a DNS domain with a dot, no quoted local part,
comment, IP literal or display name, at most 254 bytes) and lowercase the
domain. The result is the address; the local part keeps its case, because a mail
domain may treat it as case-sensitive. Lookup and uniqueness compare the address
exactly. Google sign-up stores the verified ID-token email through the same
parser and refuses one it rejects (`login_failed`). Mail for an existing account
goes only to `account.email`, never to the text a visitor typed.

## Schema

Changes to `account`: `google_sub` becomes nullable; `email` becomes `UNIQUE`
(existing rows pass through the parser in the migration, which fails on a
duplicate or a rejected address); the Google login no longer updates `email`. A
deferred constraint trigger requires `google_sub IS NOT NULL` or a
`password_credential` row at commit.

| Table                 | Columns (beyond `id`, `created_at`)                                                                                                                                                                                       |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `password_credential` | `account_id` unique, `hash` (PHC string), `version` (incremented on every change)                                                                                                                                         |
| `email_link`          | `purpose` (`registration`, `add_password`, `reset`), `token_hash` unique, `email`, `account_id` and `auth_epoch` (both null only for `registration`), `expires_at`, `consumed_at`                                         |
| `pending_auth`        | `token_hash` unique, `csrf_secret`, `account_id`, `auth_epoch`, `primary_kind` (`google`, `password`), `primary_at`, `next` (validated consent path or null), `expires_at`, `consumed_at`                                 |
| `totp_credential`     | `account_id` unique, `sealed_secret`, `key_id`, `last_step`, `failures`, `cooldown_until`, `last_failure_at`                                                                                                              |
| `totp_enrollment`     | `account_id` unique, `web_session_id`, `sealed_secret`, `key_id`, `auth_epoch`, `failures`, `expires_at`                                                                                                                  |
| `recovery_code`       | `account_id`, `digest` unique, `used_at`                                                                                                                                                                                  |
| `recovery_attempt`    | `account_id`, `at`                                                                                                                                                                                                        |
| `mail_outbox`         | `kind`, `class` (`signup`, `recovery`, `notice`), `account_id` or `email_link_id` (exactly one, `ON DELETE CASCADE`), `sealed_payload`, `key_id`, `attempts`, `claimed_until`, `next_attempt_at`, `expires_at`, `sent_at` |

`web_session` gains `proof_at` and `factor_at`; existing rows get null, so their
first sensitive change needs a reauth. Ended links, pending authentications and
enrollments are deleted after 7 days; a sent or expired outbox row loses its
payload at once and is deleted after 7 days.

## Locking and authority

`lockAccount(tx, id)` is `SELECT ... FROM account WHERE id = $1 FOR UPDATE`.
Every issuer and every sensitive change runs in one transaction that takes it
first and then revalidates, against the locked state, exactly what authorized
the request: the account is not blocked or deleting; the epoch the request
carries equals `auth_epoch`; the acting session row is live; the password
`version` it verified is unchanged; the link or pending row is unconsumed,
unexpired, of the right purpose and bound to this account and epoch; recent
proofs are within their windows; on an enrolled account the factor rule below
holds. Any mismatch fails without changing state. The proof each issuer accepts:

| Issuer                         | Authority                                                                                  |
| ------------------------------ | ------------------------------------------------------------------------------------------ |
| Google or password sign-in     | The primary credential; an enrolled account gets a pending authentication, never a session |
| Factor completion              | A live pending row at the current epoch plus a TOTP or recovery code                       |
| Registration completion        | A live `registration` link; creates an account that has no factor                          |
| Session sliding                | The live session at the current epoch; adds no proof                                       |
| OAuth consent                  | A live web session at the current epoch; grant and code take that epoch                    |
| Code exchange, refresh         | A live grant and code or token whose epoch equals the account's                            |
| Personal token (web)           | A live session, recent proof and, when enrolled, recent `factor_at`                        |
| Personal token (admin command) | Operator authority; refused for an account with an enrolled factor                         |

On an enrolled account every web session exists only because a factor completed,
so a live session at the current epoch carries the factor.

`bumpEpoch(tx, id)` increments `auth_epoch` and in the same transaction revokes
every web session, OAuth grant with its codes and tokens, personal token,
pending authentication, TOTP enrollment and outstanding link of the account;
after commit it drops their credential-cache entries. A signed-in change creates
its one replacement session in the same transaction, under the same lock, with a
new token and CSRF secret; the cookie is set only after commit. The replacement
copies `proof_at` and `factor_at` from the acting session and moves either
forward only when that credential was verified in this request. The acting
session is among those revoked. Callers: password set or change, reset (creates
no session), TOTP confirmation or removal, recovery-code replacement, block and
deletion.

## Pre-session requests

These routes carry no session and are the only exceptions to the session CSRF
rule:

| Method and path                     | Requirements                                               |
| ----------------------------------- | ---------------------------------------------------------- |
| `POST /api/v1/auth/signup`          | `Origin`, JSON body, per-IP bucket                         |
| `POST /api/v1/auth/signup/complete` | `Origin`, JSON body, per-IP bucket                         |
| `POST /api/v1/auth/password`        | `Origin`, JSON body, per-IP bucket                         |
| `POST /api/v1/auth/reset`           | `Origin`, JSON body, per-IP bucket                         |
| `POST /api/v1/auth/reset/complete`  | `Origin`, JSON body, per-IP bucket                         |
| `POST /api/v1/auth/factor`          | `Origin`, JSON body, pending cookie and its `X-CSRF-Token` |
| `GET /api/v1/auth/link/{token}`     | per-IP bucket; reads only                                  |
| `GET /api/v1/auth/factor`           | pending cookie; returns `{csrf, next}`; reads only         |

`Origin` must equal the public origin. Add-password completion is a session
route with the session CSRF rule. `__Host-pending` is `Path=/`, `Secure`,
`HttpOnly`, `SameSite=Strict`, 5 minutes, and is set only by a primary sign-in
on an enrolled account; a new primary sign-in consumes any earlier pending row
of that browser. The pending row stores a 32-byte random CSRF secret, as web
sessions do; it is returned in the sign-in response body (password) or by
`GET /api/v1/auth/factor` (Google redirect) and compared in constant time.
Completion and expiry clear the cookie. A pending row authorizes only factor
completion.

## Sign-in entry and OAuth

`/login?next=<path>` is the one sign-in page; `next` is accepted only as a
consent path `/oauth/consent/{handle}`, otherwise `/account`. Google start,
password sign-in and the pending row carry it; registration completion ignores
it. Unauthenticated consent redirects there instead of straight to Google
([OAuth](oauth.md#consent)); the `__Host-oauth` browser binding is unchanged.

## Flows

**Sign-up.** `POST /api/v1/auth/signup {email}`: 202 `{}` always. When sign-up
admits the address and no account holds it, write a `registration` link and its
outbox row; when an account holds it, write an `account_exists` notice to that
account's email instead, within the per-recipient limit. The handler performs
one lookup and one insert in both branches and never waits for mail.
`GET /api/v1/auth/link/{token}` returns `{purpose, email}` for a live link and
consumes nothing. `POST /api/v1/auth/signup/complete {token, password}`
validates the password, then in one transaction claims the link, rechecks that
the address is free and sign-up still admits it, creates the account and
password, and issues a session. Every refusal at this step is 409
`link_invalid`.

**Link redemption.** Completion of any link, in one transaction: select it by
digest `FOR UPDATE`, require the route's purpose, no `consumed_at`, an unexpired
`expires_at` and, for account purposes, the locked account's id and epoch; then
consume it together with the change. A new link of a purpose for an owner is
written only when its outbox row is admitted by every limit and cap; then, in
the same transaction, it consumes that owner's earlier links of the purpose. A
request refused by a limit changes no link. Any epoch change invalidates every
account link.

**Password sign-in.** `POST /api/v1/auth/password {email, password, next}`. An
unenrolled account gets a session and `{next}`; an enrolled one gets
`{factor_required: true, csrf}` and the pending cookie. Google sign-in on an
enrolled account sets the pending row and redirects to `/login/factor`.

**Factor completion.** `POST /api/v1/auth/factor {code}` or `{recovery_code}`
verifies through the verifier below, consumes the pending row, issues a session
with `proof_at` set to the pending row's primary verification time and
`factor_at` now, and returns `{next}`.

**Reset.** `POST /api/v1/auth/reset {email}`: 202 `{}` always; a link only when
an account holds the address and has a password.
`POST /api/v1/auth/reset/complete {token, password}` redeems the link, sets the
hash, increments `version`, calls `bumpEpoch`, writes a `password_changed`
notice and returns 204 with no session. TOTP state is untouched.

**Reauth.** `POST /api/v1/me/reauth {password}` (session and CSRF) verifies the
password and sets `proof_at`. A Google account signs in again, which creates a
session with `proof_at` now.

**Add a password.** `POST /api/v1/me/password/link` (session, CSRF, recent
proof) sends an `add_password` link to `account.email`.
`POST /api/v1/me/password {token, password}` redeems it with a live session of
that account, recent proof and, when enrolled, a recent `factor_at` or a `code`.

**Change password.** `POST /api/v1/me/password {current, password, code?}`
verifies `current` (counted as proof) and, when enrolled, a recent `factor_at`
or `code`; then sets the hash and calls `bumpEpoch`.

## Answers and limits

| Route                   | Answer whatever the email                      | Limits                                                             |
| ----------------------- | ---------------------------------------------- | ------------------------------------------------------------------ |
| Sign-up, reset          | 202 `{}`                                       | Per IP (429); per recipient and global caps silently drop the mail |
| Password sign-in        | 401 `login_failed` on any failure              | Per IP (429); per email; hash queue (503 `busy`)                   |
| Reauth, change password | 401 `reauth_failed`                            | Per account email; hash queue                                      |
| Factor completion       | 401 `factor_failed`; 429 during TOTP cool-down | Verifier state                                                     |

Per-IP limits and the hash queue answer the same for known and unknown emails.
The per-email counter is keyed by the parsed address and counts attempts whether
or not an account holds it. An unknown email, or an account without a password,
still hashes against a fixed dummy hash.

## Password rules

Argon2id, PHC string, constant-time compare. Length 15 to 128 code points after
NFC, at most 1,024 bytes, no composition rules, no truncation; a password on the
bundled common-password list or equal to the address or its local part is 400
`password_rejected`. No breach service is queried: the server makes no outbound
call for passwords. Hashing waits in a bounded queue.

## TOTP

RFC 6238, HMAC-SHA-1, 20-byte secret, six digits, 30-second steps. One verifier
serves every route that takes a code (factor completion, change password,
replacement, removal, recovery-code replacement):

1. Under the lock, if `cooldown_until` is in the future, answer 429 without
   checking the code.
2. Accept a step in t-1, t, t+1 that is greater than `last_step`. On success set
   `last_step` in the transaction of the authorized change; reset `failures`
   only when `last_failure_at` is older than 24 hours.
3. A wrong or replayed code increments `failures`, sets `last_failure_at` and,
   from the fifth failure, `cooldown_until` (15 minutes doubling to 24 hours).
   On a failure the request's own transaction, which holds the account lock,
   records this and commits with no authorized change; there is no nested or
   deferred failure transaction. Reset and sign-in never clear it.

`POST /api/v1/me/totp/setup` (session, CSRF, recent proof; a recent `factor_at`
when replacing) supersedes any enrollment and returns `{otpauth_uri, secret}`
once; issuer `trachuan.vn`, label `account.email`. The web draws the QR code
with a bundled encoder. `POST /api/v1/me/totp/confirm {code}` checks the
enrollment's session, epoch and expiry, verifies the code against the new secret
(five wrong codes end the enrollment), installs or replaces the credential with
`last_step` set, calls `bumpEpoch`, and on first enrollment returns ten recovery
codes once. `DELETE /api/v1/me/totp {code | recovery_code}` and
`POST /api/v1/me/recovery-codes {code}` need recent proof. Enrollment routes
return 404 while `SERVER_TOTP_ENROLLMENT` is off; verification, removal and
recovery never depend on it.

Recovery codes are 128-bit, shown as 26 Base32 characters in groups of five.
Input is normalized by removing spaces and hyphens and upper-casing; anything
but 26 Base32 characters is a failure. The digest is SHA-256 of
`"trachuan-recovery-v1" || 0x00 || decimal account id || 0x00 || code`. A used,
unknown or malformed code inserts a `recovery_attempt` row in the request's
locked transaction, which then commits with no authorized change; beyond the
daily bound the answer is 429. The TOTP cool-down never blocks recovery codes.

## Key ring

One sealing package serves TOTP secrets and outbox payloads; a purpose string in
the associated data separates them. Podman secret `account_keys` holds one or
two lines of `base64url(32 bytes)`, the first active. A key's id is the first 8
bytes of its SHA-256, hex. A malformed file stops the server at start. Sealing
is AES-256-GCM with a random 96-bit nonce and associated data
`purpose|owner_kind|owner_id|row_id|key_id`. A read with the previous key
re-seals with the active key in the same transaction. An unknown key id or
failed open fails that row closed (TOTP `factor_failed` and an alarm; outbox row
dropped and alarmed) and keeps `/readyz` green.

Rotation: restart the server with the new key first and the old second (the
server is the only writer, so after the restart no row is written with the old
key); run `account keys reseal`, which fails if any row cannot be opened; run
`account keys check`, which must report no row on the old key; only then restart
with the new key alone.

## Mail outbox

Classes share the global caps: `signup` (registration and account-exists mail)
may use at most the signup share of the hourly and monthly budget; `recovery`
(reset, add password) and `notice` (security notices) use the rest, and a
security notice is always enqueued in the transaction of its change, outside the
row cap. Notices stay bounded because security changes are: each account may
make at most the number of security changes per day in budgets (reset
completions have their own allowance, so signed-in changes cannot block mailbox
recovery), and a change beyond it is refused with 429 before anything is
written, so every committed change has exactly one notice and no row is ever
replaced. One sender claims a due row by setting `claimed_until` and
incrementing `attempts` in a committed statement, then sends: SMTP to OCI Email
Delivery with mandatory STARTTLS, certificate verification for the configured
host and AUTH only after TLS, under one deadline per message. Success sets
`sent_at` and clears the payload; a failure sets `next_attempt_at`; a crash
leaves the row to be claimed again after `claimed_until`. A message whose
outcome was uncertain may be delivered twice. After the attempt bound or
`expires_at` the payload is cleared. Mail is from `no-reply@trachuan.vn`,
Vietnamese then English, plain text. Kinds: `registration`, `account_exists`,
`add_password`, `reset`, `password_changed`, `totp_enrolled`, `totp_removed`,
`recovery_codes_replaced`. Logs carry kind, row id and result, never the address
or link.

## Release floor

`SERVER_TOTP_ENROLLMENT` defaults to off. It turns on only after the signed pull
deployment's trust policy pins a minimum release at or above the first image
that enforces factors on every issuer; deploy, rollback and restore refuse
images below it.

## Tests

Each issuer on an enrolled account issues nothing without the factor; a reset,
password change or enrollment racing every issuer leaves no credential at the
old epoch and no acting session token alive; answers in the table above are
byte-identical across known, unknown, Google-only and capped emails; a link
completes only for its purpose, once; a registration completing after the
address was taken or sign-up closed creates nothing; the TOTP cool-down survives
a rolled-back request; the outbox never stores an unsealed payload; a stalled
SMTP server does not stop later sends.
