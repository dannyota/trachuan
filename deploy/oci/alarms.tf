resource "oci_ons_notification_topic" "alarms" {
  compartment_id = var.compartment_ocid
  description    = "trachuan host alarms"
  name           = "trachuan-alarms"
}

resource "oci_monitoring_alarm" "backup_job_failed" {
  body                                          = "A backup job reported failed or unknown."
  compartment_id                                = var.compartment_ocid
  destinations                                  = [oci_ons_notification_topic.alarms.id]
  display_name                                  = "trachuan backup job failed"
  evaluation_slack_duration                     = "PT3M"
  is_enabled                                    = true
  is_notifications_per_metric_dimension_enabled = false
  message_format                                = "ONS_OPTIMIZED"
  metric_compartment_id                         = var.compartment_ocid
  metric_compartment_id_in_subtree              = false
  namespace                                     = "trachuan_host"
  notification_title                            = "trachuan backup job failed"
  notification_version                          = "1.X"
  pending_duration                              = "PT5M"
  query                                         = "backup_failed[5m].max() > 0"
  repeat_notification_duration                  = "PT4H"
  resolution                                    = "1m"
  rule_name                                     = "base"
  severity                                      = "CRITICAL"
}

resource "oci_monitoring_alarm" "instance_not_accessible" {
  body                                          = "Instance accessibility status is nonzero."
  compartment_id                                = var.compartment_ocid
  destinations                                  = [oci_ons_notification_topic.alarms.id]
  display_name                                  = "trachuan instance not accessible"
  evaluation_slack_duration                     = "PT3M"
  is_enabled                                    = true
  is_notifications_per_metric_dimension_enabled = false
  message_format                                = "ONS_OPTIMIZED"
  metric_compartment_id                         = var.compartment_ocid
  metric_compartment_id_in_subtree              = false
  namespace                                     = "oci_compute_instance_health"
  notification_title                            = "trachuan instance not accessible"
  notification_version                          = "1.X"
  pending_duration                              = "PT1M"
  query                                         = "instance_accessibility_status[1m].max() > 0"
  repeat_notification_duration                  = "PT4H"
  resolution                                    = "1m"
  rule_name                                     = "base"
  severity                                      = "CRITICAL"
}

resource "oci_monitoring_alarm" "data_volume_not_mounted" {
  body                                          = "/srv/trachuan/data is not mounted."
  compartment_id                                = var.compartment_ocid
  destinations                                  = [oci_ons_notification_topic.alarms.id]
  display_name                                  = "trachuan data volume not mounted"
  evaluation_slack_duration                     = "PT3M"
  is_enabled                                    = true
  is_notifications_per_metric_dimension_enabled = false
  message_format                                = "ONS_OPTIMIZED"
  metric_compartment_id                         = var.compartment_ocid
  metric_compartment_id_in_subtree              = false
  namespace                                     = "trachuan_host"
  notification_title                            = "trachuan data volume not mounted"
  notification_version                          = "1.X"
  pending_duration                              = "PT2M"
  query                                         = "data_mounted[1m].min() < 1"
  repeat_notification_duration                  = "PT4H"
  resolution                                    = "1m"
  rule_name                                     = "base"
  severity                                      = "CRITICAL"
}

resource "oci_monitoring_alarm" "backup_result_absent" {
  body                                          = "No successful full backup is recorded: the backup jobs never ran or their result file is gone."
  compartment_id                                = var.compartment_ocid
  destinations                                  = [oci_ons_notification_topic.alarms.id]
  display_name                                  = "trachuan backup result absent"
  evaluation_slack_duration                     = "PT3M"
  is_enabled                                    = true
  is_notifications_per_metric_dimension_enabled = false
  message_format                                = "ONS_OPTIMIZED"
  metric_compartment_id                         = var.compartment_ocid
  metric_compartment_id_in_subtree              = false
  namespace                                     = "trachuan_host"
  notification_title                            = "trachuan backup result absent"
  notification_version                          = "1.X"
  pending_duration                              = "PT15M"
  query                                         = "backup_last_success_age_seconds[5m]{job = \"full\"}.absent()"
  repeat_notification_duration                  = "PT4H"
  resolution                                    = "1m"
  rule_name                                     = "base"
  severity                                      = "CRITICAL"
}

