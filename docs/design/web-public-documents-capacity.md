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
of every stream, and the gates that pass or fail. Searches use distinct queries,
never repeated within a run: the evaluation suite questions first, then the
distinct document titles and article headings of the release under test, which
is public law text; a preflight checks the count covers every phase (about 3,400
searches) before any traffic. A completed reader load is the reader page HTML
with the session cookie, then the first `/api/v1/documents/{key}` page; its
latency runs from the first byte sent to the last byte of the second response.
Summary URLs come from `/api/v1/public/documents`.

Each simulated address sends its traffic with `X-Forwarded-For` set to one
address from `2001:db8::/32`, the documentation range. Phase B uses 16
addresses, phase D 40 for pages and 50 for the public API. Each signed-in
account uses one fixed address of its own. In phase B a `busy` answer is retried
at most three times, after 1, 2 and 4 seconds or the longer `Retry-After` up to
5 seconds. A page answer is busy when it is a 503, or a 429 whose `Retry-After`
is at most 5 seconds (render admission); a per-address 429 is not busy and
counts as a failure. In phase D, 5% of the public requests are cancelled at a
random point and their 429 and 503 answers are not gated; signed-in requests are
never cancelled and keep their gates.

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
each with a Google subject that starts with `loadcheck:`, a prefix that Google
login refuses before any account lookup or creation, so no Google identity can
ever reach such an account.
`server admin session create --account ID --out FILE` mints one web session for
such an account and refuses any account whose subject lacks the prefix, so it
can never sign in as a real person. It writes the session value only into
`FILE`, created new with mode `0600` and refused if it exists, never to standard
output, and the session lasts at most one day. A failed command delivers no
value: it revokes what it can and removes its file, and a session row that a
commit with a lost outcome still leaves is unusable, because only its hash is
stored, and ends within the day or when the account is blocked. Anyone with a
shell on the host can already write sessions into the database, so the command
adds no privilege. After the window the accounts are blocked, which revokes
their sessions. No owner action or real Google account is needed.

## Sampling

- Renders: Nitro logs one line per summary render with the summary path, in the
  hook that admits renders. A cache hit runs no render and logs nothing. In
  phase C a key whose second request logged no second render was a hit.
- Reader-pool wait: each reader pool times every acquire with pgx's acquire
  tracer. The server logs once a minute, and only for minutes with acquires, the
  count per fixed bucket with the p50, p99 and maximum wait. The gate's p99 is
  taken over the bucket counts summed across the run, as the upper edge of the
  bucket that holds it.
- PostgreSQL connections: on A1 the host runs a loop, once a second, that
  appends `<RFC3339 UTC> <count>` to `connections.txt`, the count of client
  backends in `pg_stat_activity` read through `podman exec` into the PostgreSQL
  container. On A1 the sampler runs without `--pg` and holds no database
  connection. A malformed line fails the report; a last line cut short by
  killing the loop is ignored.
- CPU and memory: once a second, for web, server, queryembed, postgres and
  caddy, from each service's cgroup v2 directory
  (`app.slice/trachuan-<name>.service`): CPU is the `cpu.stat` `usage_usec`
  delta over wall time (100 is one core), and memory is `memory.current` and
  `memory.peak`. A falling counter or a directory that vanishes and returns is a
  restart; CPU is then not reported for that second. Host CPU comes from
  `/proc/stat` (the MacBook uses `ps`). `ps` only marks restarts, by a changed
  process start time; its CPU is a lifetime average and feeds no gate. The
  MacBook has no service cgroups, so the web and server memory gates are
  `unavailable` locally.

## A1 window

The window runs between 01:00 and 03:00 Vietnam time, with no deploy in flight
and none tagged until it ends; a deploy rewrites `server.env` and would end the
window. The coordinator runs every host step:

1. Check that the host watch shows no open failure. Create the accounts and
   sessions; check `/api/v1/public/status`.
2. Add `SERVER_PUBLIC_SUMMARIES=true` to `/srv/trachuan/host/server.env` and
   restart `trachuan-server`; status shows `public_summaries` true.
3. Start the sampler and the connections loop, then run the phases. Phase D's
   activation is the runbook's compare-and-swap rollback to the previous release
   and, after 60 seconds, back. Summary answers in those 60 seconds are not
   gated. The two activations make the host watch send two "corpus live"
   Telegram messages; they are expected, not an incident. Phases may run as
   separate invocations, each with its own `--out` directory: A, then D twice. A
   report whose run has no phase A takes A's search and reader latencies from
   `--baseline`, the A run's directory; with neither, the relative bounds are
   `unavailable`. A sample counts toward the phase whose time window holds it;
   one outside every phase is ignored. Host CPU is gated per phase in A to C on
   the highest 60 s rolling mean of the 1 s samples (the phase mean when it has
   fewer than 60), and recorded as `info` in D; the phase mean and the 1 s peak
   are `info` rows. A service's memory peak for the run is the largest
   `memory.current` in its phases, raised to the last sample's `memory.peak`
   only when that is higher than the first sample's. Requests starting in the 60
   s after an activation count toward the absolute latency bounds but not the
   relative one.
4. Remove the flag line, restart the server, and check status shows
   `public_summaries` false. Block the accounts and delete the session files.
   Check that the uptime alarms are OK.

Abort the window by going to step 4 when any of these occurs: a signed-in
request fails other than with its account's own limit; search p95 over one
minute exceeds 2 s; a service's `memory.current` passes 90% of its cgroup cap; a
container restarts; PostgreSQL connections pass 45; or a non-test request fails
in the server log; in phases A to C only, host CPU stays above 90% for 60
seconds. Phase D has no host CPU trigger. During phase D, complete the
activation back to the release that was active before the run first.
