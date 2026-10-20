resource "rwx_group" "engineering" {
  name = "engineering"
}

data "rwx_user" "engineer" {
  email = "engineer@example.com"
}

resource "rwx_group_user_membership" "engineer" {
  group_id = rwx_group.engineering.id
  user_id  = data.rwx_user.engineer.id
}
