resource "oci_email_email_domain" "main" {
  compartment_id = var.compartment_ocid
  description    = "trachuan account mail"
  name           = "trachuan.vn"
}

resource "oci_email_sender" "no_reply" {
  compartment_id = var.compartment_ocid
  email_address  = var.sender_email
}

resource "oci_email_dkim" "main" {
  email_domain_id = oci_email_email_domain.main.id
  name            = "trachuan-202611"
}
