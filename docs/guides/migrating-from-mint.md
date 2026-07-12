---
page_title: "Migrating from the rwx-research/mint provider"
subcategory: ""
description: |-
  How to move existing Terraform state from the legacy rwx-research/mint provider to rwx-cloud/rwx.
---

# Migrating from `rwx-research/mint`

The `rwx-cloud/rwx` provider is the successor to the legacy
[`rwx-research/mint`](https://registry.terraform.io/providers/rwx-research/mint)
provider. It manages the same RWX vault secrets and variables; only the provider
name and resource type prefixes have changed:

| Legacy (`mint`)  | New (`rwx`)     |
| ---------------- | --------------- |
| `mint_secret`    | `rwx_secret`    |
| `mint_variable`  | `rwx_variable`  |

The resource schemas are **identical**, so you can migrate existing Terraform
state in place without destroying or recreating anything in RWX. This matters
in particular for secrets: the provider does not support importing secrets, and
creating a secret that already exists fails, so a destroy/recreate cycle is
disruptive. Migrating state avoids that entirely.

## 1. Update your configuration

Change the provider requirement, the provider block, and every resource type.

```hcl
terraform {
  required_providers {
    rwx = {
      source = "rwx-cloud/rwx"
    }
  }
}

provider "rwx" {
  access_token = var.rwx_access_token
}

resource "rwx_secret" "example" {
  vault        = "default"
  name         = "my-secret"
  secret_value = "a-secret-token"
}

resource "rwx_variable" "example" {
  vault = "default"
  name  = "my-variable"
  value = "some-value"
}
```

The `RWX_ACCESS_TOKEN` environment variable is unchanged. If you set the host
explicitly, note the environment variable is now `RWX_HOST` (was `MINT_HOST`).

## 2. Back up your state

```sh
terraform state pull > backup.tfstate
```

## 3. Install the new provider

```sh
terraform init
```

## 4. Move existing resources in state

Re-point state at the new provider, then rename each resource's type. Run one
`state mv` per resource you manage (the addresses below are examples):

```sh
terraform state replace-provider \
  registry.terraform.io/rwx-research/mint \
  registry.terraform.io/rwx-cloud/rwx

terraform state mv mint_secret.example   rwx_secret.example
terraform state mv mint_variable.example rwx_variable.example
```

For resources created with `count` or `for_each`, move each instance address
(e.g. `mint_secret.example[0]`, `mint_secret.example["prod"]`).

## 5. Verify

```sh
terraform plan
```

The plan **must show no changes**. If it proposes to create or destroy any
secret or variable, stop and restore `backup.tfstate` — an address was missed or
mistyped. Only once the plan is clean should you apply or commit the change.
