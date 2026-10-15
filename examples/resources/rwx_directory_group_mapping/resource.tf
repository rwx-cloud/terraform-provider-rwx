resource "rwx_directory_group_mapping" "engineering" {
  directory_id       = "00000000-0000-0000-0000-000000000001"
  directory_group_id = "00000000-0000-0000-0000-000000000002"
  group_id           = "00000000-0000-0000-0000-000000000003"
}

data "rwx_directory" "engineering" {
  name = "Engineering"
}

data "rwx_directory_group" "engineering" {
  directory_id = data.rwx_directory.engineering.id
  name         = "Engineering"
}

data "rwx_group" "engineering" {
  name = "Engineering"
}

resource "rwx_directory_group_mapping" "by_name" {
  directory_id           = data.rwx_directory.engineering.id
  directory_group_id     = data.rwx_directory_group.engineering.id
  group_id               = data.rwx_group.engineering.id
  reconciliation_timeout = "20m"
}
