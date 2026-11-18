# Host alarms

OCI Monitoring sends alarms to the Notifications topic `trachuan-alarms`.
Confirm the email subscription from the confirmation message. In the OCI
Console, open Notifications, select `trachuan-alarms`, and check that the
subscription is active. See [Oracle's subscription instructions][subscription].

The alarms, their conditions, how long each must persist and their severity are
listed once in [host budgets](../design/budgets-host.md#host-alarms); they are
defined in `deploy/oci/` and applied with OpenTofu, never created by hand.

The external HTTPS alarms mean every vantage point failed for 5 minutes. In the
OCI Console, open Health Checks and inspect the results page for
`trachuan website uptime` (`/`) or `trachuan public status uptime`
(`/api/v1/public/status`). Check DNS, Caddy and the host alarms above.

A deleted monitor or missing Health Checks data produces no failure samples, so
these alarms stay quiet. A firing alarm can also [reset on missing data][reset]
without recovery. After any reset, confirm in the monitor's results that probes
resumed and succeed.

Set backup job age and absence windows to match each job's schedule. The host
publishes last-success age separately for `full`, `diff`, `verify`, `health` and
`restore-check`. A missing result or missing `last_success_at` has no age
metric. A missing result has no failure metric either. Review job schedules and
alarm windows before enabling those alarms.

To check metrics, set `COMPARTMENT_ID` to the OCID of compartment `trachuan`,
then query a recent UTC interval with the [OCI CLI][summarize]:

```sh
oci monitoring metric-data summarize-metrics-data \
  --compartment-id "$COMPARTMENT_ID" --namespace trachuan_host \
  --query-text 'heartbeat[1m].mean()' \
  --start-time '2026-01-01T00:00:00Z' --end-time '2026-01-01T01:00:00Z'
```

Replace the example times with a recent interval. On the host, check
`systemctl status trachuan-metrics.timer` and
`journalctl -u trachuan-metrics.service` if the stream is absent.

The host watch sends Telegram alerts when local job or host health changes.
Check `systemctl status trachuan-watch.timer trachuan-watch.service`,
`journalctl -u trachuan-watch.service` and
`/srv/trachuan/state/watch/result.json`; `last_success_at` records the last good
run. The "Telegram watch silent" email alarm means the watch has not succeeded
within an hour, or has never succeeded after install. Check
`/var/log/trachuan/watch/watch.log`, Vault access and Telegram delivery. OCI
email alarms remain the independent backstop when the host cannot send alerts.

Check that metadata HTTP is blocked for the application account and permitted
for the publisher account. The first command must fail and the second must
succeed:

```sh
sudo -u trachuan curl -s -m 3 http://169.254.169.254/opc/v2/instance/ >/dev/null
sudo -u trachuan-metrics curl -s -m 3 http://169.254.169.254/opc/v2/instance/ >/dev/null
```

During maintenance, use the OCI Console to suppress the affected alarm for a
bounded interval, then remove the suppression when maintenance ends. Confirm
that the alarm and `trachuan-alarms` subscription remain enabled. See [Oracle's
suppression instructions][suppression].

## Release integrity alarm

The updater found something in `auth` or on the host that disagrees with its
[activation record](../design/activation-record.md#alerts). Treat it as a
possible compromise until you can explain it. A code clears 24 hours after it
was last seen. As `trachuan`:

```sh
sudo python3 -I -m json.tool /srv/trachuan/state/deploy/release.json
sudo python3 -I -m json.tool /var/lib/trachuan-deploy/corpus-activation.json
bash /srv/trachuan/host/auth-psql.sh active-db
bash /srv/trachuan/host/auth-psql.sh <<'SQL'
SELECT c.relname, c.relkind, c.relrowsecurity,
  (SELECT pg_catalog.count(*) FROM pg_catalog.pg_trigger t
    WHERE t.tgrelid OPERATOR(pg_catalog.=) c.oid AND NOT t.tgisinternal),
  (SELECT pg_catalog.count(*) FROM pg_catalog.pg_rewrite r
    WHERE r.ev_class OPERATOR(pg_catalog.=) c.oid),
  (SELECT pg_catalog.count(*) FROM pg_catalog.pg_policy p
    WHERE p.polrelid OPERATOR(pg_catalog.=) c.oid)
FROM pg_catalog.pg_class c
JOIN pg_catalog.pg_namespace n ON n.oid OPERATOR(pg_catalog.=) c.relnamespace
WHERE n.nspname OPERATOR(pg_catalog.=) 'public'
  AND c.relname OPERATOR(pg_catalog.=) ANY (ARRAY['corpus_release',
    'frameworks_release', 'release_journal', 'retired_release']::pg_catalog.name[])
ORDER BY 1;
SQL
```

A healthy result is four rows of the form `<table>|r|f|0|0|0`. Use
`frameworks-activation.json` and `release-frameworks.json` for that kind.

**A forgery or your own manual swap?** `activated_by`, `activated_at` and every
journal row can be written by the server, so they prove nothing. It was a manual
swap only if you ran one yourself and the pointer names the database you chose.
In that case, put the pointer back or re-sign that release the same day, and no
further step is needed. A failed `active-db` read, a pointer you did not set, or
any other row in the check above is a compromise.

| Code                    | Innocent explanation to rule out first                 |
| ----------------------- | ------------------------------------------------------ |
| `pointer_changed`       | your manual swap                                       |
| `serving_database_gone` | none: serving is broken, treat as an incident          |
| `active_cleared`        | you restored the cluster to an earlier time; re-sign   |
| `adoption_failed`       | a re-signed release whose database you changed by hand |
| `record_rebuilt`        | disk damage or your own edit of the file               |
| `journal_row_refused`   | none                                                   |

**If it may be a compromise:**

1. Stop the server: `systemctl --user stop trachuan-server.service`.
2. Stop activations:
   `printf 'verify\n' | sudo tee /etc/trachuan/deploy-mode >/dev/null`.
3. Save the output above outside the host.
4. [Repair access to `auth`](release-restore.md#repair-access-to-auth), then
   rotate every database password ([secrets](host.md#create-secrets)).
5. Remove what was planted, or
   [restore production](backups.md#restore-production) from a backup older than
   the first evidence.
6. Set the pointer back to the record's `active` with a compare-and-swap, as in
   [manual restore](release-restore.md#restore-a-corpus-manually).
7. Return to `activate` the same way and start the server.

[subscription]:
  https://docs.oracle.com/en-us/iaas/Content/Notification/Tasks/managingsubscriptions.htm
[summarize]:
  https://docs.oracle.com/en-us/iaas/tools/oci-cli/latest/oci_cli_docs/cmdref/monitoring/metric-data/summarize-metrics-data.html
[suppression]:
  https://docs.oracle.com/en-us/iaas/Content/Monitoring/Tasks/suppressingalarms.htm
[reset]:
  https://docs.oracle.com/en-us/iaas/Content/Monitoring/Concepts/monitoringoverview.htm#monitoringoverview_topic_About_the_Internal_Reset_Period
