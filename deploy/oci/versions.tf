terraform {
  required_version = "1.13.9"

  required_providers {
    oci = {
      source  = "oracle/oci"
      version = "9.11.0"
    }
  }

  backend "s3" {
    bucket                      = "trachuan-tfstate"
    key                         = "oci/terraform.tfstate"
    region                      = "ap-singapore-1"
    endpoints                   = { s3 = "https://axj58m4cpgbr.compat.objectstorage.ap-singapore-1.oraclecloud.com" }
    use_path_style              = true
    skip_credentials_validation = true
    skip_region_validation      = true
    skip_requesting_account_id  = true
    skip_metadata_api_check     = true
    skip_s3_checksum            = true
  }
}

provider "oci" {
  region = var.region
}
