resource "rwx_oidc_token" "example" {
  vault    = "default"
  name     = "aws"
  audience = "sts.amazonaws.com"
}
