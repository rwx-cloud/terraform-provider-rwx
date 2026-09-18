# Terraform RWX Provider

The [RWX Provider](https://registry.terraform.io/providers/rwx-cloud/rwx/latest/docs) enables [Terraform](https://terraform.io) to manage [RWX](https://www.rwx.com) resources such as vault secrets and variables.

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

resource "rwx_secret" "example" {
  vault        = "default"
  name         = "my-secret"
  secret_value = "a-secret-token"
  description  = "holds a secret token"
}

resource "rwx_variable" "example" {
  vault = "default"
  name  = "my-variable"
  value = "some-value"
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

Releases are cut by pushing a semver tag (e.g. `v1.2.3`). The
[`.rwx/continuous_deployment.yml`](.rwx/continuous_deployment.yml) pipeline runs
the build, lint, docs, and test tasks, then runs
[GoReleaser](https://goreleaser.com) to build, GPG-sign, and publish the release
artifacts that the Terraform Registry ingests.

## License

[MIT](LICENSE)
