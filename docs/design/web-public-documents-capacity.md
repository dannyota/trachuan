# Summary capacity harness

How the capacity acceptance in
[serving public summaries](web-public-documents-serving.md#capacity-acceptance)
is run: the load tool, the accounts, what is sampled, and the window on the
serving host. The phases, traffic and gates stay in that note; measured results
go into [budgets](budgets.md).

## Load tool

`apps/server/cmd/loadcheck`, a Go command in the main module. Go gives one
static binary for the MacBook and the arm64 host, reuses the e2e sign-in and
request code, and times requests without a new dependency. It has no production
role and is never part of an image.

The tool sends open-loop traffic: each stream starts requests at its stated rate
whether or not earlier ones finished, so a slow server shows as latency, not as
a lower rate. Per request it records start, duration, status, retries and
whether it was aborted. A phase report gives the counts and the p50, p95 and p99
of every stream, and the gates that pass or fail. Searches use distinct queries
from the evaluation suite, never repeated within a run. A completed reader load
is the reader page HTML with the session cookie, then the first
`/api/v1/documents/{key}` page; its latency runs from the first byte sent to the
last byte of the second response. Summary URLs come from
`/api/v1/public/documents`.

Each simulated address sends its traffic with `X-Forwarded-For` set to one
address from `2001:db8::/32`, the documentation range. Phase B uses 16
addresses, phase D 40 for pages and 50 for the public API. Each signed-in
account uses one fixed address of its own. In phase B a `busy` answer is retried
at most three times, after 1, 2 and 4 seconds or the longer `Retry-After` up to
5 seconds. In phase D, 5% of requests are cancelled at a random point.

## Where it runs

On the MacBook, the stack runs as the e2e stack does: server, web, the e2e edge
and fake Google, with the real release `vn/20261117_7` as the corpus, the real
query embedder and `SERVER_PUBLIC_SUMMARIES=true`. The edge appends the client's
`X-Forwarded-For`, so simulated addresses reach Nitro and the server as they
would through Caddy.

On the A1 host, the tool runs on the host and connects to `127.0.0.1:3000` for
pages and `127.0.0.1:8080` for the API, both of which trust `127.0.0.1` as a
proxy. Caddy replaces any client `X-Forwarded-For`, so remote addresses cannot
be simulated through it, and one remote client would meet the per-address
limits. TLS and Caddy are therefore outside the measured path on A1; they were
measured with real traffic before. The tool's own CPU is reported beside the
host total.

## Accounts

Locally, three accounts sign in through fake Google with the e2e flow.

On A1, three dedicated accounts are created with `server admin account create`,
each with a synthetic Google subject that no Google account can match. A new
`server admin session create --account ID` prints one web session value once,
valid for one day, into a file with mode `0600` that only the tool reads. Anyone
with a shell on the host can already write sessions into the database, so the
command adds no privilege. After the window, the accounts are blocked, which
revokes their sessions. No owner action or real Google account is needed.

## Sampling

- Renders: Nitro logs one line per summary render with the summary path, in the
  hook that admits renders. A cache hit runs no render and logs nothing. In
  phase C a key whose second request logged no second render was a hit.
- Reader-pool wait: each reader pool times every acquire with pgx's acquire
  tracer. The server logs once a minute, and only for minutes with acquires, the
  count and the p50, p99 and maximum wait from fixed buckets.
- PostgreSQL connections: client backends in `pg_stat_activity`, once a second.
- CPU and memory: once a second, host CPU from `/proc/stat` (the MacBook uses
  `ps`), and the RSS and CPU of the web, server and PostgreSQL processes; a
  changed process or container start time marks a restart.

## A1 window

The window runs between 01:00 and 03:00 Vietnam time, with no deploy in flight
and none tagged until it ends; a deploy rewrites `server.env` and would end the
window. The coordinator runs every host step:

1. Create the accounts and sessions; check `/api/v1/public/status`.
2. Add `SERVER_PUBLIC_SUMMARIES=true` to `/srv/trachuan/host/server.env` and
   restart `trachuan-server`; status shows `public_summaries` true.
3. Start the samplers, then run phases A to D. Phase D's activation is the
   runbook's compare-and-swap rollback to the previous release and, after 60
   seconds, back. Summary answers in those 60 seconds are not gated.
4. Remove the flag line, restart the server, and check status shows
   `public_summaries` false. Block the accounts and delete the session files.

Abort the window by going to step 4 when any of these occurs: a signed-in
request fails other than with its account's own limit; search p95 over one
minute exceeds 2 s; host CPU stays above 90% for 60 seconds; web or server RSS
passes 90% of its container limit; a container restarts; PostgreSQL connections
pass 45; or a non-test request fails in the server log. During phase D, complete
the activation back to `vn/20261117_7` first.
