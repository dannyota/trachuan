resource "oci_identity_user" "smtp" {
  compartment_id = var.tenancy_ocid
  description    = "trachuan server SMTP submission (no console, no API keys)"
  email          = var.identity_emails["smtp"]
  name           = "trachuan-smtp"
}

resource "oci_identity_group" "tfstate" {
  compartment_id = var.tenancy_ocid
  description    = "OpenTofu state for trachuan"
  name           = "trachuan-tfstate"
}

resource "oci_identity_dynamic_group" "host" {
  compartment_id = var.tenancy_ocid
  description    = "the trachuan production instance"
  matching_rule  = "Any {instance.id = '${oci_core_instance.host.id}'}"
  name           = "trachuan-host"
}

resource "oci_identity_policy" "host_metrics" {
  compartment_id = var.tenancy_ocid
  description    = "host timer publishes custom metrics"
  name           = "trachuan-host-metrics"
  statements     = ["Allow dynamic-group trachuan-host to use metrics in compartment trachuan where target.metrics.namespace = 'trachuan_host'"]
}

resource "oci_identity_policy" "tfstate" {
  compartment_id = var.tenancy_ocid
  description    = "OpenTofu state bucket only"
  name           = "trachuan-tfstate"
  statements     = ["Allow group trachuan-tfstate to read buckets in compartment trachuan where target.bucket.name = 'trachuan-tfstate'", "Allow group trachuan-tfstate to manage objects in compartment trachuan where target.bucket.name = 'trachuan-tfstate'"]
}

resource "oci_identity_user" "backup" {
  compartment_id = var.tenancy_ocid
  description    = "trachuan pgBackRest (customer secret key only)"
  email          = var.identity_emails["backup"]
  name           = "trachuan-backup"
}

resource "oci_identity_policy" "smtp" {
  compartment_id = var.tenancy_ocid
  description    = "trachuan SMTP submission"
  name           = "trachuan-smtp"
  statements     = ["Allow group trachuan-smtp to use email-family in compartment trachuan"]
}

resource "oci_identity_group" "smtp" {
  compartment_id = var.tenancy_ocid
  description    = "trachuan server SMTP submission"
  name           = "trachuan-smtp"
}

resource "oci_identity_user_group_membership" "release_verify" {
  compartment_id = var.tenancy_ocid
  group_id       = oci_identity_group.release_verify.id
  user_id        = oci_identity_user.release_verify.id
}

resource "oci_identity_group" "release_upload" {
  compartment_id = var.tenancy_ocid
  description    = "trachuan trachuan-release-upload"
  name           = "trachuan-release-upload"
}

resource "oci_identity_policy" "backup" {
  compartment_id = var.tenancy_ocid
  description    = "pgBackRest access to the backup bucket only"
  name           = "trachuan-backup"
  statements     = ["Allow group trachuan-backup to read buckets in compartment trachuan where target.bucket.name = 'trachuan-backups'", "Allow group trachuan-backup to manage objects in compartment trachuan where target.bucket.name = 'trachuan-backups'"]
}

resource "oci_identity_policy" "releases" {
  compartment_id = var.tenancy_ocid
  description    = "corpus release upload and verify"
  name           = "trachuan-releases"
  statements     = ["Allow group trachuan-release-upload to read buckets in compartment trachuan where target.bucket.name = 'trachuan-releases'", "Allow group trachuan-release-upload to manage objects in compartment trachuan where all {target.bucket.name = 'trachuan-releases', any {request.permission = 'OBJECT_CREATE', request.permission = 'OBJECT_OVERWRITE', request.permission = 'OBJECT_INSPECT', request.permission = 'OBJECT_READ'}}", "Allow group trachuan-release-verify to read buckets in compartment trachuan where target.bucket.name = 'trachuan-releases'", "Allow group trachuan-release-verify to read objects in compartment trachuan where target.bucket.name = 'trachuan-releases'"]
}

resource "oci_identity_group" "backup" {
  compartment_id = var.tenancy_ocid
  description    = "trachuan pgBackRest repository access"
  name           = "trachuan-backup"
}

resource "oci_identity_user_group_membership" "backup" {
  compartment_id = var.tenancy_ocid
  group_id       = oci_identity_group.backup.id
  user_id        = oci_identity_user.backup.id
}

resource "oci_identity_policy" "host_deploy" {
  compartment_id = var.tenancy_ocid
  description    = "host reads its named secrets and corpus releases"
  name           = "trachuan-host-deploy"
  statements     = ["Allow dynamic-group trachuan-host to read secret-bundles in compartment trachuan where any { target.secret.name = 'google_client_secret', target.secret.name = 'smtp_username', target.secret.name = 'smtp_password', target.secret.name = 'account_keys', target.secret.name = 'postgres_password', target.secret.name = 'auth_db_password', target.secret.name = 'corpus_reader_password', target.secret.name = 'activator_db_password', target.secret.name = 'backup_s3_key', target.secret.name = 'backup_s3_key_secret', target.secret.name = 'server_signup_emails'}", "Allow dynamic-group trachuan-host to read objects in compartment trachuan where target.bucket.name = 'trachuan-releases'", "Allow dynamic-group trachuan-host to read buckets in compartment trachuan where target.bucket.name = 'trachuan-releases'", "Allow dynamic-group trachuan-host to use log-content in compartment trachuan"]
}

resource "oci_identity_user_group_membership" "tfstate" {
  compartment_id = var.tenancy_ocid
  group_id       = oci_identity_group.tfstate.id
  user_id        = oci_identity_user.tfstate.id
}

resource "oci_identity_user_group_membership" "smtp" {
  compartment_id = var.tenancy_ocid
  group_id       = oci_identity_group.smtp.id
  user_id        = oci_identity_user.smtp.id
}

resource "oci_identity_user" "tfstate" {
  compartment_id = var.tenancy_ocid
  description    = "OpenTofu state backend (customer secret key only)"
  email          = var.identity_emails["tfstate"]
  name           = "trachuan-tfstate"
}

resource "oci_identity_user" "release_verify" {
  compartment_id = var.tenancy_ocid
  description    = "trachuan trachuan-release-verify (customer secret key only)"
  email          = var.identity_emails["release_verify"]
  name           = "trachuan-release-verify"
}

resource "oci_identity_group" "release_verify" {
  compartment_id = var.tenancy_ocid
  description    = "trachuan trachuan-release-verify"
  name           = "trachuan-release-verify"
}

resource "oci_identity_user" "release_upload" {
  compartment_id = var.tenancy_ocid
  description    = "trachuan trachuan-release-upload (customer secret key only)"
  email          = var.identity_emails["release_upload"]
  name           = "trachuan-release-upload"
}

resource "oci_identity_user_group_membership" "release_upload" {
  compartment_id = var.tenancy_ocid
  group_id       = oci_identity_group.release_upload.id
  user_id        = oci_identity_user.release_upload.id
}
