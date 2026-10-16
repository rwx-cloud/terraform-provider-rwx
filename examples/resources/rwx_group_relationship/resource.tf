# Choose either the direct-ID resource or the by-name resource for a given group pair.
resource "rwx_group_relationship" "engineering_deployers" {
  parent_group_id = "00000000-0000-0000-0000-000000000001"
  child_group_id  = "00000000-0000-0000-0000-000000000002"
}

data "rwx_group" "engineering" {
  name = "Engineering"
}

data "rwx_group" "deployers" {
  name = "Deployers"
}

resource "rwx_group_relationship" "by_name" {
  parent_group_id = data.rwx_group.engineering.id
  child_group_id  = data.rwx_group.deployers.id
}
