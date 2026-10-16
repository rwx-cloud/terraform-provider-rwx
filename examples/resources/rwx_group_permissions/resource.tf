resource "rwx_group" "deployers" {
  name = "Deployers"
}

resource "rwx_vault" "deployments" {
  name = "deployments"
}

resource "rwx_group_permissions" "deployers" {
  group_id = rwx_group.deployers.id
  permissions = [
    { key = "run:read" },
    {
      key         = "vault:unlock"
      resource_id = rwx_vault.deployments.id
    },
  ]
}

data "rwx_group" "auditors" {
  name = "Auditors"
}

resource "rwx_group_permissions" "auditors" {
  group_id = data.rwx_group.auditors.id
  permissions = [
    { key = "run:read" },
  ]
}
