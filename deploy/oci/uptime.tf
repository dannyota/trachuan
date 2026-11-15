resource "oci_health_checks_http_monitor" "website" {
  compartment_id      = var.compartment_ocid
  display_name        = "trachuan website uptime"
  interval_in_seconds = 60
  is_enabled          = true
  method              = "GET"
  path                = "/"
  port                = 443
  protocol            = "HTTPS"
  targets             = ["trachuan.vn"]
  timeout_in_seconds  = 30
}

resource "oci_health_checks_http_monitor" "public_status" {
  compartment_id      = var.compartment_ocid
  display_name        = "trachuan public status uptime"
  interval_in_seconds = 60
  is_enabled          = true
  method              = "GET"
  path                = "/api/v1/public/status"
  port                = 443
  protocol            = "HTTPS"
  targets             = ["trachuan.vn"]
  timeout_in_seconds  = 30
}

resource "oci_monitoring_alarm" "website_unavailable" {
  body                                          = "https://trachuan.vn/ failed from every vantage point for 5 minutes."
  compartment_id                                = var.compartment_ocid
  destinations                                  = [oci_ons_notification_topic.alarms.id]
  display_name                                  = "trachuan website unavailable"
  evaluation_slack_duration                     = "PT3M"
  is_enabled                                    = true
  is_notifications_per_metric_dimension_enabled = false
  message_format                                = "ONS_OPTIMIZED"
  metric_compartment_id                         = var.compartment_ocid
  metric_compartment_id_in_subtree              = false
  namespace                                     = "oci_healthchecks"
  notification_title                            = "trachuan website unavailable"
  notification_version                          = "1.X"
  pending_duration                              = "PT5M"
  query                                         = "HTTP.isHealthy[1m]{resourceId = \"${oci_health_checks_http_monitor.website.id}\"}.grouping().max() < 1"
  repeat_notification_duration                  = "PT4H"
  resolution                                    = "1m"
  rule_name                                     = "base"
  severity                                      = "CRITICAL"
}

resource "oci_monitoring_alarm" "public_status_unavailable" {
  body                                          = "https://trachuan.vn/api/v1/public/status failed from every vantage point for 5 minutes."
  compartment_id                                = var.compartment_ocid
  destinations                                  = [oci_ons_notification_topic.alarms.id]
  display_name                                  = "trachuan public status unavailable"
  evaluation_slack_duration                     = "PT3M"
  is_enabled                                    = true
  is_notifications_per_metric_dimension_enabled = false
  message_format                                = "ONS_OPTIMIZED"
  metric_compartment_id                         = var.compartment_ocid
  metric_compartment_id_in_subtree              = false
  namespace                                     = "oci_healthchecks"
  notification_title                            = "trachuan public status unavailable"
  notification_version                          = "1.X"
  pending_duration                              = "PT5M"
  query                                         = "HTTP.isHealthy[1m]{resourceId = \"${oci_health_checks_http_monitor.public_status.id}\"}.grouping().max() < 1"
  repeat_notification_duration                  = "PT4H"
  resolution                                    = "1m"
  rule_name                                     = "base"
  severity                                      = "CRITICAL"
}
