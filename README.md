# terraform-provider-atproto

A Terraform/OpenTofu provider for [AT Protocol](https://atproto.com) records, with first-class support for [Tangled](https://tangled.org).

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
