data "rwx_service_account" "deploy" {
  key = "deploy-automation"
}

data "rwx_service_account" "by_id" {
  id = "00000000-0000-0000-0000-000000000000"
}
