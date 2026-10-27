# terraform-provider-atproto

A Terraform/OpenTofu provider for [AT Protocol](https://atproto.com) records, with first-class support for [Tangled](https://tangled.org).

> **Credit:** this provider is a Go rewrite of [the atproto provider by Jens Claes (entropitor)](https://entropitor.com/blog/atproto-terraform-provider), whose design it follows.
> All credit for the idea and the shape of the resources goes there; see [Prior art](#prior-art).

## Resources

| Resource | Manages |
| --- | --- |
| `atproto_record` | A record in any collection, as JSON |
| `atproto_tangled_follow` | `sh.tangled.graph.follow` |
| `atproto_tangled_knot` | `sh.tangled.knot` (knot registration) |
| `atproto_tangled_knot_member` | Knot membership, through the knot |
| `atproto_tangled_profile` | `sh.tangled.actor.profile` |
| `atproto_tangled_public_key` | `sh.tangled.publicKey` (SSH keys) |
| `atproto_tangled_repository` | A repository on a knot and its `sh.tangled.repo` record |
| `atproto_tangled_repository_collaborator` | Repository collaborators, through the knot |

Data sources: `atproto_identity` (the provider's account) and `atproto_account` (handle to DID).

## Usage

```terraform
terraform {
  required_providers {
    atproto = {
      source = "unmango/atproto"
    }
  }
}

provider "atproto" {
  handle = "alice.bsky.social"
  # app_password, or ATPROTO_APP_PASSWORD
}

resource "atproto_tangled_public_key" "laptop" {
  name = "laptop"
  key  = file("~/.ssh/id_ed25519.pub")
}
```

See [`docs/`](docs/) for every resource.

## Prior art

[Jens Claes](https://entropitor.com) (entropitor) built a Terraform provider for AT Protocol, [`entropitor/atproto`](https://registry.terraform.io/providers/entropitor/atproto/latest/docs), and wrote it up in [A Terraform Provider for the ATmosphere](https://entropitor.com/blog/atproto-terraform-provider).
It is written in TypeScript on the author's own [TypeScript Terraform Provider SDK](https://github.com/entropitor/terraform-providers).
This provider borrows its design: the generic `atproto_record` resource, the `atproto_tangled_*` resources and their names, and the `atproto_account` and `atproto_identity` data sources.
No code was copied.

We rewrote it in Go because the Crossplane and Pulumi providers are generated from this one, and those bridges need a Go provider.
The rewrite also creates repositories on the knot and manages knot members and collaborators through the knot's ACL, as the Tangled appview does.
[ADR 0001](docs/adr/0001-rewrite-instead-of-fork.md) records the decision in full.

The two providers are not interchangeable: the source address and schema differ, and state does not migrate from one to the other.

## Development

This repo is nix first.
`direnv allow` loads the dev shell automatically, or use `nix develop`.

```sh
make build  # nix build .#
make test   # go tool ginkgo run -r
make lint   # golangci-lint + nix flake check
make fmt    # nix fmt (treefmt)
make docs   # tfplugindocs generate
make tidy   # go mod tidy + gomod2nix
```
