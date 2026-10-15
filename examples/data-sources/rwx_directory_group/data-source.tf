data "rwx_directory" "engineering" {
  name = "Engineering"
}

data "rwx_directory_group" "engineering" {
  directory_id = data.rwx_directory.engineering.id
  name         = "Engineering"
}

data "rwx_directory_group" "by_id" {
  directory_id = "00000000-0000-0000-0000-000000000001"
  id           = "00000000-0000-0000-0000-000000000002"
}
