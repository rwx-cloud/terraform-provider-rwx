resource "rwx_vault" "example" {
  name               = "example"
  unlocked           = false
  approvals_enabled  = true
  required_approvals = 2

  repository_permissions = [{
    repository_slug = "rwx-cloud/example"
    branch_pattern  = "main"
  }]
}
