resource "rwx_vault" "example" {
  name     = "example"
  unlocked = false

  repository_permissions = [{
    repository_slug = "rwx-cloud/example"
    branch_pattern  = "main"
  }]
}
