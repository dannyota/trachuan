# Host watch and Telegram alerts

Proposed. A host timer reads the same local job results as the metrics
publisher, checks readiness, memory and disk, and sends the owner a short
Telegram message when a state changes. OCI email alarms
([host alarms](budgets-host.md#host-alarms)) stay the independent backstop: they
fire even when the host is down and cannot send anything itself.

## Unit and user

`trachuan-watch.timer` starts `trachuan-watch.service` every minute
(`OnCalendar=minutely`, `Persistent=false`). The service runs
`/usr/local/libexec/trachuan/watch.py` with `python3 -I` as system user
`trachuan-watch`. It keeps the hardening of `trachuan-metrics.service`
(`ProtectSystem=strict`, `ProtectHome`, `PrivateTmp`, `NoNewPrivileges`) and
adds `ReadWritePaths=/srv/trachuan/state/watch /var/log/trachuan/watch`,
`LimitCORE=0`, `MemorySwapMax=0`, `TimeoutStartSec=45` and `MemoryMax=128M`.
`LimitCORE=0` and `MemorySwapMax=0` keep the token out of core dumps and swap,
as for every [secret holder](deployment.md#secrets). It uses the Python standard
library plus the OCI SDK the metrics publisher already uses.

`install.sh` creates the user; grants it read ACLs on the result files the
metrics user reads and traversal (`--x`) on `/srv/trachuan/state` and
`/var/log/trachuan`; creates `/srv/trachuan/state/watch/` and
`/var/log/trachuan/watch/` owned by it, mode 0750; grants the metrics user `r-x`
on the watch state directory and `r--` on `result.json`; and adds `watch.log` to
the logrotate file with `copytruncate`. The OCI Logging source in
`deploy/oci/logging.tf` adds the path `/var/log/trachuan/watch/*.log`, because
its `/var/log/trachuan/*.log` pattern does not descend into subdirectories. The
signed host bundle ships the units like the others
([pull deployment](pull-deployment.md)).

## Signals

Each run observes every signal once. A failure signal fires when its condition
has held for the window in the table and clears only on a confirmed good state.
An indeterminate observation (a missing result, `running`, `skipped`, a check
skipped during a deploy) keeps the current state and does not advance or reset
the window.

| Signal                     | Fires when                                                                                                  | Clears when                                               | Mark |
| -------------------------- | ----------------------------------------------------------------------------------------------------------- | --------------------------------------------------------- | ---- |
| site not ready             | `GET http://127.0.0.1:8080/readyz` is not 200 within 5 s on 3 consecutive observations spanning 3 min       | one 200                                                   | 🔴   |
| deploy stopped             | deploy `result.json` status `stopped`                                                                       | status `ok` or `current`                                  | 🔴   |
| backup failed              | a backup job result (`full`, `diff`, `verify`, `health`, `restore-check`) is `failed` or `unknown`          | that job is `ok` with `last_success_at` after the failure | 🔴   |
| backup old                 | the newest `full` or `diff` `last_success_at` is over 26 h old                                              | a newer success brings it under 26 h                      | 🔴   |
| deploy failing             | 3 consecutive completed updater attempts (distinct `started_at`) are `failed` or `rolled_back`, over 15 min | status `ok` or `current`                                  | 🟠   |
| corpus release failing     | consecutive completed attempts in `release.json` are `failed` for 30 min                                    | status `ok`, `current` or `verified`                      | 🟠   |
| corpus release waiting     | `release.json` `waiting_since` is over 24 h old                                                             | status `ok`, `current` or `verified`                      | 🟠   |
| frameworks release failing | as corpus release failing, in `release-frameworks.json`                                                     | status `ok`, `current` or `verified`                      | 🟠   |
| frameworks release waiting | as corpus release waiting, in `release-frameworks.json`                                                     | status `ok`, `current` or `verified`                      | 🟠   |
| secrets failing or stale   | `secrets.json` attempts are `failed` for 30 min, or `last_success_at` is over 2 h old                       | `ok` or `current` with `last_success_at` under 2 h old    | 🟠   |
| filesystem                 | `/` or `/srv/trachuan/data` used over 70% on every observation for 15 min                                   | used under 65%                                            | 🟠   |
| memory                     | `MemAvailable` under 10% of `MemTotal` in `/proc/meminfo` on every observation for 15 min                   | `MemAvailable` over 15%                                   | 🟠   |

The readiness check is skipped while
`systemctl is-active trachuan-deploy.service` reports `active` or `activating`:
an activation stops and restarts services, and the updater unit's own
`TimeoutStartSec=60min` bounds that window. The OCI uptime alarms still cover an
outage during a deploy. Deploy failing tells the owner after three failed
attempts instead of the OCI alarm's 30 minutes; one or two failed attempts, such
as a sigstore timeout during a deploy, never alert.

Two notices are events, not failures:

- **Release live.** The active release is `release` and `sequence` from the
  deploy result when its status is `ok` or `current`; `current` keeps them after
  the activating run.
- **Corpus live.** The active corpus is the `release` field of
  `GET http://127.0.0.1:8080/api/v1/public/status`, the release the server
  serves. `release.json` is not used for this: `verified` and `current` can
  describe a manifest whose activation is disabled. A failed or non-200 status
  read is indeterminate.

An unreadable or malformed result file fails the run (see
[watch health](#watch-health)) instead of guessing.

## Messages

English, Telegram HTML parse mode, link previews off. Four lines:

1. Mark, product in bold, then what happened:
   `🔴 <b>trachuan</b> · site not ready`.
2. The facts with numbers and the start time:
   `/readyz failed 3 checks in a row since 14:02 ICT; last HTTP 503.`
3. `Action:` what the owner should do, or `System:` what the host does by
   itself.
4. In `<i>`: `HH:MM ICT · HH:MM UTC · <job> · <runbook path>`.

Marks: 🔴 down or failed, 🟠 degraded or still failing, 🟢 recovered or live. A
recovery names how long the signal fired. Every variable part is escaped (`&`,
`<`, `>`), and a message stays under Telegram's 4,096 characters
([sendMessage](https://core.telegram.org/bots/api#sendmessage)). A job's
`message` field is at most 160 characters after collapsing whitespace; the jobs
write status there, never secret values or account data, and the watch adds
nothing else from the host. Messages never carry account email, user questions
or secret values.

Times come from the host clock (UTC, kept by chrony). The send time and the
start times are shown in ICT through the tz database zone `Asia/Ho_Chi_Minh`,
then in UTC.

## State and delivery

`/srv/trachuan/state/watch/state.json` keeps, per failure signal, the observed
state, its window start and the attempts counted, the state last told and when;
for each event, the identity observed and the identity last told; send times for
the rate limit; and a Telegram retry deadline. It is replaced atomically. A
missing or invalid state file starts fresh: the first run sets each told event
identity to the observed one without a message, and a failure already present is
told after its own window.

Due messages:

- A failure signal whose state differs from the state last told: fired gives the
  🔴 or 🟠 message; cleared after a told failure gives the 🟢 recovery; fired
  and cleared before it was told gives nothing.
- A failure still firing 24 hours after it was last told: a 🟠 reminder.
- An event whose observed identity differs from the told one: one 🟢 message
  naming the newest identity. An identity is never marked told until its message
  is accepted, so a blocked delivery carries it forward.

Due messages go out 🔴 first, then 🟠, then 🟢, at most 20 in any rolling hour;
the rest stay due. Delivery is one `sendMessage` call per message with a
10-second timeout and no retry inside the run. A 429 stores
`parameters.retry_after` as the retry deadline when it is a positive integer,
otherwise 60 seconds, and no run sends before it
([ResponseParameters](https://core.telegram.org/bots/api#responseparameters)). A
non-2xx status or `"ok": false` stops sending for the run.

The state file is rewritten after each accepted message, so a crash loses at
most one record. `sendMessage` has no idempotency key, so a message Telegram
accepted just before a timeout or crash is sent again by the next run: delivery
is at least once, and one duplicate is accepted.

## Secrets

The bot token and chat id are the Vault secrets `telegram_bot_token` and
`telegram_chat_id`. The watch reads them with the instance principal only in a
run that has a message due, keeps them in memory for that run, and never writes
them to disk, a log, the state file, process arguments or an exception message.
The host dynamic-group policy in `deploy/oci/iam.tf` names both secrets; the
updater never reads them. The token appears only in the Bot API request path, so
every error from Vault or Telegram is logged as a fixed text plus the HTTP
status and Telegram `error_code`, never the URL, response body or exception
text. The chat id is configuration, kept out of the repository because the
repository may become public.

## Watch health

Each run writes `/srv/trachuan/state/watch/result.json` (`ok` or `failed`,
`last_success_at`, a fixed message) and one line per due message to
`/var/log/trachuan/watch/watch.log`, which OCI Logging collects with the other
host job logs. A run succeeds when it read every signal and, after sending, has
no message left due or had at least one message accepted. A run that leaves
messages due without an acceptance, whether from a send error, a retry deadline
or the rate limit, fails, so a long Telegram block ends in the watch alarm.

The metrics publisher reports `watch_stale`: 0 when the result has a
`last_success_at` under 1 hour old, and 1 when it is older, absent, never
succeeded or malformed. A bad watch result never fails the other metrics. The
OCI alarm "Telegram watch silent" (`WARNING`, `watch_stale` 1 for 10 minutes,
daily repeat) emails the owner when Telegram alerts have stopped working,
including a watch that never succeeded after install.

## Tests

Unit tests run in CI with fixtures and fake Vault and Telegram clients: every
signal's fire, hold, indeterminate, clear and reminder transitions; attempt
counting by `started_at`; the readiness skip during a deploy; release live
across `ok` then `current`; corpus live from the status endpoint, unchanged by a
`verified` manifest with activation disabled; both events across a blocked
delivery; the first run; the rate limit and its ordering; the retry deadline and
run success while messages stay due; a crash before and after the state write;
HTML escaping, the 160 character cap and the 4,096 character limit; ICT and UTC
rendering across midnight; Vault failure; `watch_stale` for each result case;
and that no log line, state file, result or raised error contains the token or
chat id.
