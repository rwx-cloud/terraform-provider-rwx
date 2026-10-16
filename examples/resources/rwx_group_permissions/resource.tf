resource "rwx_group_permissions" "deployers" {
  group_id = "00000000-0000-4000-8000-000000000001"
  permissions = [
    { key = "run:read" },
    {
      key         = "vault:unlock"
      resource_id = "00000000-0000-4000-8000-000000000002"
    },
  ]
}
