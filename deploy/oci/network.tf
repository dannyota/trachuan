resource "oci_core_default_security_list" "main" {
  compartment_id = var.compartment_ocid
  display_name   = "Default Security List for trachuan-vcn"
  freeform_tags = {
    project = "trachuan"
  }
  manage_default_resource_id = oci_core_vcn.main.default_security_list_id
  egress_security_rules {
    destination      = "0.0.0.0/0"
    destination_type = "CIDR_BLOCK"
    protocol         = "all"
    stateless        = false
  }
  ingress_security_rules {
    protocol    = "1"
    source      = "10.20.0.0/16"
    source_type = "CIDR_BLOCK"
    stateless   = false
    icmp_options {
      code = -1
      type = 3
    }
  }
  ingress_security_rules {
    protocol    = "1"
    source      = "0.0.0.0/0"
    source_type = "CIDR_BLOCK"
    stateless   = false
    icmp_options {
      code = 4
      type = 3
    }
  }
  ingress_security_rules {
    protocol    = "6"
    source      = "0.0.0.0/0"
    source_type = "CIDR_BLOCK"
    stateless   = false
    tcp_options {
      max = 22
      min = 22
    }
  }
}

resource "oci_core_vcn" "main" {
  cidr_block     = "10.20.0.0/16"
  cidr_blocks    = ["10.20.0.0/16"]
  compartment_id = var.compartment_ocid
  display_name   = "trachuan-vcn"
  dns_label      = "trachuan"
  freeform_tags = {
    project = "trachuan"
  }
  is_ipv6enabled = false
}

resource "oci_core_security_list" "host" {
  compartment_id = var.compartment_ocid
  display_name   = "trachuan-sl"
  freeform_tags = {
    project = "trachuan"
  }
  vcn_id = oci_core_vcn.main.id
  egress_security_rules {
    destination      = "0.0.0.0/0"
    destination_type = "CIDR_BLOCK"
    protocol         = "all"
    stateless        = false
  }
  ingress_security_rules {
    protocol    = "1"
    source      = "0.0.0.0/0"
    source_type = "CIDR_BLOCK"
    stateless   = false
  }
  ingress_security_rules {
    description = "HTTP for ACME and redirect"
    protocol    = "6"
    source      = "0.0.0.0/0"
    source_type = "CIDR_BLOCK"
    stateless   = false
    tcp_options {
      max = 80
      min = 80
    }
  }
  ingress_security_rules {
    description = "HTTPS"
    protocol    = "6"
    source      = "0.0.0.0/0"
    source_type = "CIDR_BLOCK"
    stateless   = false
    tcp_options {
      max = 443
      min = 443
    }
  }
  ingress_security_rules {
    description = "HTTP/3"
    protocol    = "17"
    source      = "0.0.0.0/0"
    source_type = "CIDR_BLOCK"
    stateless   = false
    udp_options {
      max = 443
      min = 443
    }
  }
  dynamic "ingress_security_rules" {
    for_each = var.owner_ssh_rules
    content {
      description = ingress_security_rules.value.description
      protocol    = "6"
      source      = ingress_security_rules.value.cidr
      source_type = "CIDR_BLOCK"
      stateless   = false
      tcp_options {
        max = 22
        min = 22
      }
    }
  }
}

resource "oci_core_default_dhcp_options" "main" {
  compartment_id   = var.compartment_ocid
  display_name     = "Default DHCP Options for trachuan-vcn"
  domain_name_type = "CUSTOM_DOMAIN"
  freeform_tags = {
    project = "trachuan"
  }
  manage_default_resource_id = oci_core_vcn.main.default_dhcp_options_id
  options {
    search_domain_names = ["trachuan.oraclevcn.com"]
    type                = "SearchDomain"
  }
  options {
    server_type = "VcnLocalPlusInternet"
    type        = "DomainNameServer"
  }
}

resource "oci_core_internet_gateway" "main" {
  compartment_id = var.compartment_ocid
  display_name   = "trachuan-igw"
  enabled        = true
  freeform_tags = {
    project = "trachuan"
  }
  vcn_id = oci_core_vcn.main.id
}

resource "oci_core_default_route_table" "main" {
  compartment_id = var.compartment_ocid
  display_name   = "Default Route Table for trachuan-vcn"
  freeform_tags = {
    project = "trachuan"
  }
  manage_default_resource_id = oci_core_vcn.main.default_route_table_id
}

resource "oci_core_subnet" "public" {
  cidr_block      = "10.20.1.0/24"
  compartment_id  = var.compartment_ocid
  dhcp_options_id = oci_core_default_dhcp_options.main.id
  display_name    = "trachuan-public"
  dns_label       = "pub"
  freeform_tags = {
    project = "trachuan"
  }
  ipv4cidr_blocks            = ["10.20.1.0/24"]
  prohibit_internet_ingress  = false
  prohibit_public_ip_on_vnic = false
  route_table_id             = oci_core_route_table.public.id
  security_list_ids          = [oci_core_security_list.host.id]
  vcn_id                     = oci_core_vcn.main.id
}

resource "oci_core_route_table" "public" {
  compartment_id = var.compartment_ocid
  display_name   = "trachuan-rt"
  freeform_tags = {
    project = "trachuan"
  }
  vcn_id = oci_core_vcn.main.id
  route_rules {
    destination       = "0.0.0.0/0"
    destination_type  = "CIDR_BLOCK"
    network_entity_id = oci_core_internet_gateway.main.id
    route_type        = "STATIC"
  }
}
