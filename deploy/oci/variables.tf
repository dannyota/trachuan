variable "region" {
  type    = string
  default = "ap-singapore-1"
}

variable "tenancy_ocid" {
  type = string
}

variable "compartment_ocid" {
  type = string
}

variable "alarm_email" {
  type      = string
  sensitive = true
}

variable "instance_image_ocid" {
  type = string
}

variable "identity_emails" {
  type      = map(string)
  sensitive = true
}

variable "sender_email" {
  type = string
}

variable "owner_ssh_rules" {
  type = list(object({
    cidr        = string
    description = string
  }))
}
