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
`trachuan-watch`, with the hardening of `trachuan-metrics.service`,
`TimeoutStartSec=45` and `MemoryMax=128M`. It uses the Python standard library
plus the OCI SDK the metrics publisher already uses. `install.sh` creates the
user, grants it read ACLs on the result files the metrics user reads, and owns
`/srv/trachuan/state/watch/` for it. The signed host bundle ships it like the
other units ([pull deployment](pull-deployment.md)).

## Signals

Each run observes every signal once. A signal fires when its condition has held
for the time in the table, measured from the first run that saw it, and clears
on the first run that does not see it.

| Signal                   | Source                                                      | Fires after | Mark |
| ------------------------ | ----------------------------------------------------------- | ----------- | ---- |
| site not ready           | `GET http://127.0.0.1:8080/readyz` not 200 within 5 s       | 3 min       | 🔴   |
| deploy stopped           | deploy `result.json` status `stopped`                       | at once     | 🔴   |
| backup failed            | a backup job result `failed` or `unknown`                   | at once     | 🔴   |
| backup old               | newest `full` or `diff` success over 26 h                   | at once     | 🔴   |
| deploy failing           | deploy status `failed` or `rolled_back`                     | 15 min      | 🟠   |
| corpus release failing   | `release.json` status `failed`                              | 30 min      | 🟠   |
| corpus release waiting   | `release.json` `waiting` for over 24 h                      | at once     | 🟠   |
| secrets failing or stale | `secrets.json` `failed`, or last success over 2 h           | 30 min      | 🟠   |
| filesystem               | `/` or `/srv/trachuan/data` used over 70%                   | 15 min      | 🟠   |
| memory                   | `MemAvailable` under 10% of `MemTotal` in `/proc/meminfo`   | 15 min      | 🟠   |
| release live             | deploy status `ok` with a release or sequence not seen yet  | at once     | 🟢   |
| corpus live              | `release.json` status `ok` with a corpus `rel` not seen yet | at once     | 🟢   |

A missing result file is "no data", not a failure; the OCI absence alarms cover
it. An unreadable or malformed result fails the run (see
[watch health](#watch-health)) instead of guessing. The thresholds match the OCI
alarms, except deploy failing, which tells the owner after 15 minutes (three
failed runs) instead of 30; one or two failed runs, such as a sigstore timeout
during a deploy, never alert.

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
`<`, `>`). A job's `message` field is at most 160 characters after collapsing
whitespace; the jobs write status there, never secret values or account data,
and the watch adds nothing else from the host. Messages never carry account
email, user questions or secret values.

Times come from the host clock (UTC, kept by chrony). The send time and the
start times are shown in ICT through the tz database zone `Asia/Ho_Chi_Minh`,
then in UTC.

## State and delivery

`/srv/trachuan/state/watch/state.json` keeps, per signal, the observed state,
when it started, the state last told to the owner and when; plus the last
release, sequence and corpus `rel` seen, and send times for the rate limit. It
is written atomically. A missing or invalid state file starts fresh: the first
run records the current releases without a 🟢 message, and a signal already
firing is told after its own window.

A message is due when a signal's state differs from the state last told: fired
gives the 🔴 or 🟠 message, cleared after a fired message gives the 🟢 recovery,
and a signal that fired and cleared before it was told sends nothing. A signal
still firing 24 hours after it was last told is due again as a 🟠 reminder. Due
messages go out 🔴 first, then 🟠, then 🟢, at most 20 in any rolling hour; what
does not fit stays due for the next run. The state records a message as told
only after Telegram accepts it.

Delivery is one `sendMessage` call per message to the Bot API with a 10-second
timeout and no retry inside the run; the next run retries. A 429, a non-2xx
status or `"ok": false` stops sending for this run.

## Secrets

The bot token and chat id are the Vault secrets `telegram_bot_token` and
`telegram_chat_id`. The watch reads them with the instance principal only in a
run that has a message due, keeps them in memory for that run, and never writes
them to disk, a log, the state file or an exception message. The host
dynamic-group policy in `deploy/oci/iam.tf` names both secrets; the updater
never reads them. The token appears only in the Bot API request path, so every
error from Vault or Telegram is logged as a fixed text plus the HTTP status and
Telegram `error_code`, never the URL, response body or exception text. The chat
id is configuration, kept out of the repository because the repository may
become public.

## Watch health

Each run writes `/srv/trachuan/state/watch/result.json` (`ok` or `failed`,
`last_success_at`, a fixed message) and one line per due message to
`/var/log/trachuan/watch.log`, which OCI Logging collects with the other host
job logs. A run succeeds when it read every signal and either had nothing due or
delivered at least one message without an error. The metrics publisher reports
`watch_last_success_age_seconds`, and the OCI alarm "Telegram watch silent"
(`WARNING`, over 1 hour for 10 minutes, daily repeat) emails the owner when
Telegram alerts have stopped working.

## Tests

Unit tests run in CI with fixtures and fake Vault and Telegram clients: every
signal's fire, hold, clear and reminder transitions; the persistence windows;
the first run; the rate limit and its ordering; HTML escaping and the 160
character cap; ICT and UTC rendering across midnight; a 429 and an `ok: false`
reply; Vault failure; and that no log line, state file, result or raised error
contains the token or chat id.
