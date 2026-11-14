resource "oci_logging_log_group" "main" {
  compartment_id = var.compartment_ocid
  description    = "trachuan host job logs"
  display_name   = "trachuan-logs"
}

resource "oci_logging_log" "host_jobs" {
  display_name       = "trachuan-host-jobs"
  is_enabled         = true
  log_group_id       = oci_logging_log_group.main.id
  log_type           = "CUSTOM"
  retention_duration = 30
}

resource "oci_logging_unified_agent_configuration" "host_jobs" {
  compartment_id = var.compartment_ocid
  description    = "host job logs"
  display_name   = "trachuan-host-jobs"
  is_enabled     = true
  group_association {
    group_list = [oci_identity_dynamic_group.host.id]
  }
  service_configuration {
    configuration_type = "LOGGING"
    destination {
      log_object_id = oci_logging_log.host_jobs.id
    }
    sources {
      name        = "trachuan_jobs"
      paths       = ["/var/log/trachuan/*.log"]
      source_type = "LOG_TAIL"
      parser {
        is_estimate_current_event  = false
        is_keep_time_key           = false
        is_merge_cri_fields        = false
        is_null_empty_string       = false
        is_support_colonless_ident = false
        is_with_priority           = false
        parse_nested               = false
        parser_type                = "JSON"
        timeout_in_milliseconds    = 0
      }
    }
  }
}
