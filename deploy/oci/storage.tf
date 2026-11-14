resource "oci_objectstorage_bucket" "releases" {
  access_type           = "NoPublicAccess"
  auto_tiering          = "Disabled"
  bucket_scope          = "NAMESPACE"
  compartment_id        = var.compartment_ocid
  is_bucket_key_enabled = false
  name                  = "trachuan-releases"
  namespace             = data.oci_objectstorage_namespace.current.namespace
  object_events_enabled = false
  storage_tier          = "Standard"
  versioning            = "Disabled"
  lifecycle {
    prevent_destroy = true
  }
}

resource "oci_objectstorage_bucket" "backups" {
  access_type           = "NoPublicAccess"
  auto_tiering          = "Disabled"
  bucket_scope          = "NAMESPACE"
  compartment_id        = var.compartment_ocid
  is_bucket_key_enabled = false
  name                  = "trachuan-backups"
  namespace             = data.oci_objectstorage_namespace.current.namespace
  object_events_enabled = false
  storage_tier          = "Standard"
  versioning            = "Disabled"
  lifecycle {
    prevent_destroy = true
  }
}
