resource "rwx_vault" "example" {
  name = "example"
}

resource "rwx_vault_user_access_grant" "example" {
  vault_id   = rwx_vault.example.id
  email      = "user@example.com"
  expires_at = "2099-12-01T15:00:00Z"
}
