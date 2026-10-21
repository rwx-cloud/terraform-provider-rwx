resource "rwx_service_account" "automation" {
  display_name = "Deploy automation"
}

resource "rwx_vault" "deployments" {
  name = "deployments"
}

resource "rwx_service_account_permissions" "automation" {
  service_account_id = rwx_service_account.automation.id
  permissions = [
    { key = "run:read" },
    {
      key         = "vault:manage"
      resource_id = rwx_vault.deployments.id
    },
  ]
}

data "rwx_service_account" "reporting" {
  key = "reporting"
}

resource "rwx_service_account_permissions" "reporting" {
  service_account_id = data.rwx_service_account.reporting.id
  permissions = [
    { key = "run:read" },
  ]
}