resource "oci_monitoring_alarm" "agent_metrics_absent" {
  body                                          = "No agent metrics for 5 minutes: host down or agent stopped."
  compartment_id                                = var.compartment_ocid
  destinations                                  = [oci_ons_notification_topic.alarms.id]
  display_name                                  = "trachuan agent metrics absent"
  evaluation_slack_duration                     = "PT3M"
  is_enabled                                    = true
  is_notifications_per_metric_dimension_enabled = false
  message_format                                = "ONS_OPTIMIZED"
  metric_compartment_id                         = var.compartment_ocid
  metric_compartment_id_in_subtree              = false
  namespace                                     = "oci_computeagent"
  notification_title                            = "trachuan agent metrics absent"
  notification_version                          = "1.X"
  pending_duration                              = "PT5M"
  query                                         = "MemoryUtilization[1m].absent()"
  repeat_notification_duration                  = "PT4H"
  resolution                                    = "1m"
  rule_name                                     = "base"
  severity                                      = "CRITICAL"
}

resource "oci_ons_subscription" "alarm_email" {
  compartment_id = var.compartment_ocid
  delivery_policy = jsonencode({
    backoffRetryPolicy = {
      initialDelayInFailureRetry = 60000
      maxRetryDuration           = 7200000
      policyType                 = "EXPONENTIAL"
    }
    maxReceiveRatePerSecond = 0
  })
  endpoint = var.alarm_email
  protocol = "EMAIL"
  topic_id = oci_ons_notification_topic.alarms.id
}

resource "oci_monitoring_alarm" "memory_above_90pct" {
  body                                          = "Host memory above 90% for 5 minutes."
  compartment_id                                = var.compartment_ocid
  destinations                                  = [oci_ons_notification_topic.alarms.id]
  display_name                                  = "trachuan memory above 90%"
  evaluation_slack_duration                     = "PT3M"
  is_enabled                                    = true
  is_notifications_per_metric_dimension_enabled = false
  message_format                                = "ONS_OPTIMIZED"
  metric_compartment_id                         = var.compartment_ocid
  metric_compartment_id_in_subtree              = false
  namespace                                     = "oci_computeagent"
  notification_title                            = "trachuan memory above 90%"
  notification_version                          = "1.X"
  pending_duration                              = "PT5M"
  query                                         = "MemoryUtilization[1m].mean() > 90"
  repeat_notification_duration                  = "PT4H"
  resolution                                    = "1m"
  rule_name                                     = "base"
  severity                                      = "CRITICAL"
}

resource "oci_monitoring_alarm" "heartbeat_absent" {
  body                                          = "The host metrics timer has not reported for 5 minutes."
  compartment_id                                = var.compartment_ocid
  destinations                                  = [oci_ons_notification_topic.alarms.id]
  display_name                                  = "trachuan heartbeat absent"
  evaluation_slack_duration                     = "PT3M"
  is_enabled                                    = true
  is_notifications_per_metric_dimension_enabled = false
  message_format                                = "ONS_OPTIMIZED"
  metric_compartment_id                         = var.compartment_ocid
  metric_compartment_id_in_subtree              = false
  namespace                                     = "trachuan_host"
  notification_title                            = "trachuan heartbeat absent"
  notification_version                          = "1.X"
  pending_duration                              = "PT5M"
  query                                         = "heartbeat[1m].absent()"
  repeat_notification_duration                  = "PT4H"
  resolution                                    = "1m"
  rule_name                                     = "base"
  severity                                      = "CRITICAL"
}

resource "oci_monitoring_alarm" "deploy_result_absent" {
  body                                          = "The host updater has not reported a successful run."
  compartment_id                                = var.compartment_ocid
  destinations                                  = [oci_ons_notification_topic.alarms.id]
  display_name                                  = "trachuan deploy result absent"
  evaluation_slack_duration                     = "PT3M"
  is_enabled                                    = true
  is_notifications_per_metric_dimension_enabled = false
  message_format                                = "ONS_OPTIMIZED"
  metric_compartment_id                         = var.compartment_ocid
  metric_compartment_id_in_subtree              = false
  namespace                                     = "trachuan_host"
  notification_title                            = "trachuan deploy result absent"
  notification_version                          = "1.X"
  pending_duration                              = "PT30M"
  query                                         = "deploy_last_success_age_seconds[10m].absent()"
  repeat_notification_duration                  = "PT4H"
  resolution                                    = "1m"
  rule_name                                     = "base"
  severity                                      = "CRITICAL"
}

