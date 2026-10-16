resource "rwx_service_account_permissions" "automation" {
  service_account_id = "00000000-0000-0000-0000-000000000001"
  permissions = [
    { key = "run:read" },
    {
      key         = "vault:manage"
      resource_id = "00000000-0000-0000-0000-000000000002"
    },
  ]
}
