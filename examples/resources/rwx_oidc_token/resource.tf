resource "rwx_vault" "example" {
  name = "example"
}

resource "rwx_oidc_token" "example" {
  vault_id = rwx_vault.example.id
  name     = "aws"
  audience = "sts.amazonaws.com"
}
