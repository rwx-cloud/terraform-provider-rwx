# Terraform RWX Provider

The [RWX Provider](https://registry.terraform.io/providers/rwx-cloud/rwx/latest/docs) enables [Terraform](https://terraform.io) to manage [RWX](https://www.rwx.com) vaults, secrets, variables, and OIDC tokens.

> **Migrating from `rwx-research/mint`?** This provider is the successor to the
> [`terraform-provider-mint`](https://registry.terraform.io/providers/rwx-research/mint) provider.
> See the [migration guide](docs/guides/migrating-from-mint.md) for how to move existing state over.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://go.dev/doc/install) >= 1.25.8 (to build the provider from source)

## Using the provider

```hcl
terraform {
  required_providers {
    rwx = {
      source = "rwx-cloud/rwx"
    }
  }
}

provider "rwx" {
  # An RWX access token. May also be set via the RWX_ACCESS_TOKEN environment variable.
  access_token = var.rwx_access_token
}

resource "rwx_vault" "example" {
  name = "example"
}

resource "rwx_secret" "example" {
  vault_id     = rwx_vault.example.id
  name         = "my-secret"
  secret_value = "a-secret-token"
  description  = "holds a secret token"
}

resource "rwx_variable" "example" {
  vault_id = rwx_vault.example.id
  name     = "my-variable"
  value    = "some-value"
}

resource "rwx_oidc_token" "example" {
  vault_id = rwx_vault.example.id
  name     = "aws"
  audience = "sts.amazonaws.com"
}
```

### Authentication

The provider authenticates with the RWX API using an access token, resolved in this order:

1. The `access_token` attribute on the `provider` block.
2. The `RWX_ACCESS_TOKEN` environment variable.

The API host defaults to `cloud.rwx.com` and can be overridden with the `host`
attribute or the `RWX_HOST` environment variable (normally only needed when
developing the provider itself).

See the [full documentation on the Terraform Registry](https://registry.terraform.io/providers/rwx-cloud/rwx/latest/docs)
for all resources and their arguments.

## Developing the provider

Requirements are pinned in [`.tool-versions`](.tool-versions) (usable with
[`asdf`](https://asdf-vm.com) or [`mise`](https://mise.jdx.dev)).

Build and install the provider into your `$GOPATH/bin`:

```sh
go install
```

To use a locally built provider, add a [dev override](https://developer.hashicorp.com/terraform/cli/config/config-file#development-overrides-for-provider-developers)
to your `~/.terraformrc`:

```hcl
provider_installation {
  dev_overrides {
    "rwx-cloud/rwx" = "<path-to-your-GOPATH>/bin"
  }
  direct {}
}
```

### Generating documentation

Documentation in `docs/` is generated from the provider schema and the files in
`examples/` using [`tfplugindocs`](https://github.com/hashicorp/terraform-plugin-docs).
Regenerate it after changing schemas or examples:

```sh
cd tools
go generate ./...
```

CI fails if the committed docs are out of date, so run this before opening a PR.

### Testing

Unit tests:

```sh
go test ./...
```

Acceptance tests create and destroy **real** resources in RWX and require an
access token:

```sh
TF_ACC=1 RWX_ACCESS_TOKEN=<your-token> go test ./... -v
```

## Releasing

We release with the
`terraform-provider-rwx-tag-release` dispatch. Provide the semver tag to create (for
example, `v1.2.3`). Leave the ref empty to tag the latest commit on `main`, or
select a specific commit SHA already on `main`. Use the RWX Dispatch UI or the CLI:

```
# Current main
rwx dispatch terraform-provider-rwx-tag-release --param tag=v1.2.0

# Specific SHA already on main
rwx dispatch terraform-provider-rwx-tag-release \
  --ref <sha> \
  --param tag=v1.2.0
```

The dispatch creates and pushes the tag. The [tag pipeline](.rwx/tag.yml) then
runs the same validation used for pull requests and `main`, including the build,
lint, generated docs check, acceptance tests, and provider installation check.
[GoReleaser](https://goreleaser.com) builds, GPG-signs, and publishes the release
artifacts that the Terraform Registry ingests after validation succeeds.

## License

[MIT](LICENSE)
