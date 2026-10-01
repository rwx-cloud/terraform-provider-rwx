resource "rwx_vault" "example" {
  name = "example"
}

resource "rwx_vault_service_account_access_grant" "example" {
  vault_id        = rwx_vault.example.id
  service_account = "deploys"
}