resource "oci_monitoring_alarm" "backup_older_than_26h" {
  body                                          = "The newest successful full or differential backup is older than 26 hours."
  compartment_id                                = var.compartment_ocid
  destinations                                  = [oci_ons_notification_topic.alarms.id]
  display_name                                  = "trachuan backup older than 26h"
  evaluation_slack_duration                     = "PT3M"
  is_enabled                                    = true
  is_notifications_per_metric_dimension_enabled = false
  message_format                                = "ONS_OPTIMIZED"
  metric_compartment_id                         = var.compartment_ocid
  metric_compartment_id_in_subtree              = false
  namespace                                     = "trachuan_host"
  notification_title                            = "trachuan backup older than 26h"
  notification_version                          = "1.X"
  pending_duration                              = "PT10M"
  query                                         = "backup_last_success_age_seconds[5m]{job =~ \"full|diff\"}.grouping().min() > 93600"
  repeat_notification_duration                  = "PT4H"
  resolution                                    = "1m"
  rule_name                                     = "base"
  severity                                      = "CRITICAL"
}

resource "oci_monitoring_alarm" "deploy_failed" {
  body                                          = "The host updater reported a failed deployment run; see /var/log/trachuan/deploy.log through OCI Logging."
  compartment_id                                = var.compartment_ocid
  destinations                                  = [oci_ons_notification_topic.alarms.id]
  display_name                                  = "trachuan deploy failed"
  evaluation_slack_duration                     = "PT3M"
  is_enabled                                    = true
  is_notifications_per_metric_dimension_enabled = false
  message_format                                = "ONS_OPTIMIZED"
  metric_compartment_id                         = var.compartment_ocid
  metric_compartment_id_in_subtree              = false
  namespace                                     = "trachuan_host"
  notification_title                            = "trachuan deploy failed"
  notification_version                          = "1.X"
  pending_duration                              = "PT5M"
  query                                         = "deploy_failed[5m].max() > 0"
  repeat_notification_duration                  = "PT4H"
  resolution                                    = "1m"
  rule_name                                     = "base"
  severity                                      = "CRITICAL"
}

resource "oci_monitoring_alarm" "filesystem_above_70pct" {
  body                                          = "A host filesystem is above 70% used for 5 minutes."
  compartment_id                                = var.compartment_ocid
  destinations                                  = [oci_ons_notification_topic.alarms.id]
  display_name                                  = "trachuan filesystem above 70%"
  evaluation_slack_duration                     = "PT3M"
  is_enabled                                    = true
  is_notifications_per_metric_dimension_enabled = false
  message_format                                = "ONS_OPTIMIZED"
  metric_compartment_id                         = var.compartment_ocid
  metric_compartment_id_in_subtree              = false
  namespace                                     = "trachuan_host"
  notification_title                            = "trachuan filesystem above 70%"
  notification_version                          = "1.X"
  pending_duration                              = "PT5M"
  query                                         = "filesystem_used_percent[1m]{mount = \"/\"}.max() > 70 || filesystem_used_percent[1m]{mount = \"/srv/trachuan/data\"}.max() > 70"
  repeat_notification_duration                  = "PT4H"
  resolution                                    = "1m"
  rule_name                                     = "base"
  severity                                      = "CRITICAL"
}

resource "oci_monitoring_alarm" "infrastructure_status" {
  body                                          = "Infrastructure instance status is nonzero."
  compartment_id                                = var.compartment_ocid
  destinations                                  = [oci_ons_notification_topic.alarms.id]
  display_name                                  = "trachuan infrastructure status"
  evaluation_slack_duration                     = "PT3M"
  is_enabled                                    = true
  is_notifications_per_metric_dimension_enabled = false
  message_format                                = "ONS_OPTIMIZED"
  metric_compartment_id                         = var.compartment_ocid
  metric_compartment_id_in_subtree              = false
  namespace                                     = "oci_compute_infrastructure_health"
  notification_title                            = "trachuan infrastructure status"
  notification_version                          = "1.X"
  pending_duration                              = "PT1M"
  query                                         = "instance_status[1m].max() > 0"
  repeat_notification_duration                  = "PT4H"
  resolution                                    = "1m"
  rule_name                                     = "base"
  severity                                      = "CRITICAL"
}
