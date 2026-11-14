resource "oci_core_public_ip" "host" {
  compartment_id = var.compartment_ocid
  display_name   = "trachuan-prod"
  lifetime       = "RESERVED"
  private_ip_id  = data.oci_core_private_ips.host.private_ips[0].id
  lifecycle {
    prevent_destroy = true
  }
}

resource "oci_core_volume_attachment" "data" {
  attachment_type                     = "paravirtualized"
  device                              = "/dev/oracleoci/oraclevdb"
  display_name                        = "trachuan-data"
  instance_id                         = oci_core_instance.host.id
  is_pv_encryption_in_transit_enabled = false
  is_read_only                        = false
  is_shareable                        = false
  volume_id                           = oci_core_volume.data.id
  lifecycle {
    prevent_destroy = true
  }
}

resource "oci_core_instance" "host" {
  availability_domain = data.oci_identity_availability_domain.host.name
  compartment_id      = var.compartment_ocid
  display_name        = "trachuan-host"
  fault_domain        = "FAULT-DOMAIN-3"
  freeform_tags = {
    project = "trachuan"
  }
  shape = "VM.Standard.A1.Flex"
  state = "RUNNING"
  agent_config {
    are_all_plugins_disabled = false
    is_management_disabled   = false
    is_monitoring_disabled   = false
    plugins_config {
      desired_state = "DISABLED"
      name          = "Compute Instance Run Command"
    }
    plugins_config {
      desired_state = "ENABLED"
      name          = "Compute Instance Monitoring"
    }
    plugins_config {
      desired_state = "ENABLED"
      name          = "Custom Logs Monitoring"
    }
  }
  availability_config {
    is_live_migration_preferred = false
    recovery_action             = "RESTORE_INSTANCE"
  }
  create_vnic_details {
    assign_ipv6ip             = false
    assign_private_dns_record = false
    assign_public_ip          = "true"
    display_name              = "trachuan-host"
    freeform_tags = {
      project = "trachuan"
    }
    hostname_label         = "trachuan-host"
    private_ip             = "10.20.1.253"
    skip_source_dest_check = false
    subnet_id              = oci_core_subnet.public.id
  }
  instance_options {
    are_legacy_imds_endpoints_disabled = false
  }
  launch_options {
    boot_volume_type                    = "PARAVIRTUALIZED"
    firmware                            = "UEFI_64"
    is_consistent_volume_naming_enabled = true
    is_encryption_in_transit_enabled    = false
    is_pv_encryption_in_transit_enabled = false
    network_type                        = "PARAVIRTUALIZED"
    remote_data_volume_type             = "PARAVIRTUALIZED"
  }
  shape_config {
    local_volume_size_in_gbs = 0
    memory_in_gbs            = 24
    nvmes                    = 0
    ocpus                    = 4
    vcpus                    = 4
  }
  source_details {
    boot_volume_size_in_gbs         = "100"
    boot_volume_vpus_per_gb         = "10"
    is_preserve_boot_volume_enabled = false
    source_id                       = var.instance_image_ocid
    source_type                     = "image"
  }
  lifecycle {
    prevent_destroy = true
    ignore_changes  = [metadata, source_details, create_vnic_details]
  }
}

resource "oci_core_boot_volume" "host" {
  availability_domain = data.oci_identity_availability_domain.host.name
  compartment_id      = var.compartment_ocid
  display_name        = "trachuan-host (Boot Volume)"
  freeform_tags = {
    project = "trachuan"
  }
  is_auto_tune_enabled = false
  size_in_gbs          = "100"
  vpus_per_gb          = "10"
  source_details {
    type = "bootVolume"
  }
  lifecycle {
    prevent_destroy = true
    ignore_changes  = [source_details]
  }
}

resource "oci_core_volume" "data" {
  availability_domain     = data.oci_identity_availability_domain.host.name
  compartment_id          = var.compartment_ocid
  display_name            = "trachuan-data"
  is_auto_tune_enabled    = false
  is_reservations_enabled = false
  size_in_gbs             = "60"
  vpus_per_gb             = "10"
  lifecycle {
    prevent_destroy = true
  }
}
