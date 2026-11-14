data "oci_identity_availability_domain" "host" {
  compartment_id = var.tenancy_ocid
  ad_number      = 1
}

data "oci_objectstorage_namespace" "current" {
  compartment_id = var.tenancy_ocid
}

data "oci_core_private_ips" "host" {
  subnet_id  = oci_core_subnet.public.id
  ip_address = oci_core_instance.host.private_ip
}
