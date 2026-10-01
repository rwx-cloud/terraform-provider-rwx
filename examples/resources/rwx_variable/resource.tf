resource "rwx_vault" "example" {
  name = "example"
}

resource "rwx_variable" "example" {
  vault_id = rwx_vault.example.id
  name     = "my-variable"
  value    = "foobar"
}
