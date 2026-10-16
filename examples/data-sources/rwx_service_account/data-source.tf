data "rwx_service_account" "deploy" {
  name = "deploy-automation"
}

data "rwx_service_account" "by_id" {
  id = "00000000-0000-0000-0000-000000000000"
}

data "rwx_service_account" "by_display_name" {
  display_name = "Deploy automation"
}
