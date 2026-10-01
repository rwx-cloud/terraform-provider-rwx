terraform {
  required_providers {
    rwx = {
      source = "rwx-cloud/rwx"
    }
  }
}

provider "rwx" {}

resource "rwx_vault" "test" {
  name = "terraform-provider-verification"
}

resource "rwx_secret" "test" {
  vault_id     = rwx_vault.test.id
  name         = "foobar"
  secret_value = "test"
}

resource "rwx_variable" "test" {
  vault_id = rwx_vault.test.id
  name     = "foo"
  value    = "bar"
}

resource "rwx_oidc_token" "test" {
  vault_id = rwx_vault.test.id
  name     = "terraform-provider-verification"
  audience = "terraform-provider-verification"
}
