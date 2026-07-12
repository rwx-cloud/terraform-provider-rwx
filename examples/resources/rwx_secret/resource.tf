resource "rwx_secret" "example" {
  vault        = "default"
  name         = "my-secret"
  secret_value = "a-secret-token"
  description  = "holds a secret token"
}
