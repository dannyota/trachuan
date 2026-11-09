# OAuth clients against aboutme (2026-11-09)

The spikes plan asked whether Claude.ai, ChatGPT and Claude Code connect to
aboutme's OAuth server, before trachuan copies it. The owner stopped the live
retest on 2026-11-09 because earlier sign-ins through aboutme's OAuth already
succeeded. This note records the evidence that decision rests on and what stays
unverified. Not authority; the design owns the rules in
[auth](../design/auth.md).

## Evidence

| Source                                                                       | What it shows                                                                                                                                                                                                     |
| ---------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Owner, 2026-11-09                                                            | Earlier sign-ins through aboutme's OAuth succeeded; no retest needed. Which clients, mechanism (CIMD or DCR) and dates were not recorded                                                                          |
| aboutme `docs/design/mcp-cimd-verification.md`                               | Owner proof with Claude Code over CIMD in production on 2026-10-14                                                                                                                                                |
| Same document                                                                | Hosted Claude's CIMD connection is recorded as unverified                                                                                                                                                         |
| `https://aboutme.vn/.well-known/oauth-authorization-server`, read 2026-11-09 | `client_id_metadata_document_supported: true`; registration endpoint `/oauth/register` (DCR accepted); PKCE `S256` only; token endpoint auth `none` (public clients only); scopes `resumes:read`, `resumes:write` |
| aboutme session, 2026-11-09                                                  | `OAUTH_CIMD_ENABLED=true` in production through `flags.env`                                                                                                                                                       |

## Not verified

- ChatGPT connectors. aboutme accepts only public clients, so a ChatGPT
  registration that asks for `client_secret_post` or `client_secret_basic` would
  be refused at `/oauth/register`; redirect URIs must match exactly.
- Hosted Claude through CIMD specifically, as opposed to DCR, unless the owner's
  earlier sign-in was a Claude.ai connector over CIMD.
- Refresh after expiry and revocation timing for each client.

## Decision

Copy aboutme's `oauthsrv`, `mcpapi` and Google login after their refactor, as
[ADR 0005](../adr/0005-google-login-and-embedded-oauth.md) decided. The fallback
(`github.com/giantswarm/mcp-oauth`) is not pursued now; revisit it if the
server's OAuth acceptance tests fail for ChatGPT or hosted Claude. Those
acceptance tests run against trachuan's own deployment: a ChatGPT connector, a
Claude.ai connector, Claude Code, token refresh and revocation.
