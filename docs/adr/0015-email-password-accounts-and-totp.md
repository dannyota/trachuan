# ADR 0015: Email and password accounts, open sign-up, optional TOTP

Status: accepted 2026-11-14.

## Context

Login is Google only ([ADR 0005](0005-google-login-and-embedded-oauth.md)). The
owner wants accounts that need no Google account, so that people without one can
sign up and features can be tested with throwaway accounts. He also wants an
authenticator-app second factor for his own account, and sign-up by email as in
aboutme (aboutme ADR 0015 and 0017, which this record adapts). On 2026-11-14 the
owner chose open sign-up for anyone with a verified email, and OCI Email
Delivery for mail.

The first production host serves the owner only. ADR 0006 History (2026-11-13)
requires signed pull deployment, Vault secrets, backups with a restore proof,
alarms and the OpenTofu import before any other account signs in.

## Decision

1. **Email and password beside Google.** An account has at most one Google
   identity and at most one password credential, and needs at least one of
   them. The sign-in name is the email address; there is no separate username.
   Sessions stay the only browser credential; every issuer issues the same
   session.
2. **The account email is fixed and verified.** One canonical parser (ASCII
   addr-spec, lower-cased domain) serves registration, login lookup, storage and
   rate-limit keys. The account email is set at creation and never changes: a
   Google sign-in no longer copies the provider's current email, superseding
   the rule in [web API](../design/web-api.md#sessions). It is unique across
   accounts. A Google identity resolves by its subject only; a Google sign-in
   with a new subject whose email another account holds is refused with a
   generic message, and accounts are never merged or linked by email. Changing
   the email and removing the last credential stay out of scope.
3. **Registration verifies the mailbox before any password exists.** Sign-up
   takes an email only and sends a single-use link. The person who opens the
   link chooses the password in that ceremony, and only then is the account
   created. Before an account exists, the pending registration has its own
   identifier: its link token, sealed mail and expiry bind to that identifier
   and the purpose `registration`, not to an account or epoch. Creating the
   account rechecks, in one transaction, that the email is still free and that
   sign-up still admits it. A Google account adds a password the same way: a
   link sent to its account email, opened by the signed-in owner of that
   account.
4. **Uniform answers and bounded work.** Sign-up, login and reset answer
   identically whether or not the email is known, including when admission is
   refused. Passwords use Argon2id; link tokens are random and stored as
   SHA-256 digests. Length bounds follow NIST SP 800-63B (no composition rules;
   room for passphrases). Limits exist per client address, per email and per
   recipient, plus global caps on pending registrations, outbox size, mail sent
   per hour and concurrent hashing; pending rows and links expire and mail
   retries are bounded. The numbers live in [budgets](../design/budgets.md).
5. **Reset only for password credentials, and it never signs in.** A reset link
   is sent only for an account that has a password; a Google-only account gets
   the same answer and no mail. A reset sets the new password, increments the
   epoch and issues no session. It never removes or bypasses a second factor.
6. **Optional TOTP second factor.** RFC 6238, HMAC-SHA-1, 20-byte secret, six
   digits, 30-second step, previous, current and next step accepted, each step
   used once. One authenticator per account. It belongs to the account, so it
   also follows a Google sign-in. Primary sign-in on an enrolled account yields
   a short pending authentication, not a session; only a TOTP code or a
   recovery code completes it. Failed TOTP codes have a per-account cool-down
   that grows, ignores client address and survives password reset and new
   logins. Recovery codes have their own attempt bound and are never blocked
   by the TOTP cool-down.
7. **Recovery codes are the only lost-factor path.** First enrollment returns
   ten single-use codes once; the server keeps only their digests. Password
   reset, Google sign-in, email and the operator never remove a factor.
8. **Sealed secrets.** TOTP secrets and token-bearing outbox payloads are
   AES-256-GCM sealed with a key ring held as a Podman secret, outside the
   database, and bound to their row and owner (an account, or a pending
   registration before one exists); a database backup alone
   yields neither codes nor usable links. Outbox payloads are deleted once sent
   or expired. The QR code is drawn in the browser from the `otpauth` URI.
9. **Sensitive changes need fresh proof.** Setting or changing a password,
   enrolling, replacing or removing TOTP and regenerating recovery codes
   require CSRF and Origin checks, recent primary proof (the password, or a
   Google sign-in within the window in budgets) and, on an enrolled account, a
   TOTP or recovery code. A new authenticator is active only after a correct
   code from it. Registration (item 3) and forgotten-password reset (item 5)
   need no existing primary proof: the verified mailbox link authorizes them,
   and reset keeps its no-session and factor-preservation rules.
10. **One epoch fences all authority, under one lock.** Every issuer (Google
    and password sign-in, pending-factor completion, session rotation, OAuth
    consent, code exchange, refresh, personal token creation) and every
    sensitive change takes the account row lock and rechecks blocked state,
    credentials, proof state and `auth_epoch` before it issues. Pending
    ceremonies on an existing account record the epoch they began under and
    fail on a mismatch; a pending registration has no epoch (item 3). Each
    sensitive change and each reset increments the epoch and, in the same
    transaction, revokes every session (except the one fresh session a signed-in
    change returns), OAuth grant, refresh token, authorization code and personal
    token, and drops their credential-cache entries.
11. **Factor enforcement has a release floor.** An older image would issue a
    session without the factor. TOTP enrollment stays off until the host deploy
    policy enforces a minimum release that verifies factors; deploy, rollback
    and restore refuse images below it. Turning enrollment off later never
    lowers the floor or stops verification.
12. **Mail through OCI Email Delivery.** SMTP from a `trachuan.vn` sender with
    SPF and DKIM published through Cloudflare. Verification, reset and security
    notices (password set or changed, factor enrolled or removed) are written to
    the outbox in the same transaction as the change; a failed send retries
    within its bound and never blocks the change. Mail is bilingual.
13. **Open sign-up follows the full hardening gate.** Until every prerequisite
    in ADR 0006 History (2026-11-13) is live, `SERVER_SIGNUP_EMAILS` gates
    password registration exactly as it gates Google sign-up. Opening sign-up
    is then a configuration change.

## Consequences

- The auth schema gains password credentials, pending registrations, email
  tokens, TOTP credentials, recovery codes, pending authentications and a mail
  outbox; `account.google_sub` becomes nullable and `account.email` unique.
- The server holds a key ring secret and SMTP credentials; the host bundle and
  runbooks name them by secret name only. Key rotation needs a re-sealing run
  before an old key is removed.
- A password reset or any security change signs the account out of every MCP
  client and revokes its personal tokens; clients reconnect.
- The web gains sign-up, verify, password sign-in, reset and account security
  pages; OAuth consent uses the same sign-in, so MCP clients work for password
  and TOTP accounts.
- The contract lives in a design document reviewed before code; tests cover
  concurrent issuance against a reset or enrollment, every issuer under an
  enrolled factor, and uniform answers.

## Alternatives rejected

- Choosing the password at sign-up before the mailbox is proven: a stranger
  could pre-set a known password on someone else's address.
- A username separate from the email: a second identifier that recovery and
  support would also need.
- Passkeys first: stronger against phishing, but WebAuthn is larger protocol
  work; they can join later behind the same epoch and recovery codes.
- Email-only magic links: every sign-in would depend on mail delivery.
- Keycloak or Cognito: a second identity authority beside the server.
- AWS SES as aboutme uses: a second cloud and credential on an OCI host.

## History

- 2026-11-14: proposed after the owner chose open sign-up and OCI Email
  Delivery; revised after independent review (S-01 to S-13) and accepted.
