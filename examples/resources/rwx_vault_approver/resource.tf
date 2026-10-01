resource "rwx_vault" "example" {
  name = "deployments"
}

resource "rwx_vault_approver" "example" {
  vault_id = rwx_vault.example.id
  email    = "reviewer@example.com"
}
