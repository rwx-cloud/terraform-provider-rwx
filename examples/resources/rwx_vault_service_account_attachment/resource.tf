resource "rwx_vault" "example" {
  name = "deployments"
}

resource "rwx_service_account" "deploy" {
  display_name = "Deploy automation"
}

resource "rwx_vault_service_account_attachment" "example" {
  vault_id        = rwx_vault.example.id
  service_account = rwx_service_account.deploy.name
}
