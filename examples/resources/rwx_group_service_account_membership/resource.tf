resource "rwx_group" "automation" {
  name = "automation"
}

resource "rwx_service_account" "builder" {
  display_name = "Builder"
}

resource "rwx_group_service_account_membership" "builder" {
  group_id           = rwx_group.automation.id
  service_account_id = rwx_service_account.builder.id
}
