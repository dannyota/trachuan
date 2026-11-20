resource "oci_monitoring_alarm" "corpus_release_failed" {
  body                                          = "Every corpus release attempt for 30 minutes failed; production keeps serving the active corpus. Read release.log in OCI Logging."
  compartment_id                                = var.compartment_ocid
  destinations                                  = [oci_ons_notification_topic.alarms.id]
  display_name                                  = "trachuan corpus release failed"
  evaluation_slack_duration                     = "PT3M"
  is_enabled                                    = true
  is_notifications_per_metric_dimension_enabled = false
  message_format                                = "ONS_OPTIMIZED"
  metric_compartment_id                         = var.compartment_ocid
  metric_compartment_id_in_subtree              = false
  namespace                                     = "trachuan_host"
  notification_title                            = "trachuan corpus release failed"
  notification_version                          = "1.X"
  pending_duration                              = "PT30M"
  query                                         = "release_failed[5m].max() > 0"
  repeat_notification_duration                  = "PT24H"
  resolution                                    = "1m"
  rule_name                                     = "base"
  severity                                      = "WARNING"
}

resource "oci_monitoring_alarm" "corpus_release_waiting" {
  body                                          = "A signed corpus release has waited over a day without activation; check the image's corpus schema support and release.log."
  compartment_id                                = var.compartment_ocid
  destinations                                  = [oci_ons_notification_topic.alarms.id]
  display_name                                  = "trachuan corpus release waiting"
  evaluation_slack_duration                     = "PT3M"
  is_enabled                                    = true
  is_notifications_per_metric_dimension_enabled = false
  message_format                                = "ONS_OPTIMIZED"
  metric_compartment_id                         = var.compartment_ocid
  metric_compartment_id_in_subtree              = false
  namespace                                     = "trachuan_host"
  notification_title                            = "trachuan corpus release waiting"
  notification_version                          = "1.X"
  pending_duration                              = "PT10M"
  query                                         = "release_waiting_age_seconds[5m].min() > 86400"
  repeat_notification_duration                  = "PT24H"
  resolution                                    = "1m"
  rule_name                                     = "base"
  severity                                      = "WARNING"
}

resource "oci_monitoring_alarm" "release_integrity" {
  body                                          = "The updater raised a release integrity alert: something in auth or on the host disagrees with its activation record. Treat it as a possible compromise and follow docs/runbooks/alarms.md."
  compartment_id                                = var.compartment_ocid
  destinations                                  = [oci_ons_notification_topic.alarms.id]
  display_name                                  = "trachuan release integrity"
  evaluation_slack_duration                     = "PT3M"
  is_enabled                                    = true
  is_notifications_per_metric_dimension_enabled = false
  message_format                                = "ONS_OPTIMIZED"
  metric_compartment_id                         = var.compartment_ocid
  metric_compartment_id_in_subtree              = false
  namespace                                     = "trachuan_host"
  notification_title                            = "trachuan release integrity"
  notification_version                          = "1.X"
  pending_duration                              = "PT1M"
  query                                         = "release_integrity[1m].max() > 0"
  repeat_notification_duration                  = "PT4H"
  resolution                                    = "1m"
  rule_name                                     = "base"
  severity                                      = "CRITICAL"
}

resource "oci_monitoring_alarm" "deploy_not_confirmed" {
  body                                          = "The host updater has not confirmed production for over an hour. Read deploy.log in OCI Logging."
  compartment_id                                = var.compartment_ocid
  destinations                                  = [oci_ons_notification_topic.alarms.id]
  display_name                                  = "trachuan deploy not confirmed"
  evaluation_slack_duration                     = "PT3M"
  is_enabled                                    = true
  is_notifications_per_metric_dimension_enabled = false
  message_format                                = "ONS_OPTIMIZED"
  metric_compartment_id                         = var.compartment_ocid
  metric_compartment_id_in_subtree              = false
  namespace                                     = "trachuan_host"
  notification_title                            = "trachuan deploy not confirmed"
  notification_version                          = "1.X"
  pending_duration                              = "PT10M"
  query                                         = "deploy_last_success_age_seconds[5m].min() > 3600"
  repeat_notification_duration                  = "PT24H"
  resolution                                    = "1m"
  rule_name                                     = "base"
  severity                                      = "CRITICAL"
}

resource "oci_monitoring_alarm" "secrets_stale" {
  body                                          = "Vault secrets have not reconciled for over 2 hours; running containers keep their current secrets."
  compartment_id                                = var.compartment_ocid
  destinations                                  = [oci_ons_notification_topic.alarms.id]
  display_name                                  = "trachuan secrets stale"
  evaluation_slack_duration                     = "PT3M"
  is_enabled                                    = true
  is_notifications_per_metric_dimension_enabled = false
  message_format                                = "ONS_OPTIMIZED"
  metric_compartment_id                         = var.compartment_ocid
  metric_compartment_id_in_subtree              = false
  namespace                                     = "trachuan_host"
  notification_title                            = "trachuan secrets stale"
  notification_version                          = "1.X"
  pending_duration                              = "PT10M"
  query                                         = "secrets_last_success_age_seconds[5m].min() > 7200"
  repeat_notification_duration                  = "PT24H"
  resolution                                    = "1m"
  rule_name                                     = "base"
  severity                                      = "WARNING"
}

resource "oci_monitoring_alarm" "secrets_failed" {
  body                                          = "Every secret reconciliation for 30 minutes failed; running containers keep their current secrets. Read deploy.log in OCI Logging."
  compartment_id                                = var.compartment_ocid
  destinations                                  = [oci_ons_notification_topic.alarms.id]
  display_name                                  = "trachuan secrets failed"
  evaluation_slack_duration                     = "PT3M"
  is_enabled                                    = true
  is_notifications_per_metric_dimension_enabled = false
  message_format                                = "ONS_OPTIMIZED"
  metric_compartment_id                         = var.compartment_ocid
  metric_compartment_id_in_subtree              = false
  namespace                                     = "trachuan_host"
  notification_title                            = "trachuan secrets failed"
  notification_version                          = "1.X"
  pending_duration                              = "PT30M"
  query                                         = "secrets_failed[5m].max() > 0"
  repeat_notification_duration                  = "PT24H"
  resolution                                    = "1m"
  rule_name                                     = "base"
  severity                                      = "WARNING"
}

resource "oci_monitoring_alarm" "telegram_watch_silent" {
  body                                          = "No successful host watch run within an hour, or none since install; Telegram alerts may be blocked. Check watch.log in OCI Logging and docs/runbooks/alarms.md."
  compartment_id                                = var.compartment_ocid
  destinations                                  = [oci_ons_notification_topic.alarms.id]
  display_name                                  = "Telegram watch silent"
  evaluation_slack_duration                     = "PT3M"
  is_enabled                                    = true
  is_notifications_per_metric_dimension_enabled = false
  message_format                                = "ONS_OPTIMIZED"
  metric_compartment_id                         = var.compartment_ocid
  metric_compartment_id_in_subtree              = false
  namespace                                     = "trachuan_host"
  notification_title                            = "Telegram watch silent"
  notification_version                          = "1.X"
  pending_duration                              = "PT10M"
  query                                         = "watch_stale[5m].min() > 0"
  repeat_notification_duration                  = "PT24H"
  resolution                                    = "1m"
  rule_name                                     = "base"
  severity                                      = "WARNING"
}
