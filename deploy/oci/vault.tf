resource "oci_kms_key" "secrets" {
  compartment_id           = var.compartment_ocid
  desired_state            = "ENABLED"
  display_name             = "trachuan-secrets"
  is_auto_rotation_enabled = false
  management_endpoint      = oci_kms_vault.main.management_endpoint
  protection_mode          = "SOFTWARE"
  key_shape {
    algorithm = "AES"
    length    = 32
  }
  lifecycle {
    prevent_destroy = true
  }
}

resource "oci_kms_vault" "main" {
  compartment_id = var.compartment_ocid
  display_name   = "trachuan-vault"
  vault_type     = "DEFAULT"
  lifecycle {
    prevent_destroy = true
  }
}
