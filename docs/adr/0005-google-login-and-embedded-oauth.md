# ADR 0005: Google-only login; OAuth server copied from aboutme

Status: accepted 2026-11-08.

## Context

Live search, the reader and MCP need an identity for abuse control and revocation. MCP clients authenticate with OAuth 2.1; the 2026-07-28 spec prefers Client ID Metadata Documents and deprecates dynamic client registration. aboutme already runs an OAuth server with CIMD and the resource parameter in production, proven with Claude Code.

## Decision

Google OpenID Connect is the only login; the first login creates the account; identity is the Google subject. The MCP authorization server is embedded in the Go binary, copied from aboutme's oauthsrv, mcpapi and Google login after they are refactored into shorter decoupled files. Personal tokens for CLI clients come from our own table. Public pages (landing, topic tree, connect, sources) need no account.

## Consequences

One identity store, no extra service, no password handling. Hosted Claude and ChatGPT connections must still be proven in a spike; aboutme verified only Claude Code.

## Alternatives rejected

Keycloak (about 1 GB RAM, MCP support experimental). A new hand-written server (banhmi wrote 4k lines that never shipped). github.com/giantswarm/mcp-oauth and AuthPlane (fallbacks if the spike fails).

## History

- 2026-11-08: accepted in the brainstorming conversation.
- 2026-11-09: client spike closed by the owner without a retest; earlier
  sign-ins succeeded and aboutme records a Claude Code CIMD proof. Decision
  stands: copy oauthsrv. ChatGPT and hosted Claude over CIMD remain
  acceptance tests for Plan 2 ([research note](../research/oauth-clients-2026-11.md)).
