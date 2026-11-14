# Host alarms

OCI Monitoring sends alarms to the Notifications topic `trachuan-alarms`.
Confirm the email subscription from the confirmation message. In the OCI
Console, open Notifications, select `trachuan-alarms`, and check that the
subscription is active. See [Oracle's subscription instructions][subscription].

The primary configures the custom alarms after installing the host bundle.
Thresholds follow [host budgets](../design/budgets-host.md#host-alarms-proposed)
and the [backup design](../design/deployment.md#backups).

| Signal                                           | Trigger                                               |
| ------------------------------------------------ | ----------------------------------------------------- |
| Boot and data filesystem use                     | Above 70% for 5 minutes, separately                   |
| Memory utilization                               | Above 90% for 5 minutes                               |
| Instance accessibility and infrastructure status | Nonzero for 1 minute                                  |
| Host heartbeat or expected agent metric stream   | Absent for 5 minutes                                  |
| Newest backup                                    | Last success older than 26 hours                      |
| Data volume mount                                | `data_mounted` is 0                                   |
| Backup job                                       | `backup_failed` is 1 or last-success metric is absent |

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
