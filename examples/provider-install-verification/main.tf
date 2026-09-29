terraform {
  required_providers {
    rwx = {
      source = "rwx-cloud/rwx"
    }
  }
}

provider "rwx" {}

resource "rwx_secret" "test" {
  vault        = "default"
  name         = "foobar"
  secret_value = "test"
}

resource "rwx_variable" "test" {
  vault = "default"
  name  = "foo"
  value = "bar"
}

resource "rwx_oidc_token" "test" {
  vault    = "default"
  name     = "terraform-provider-verification"
  audience = "terraform-provider-verification"
}
