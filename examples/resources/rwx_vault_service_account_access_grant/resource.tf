resource "rwx_vault" "example" {
  name = "example"
}

data "rwx_service_account" "deploy" {
  display_name = "Deploy automation"
}

resource "rwx_vault_service_account_access_grant" "example" {
  vault_id        = rwx_vault.example.id
  service_account = data.rwx_service_account.deploy.name
}
