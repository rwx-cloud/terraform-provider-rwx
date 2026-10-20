data "rwx_user" "by_email" {
  email = "engineer@example.com"
}

data "rwx_user" "by_id" {
  id = "00000000-0000-4000-8000-000000000001"
}
