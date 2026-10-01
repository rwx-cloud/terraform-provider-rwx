resource "rwx_vault" "example" {
  name = "example"
}

resource "rwx_secret" "example" {
  vault_id     = rwx_vault.example.id
  name         = "my-secret"
  secret_value = "a-secret-token"
  description  = "holds a secret token"
}
