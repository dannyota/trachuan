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

[subscription]:
  https://docs.oracle.com/en-us/iaas/Content/Notification/Tasks/managingsubscriptions.htm
[summarize]:
  https://docs.oracle.com/en-us/iaas/tools/oci-cli/latest/oci_cli_docs/cmdref/monitoring/metric-data/summarize-metrics-data.html
[suppression]:
  https://docs.oracle.com/en-us/iaas/Content/Monitoring/Tasks/suppressingalarms.htm
[reset]:
  https://docs.oracle.com/en-us/iaas/Content/Monitoring/Concepts/monitoringoverview.htm#monitoringoverview_topic_About_the_Internal_Reset_Period
