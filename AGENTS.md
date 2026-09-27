# terraform-provider-atproto

## What this is

A Terraform/OpenTofu provider for AT Protocol records, built on the Terraform Plugin Framework (protocol 6).
Most resources manage Tangled (`sh.tangled.*`) state; `atproto_record` manages a record in any collection.
Go module: `github.com/unmango/terraform-provider-atproto`.
Provider address: `registry.opentofu.org/unmango/atproto`.
The Crossplane (Upjet) and Pulumi providers are generated from this one, so its schema is their API.

## Commands

- `make build` runs `nix build .#`, which builds via `nix/default.nix` (`buildGoApplication`) and runs Ginkgo in its checkPhase.
- `make test` runs `go tool ginkgo run -r`. The specs drive OpenTofu, so `TF_ACC_TERRAFORM_PATH` must point at a `tofu` binary; the dev shell sets it.
- `make lint` runs `golangci-lint run ./...` and `nix flake check`.
- `make fmt` runs `nix fmt`.
- `make docs` runs `tfplugindocs generate`; generated files in `docs/` must not be hand-edited and are verified by the CI codegen job.
- `make tidy` regenerates `go.sum` and `nix/gomod2nix.toml`; run it after any dependency change.

## Architecture

- `internal/atproto` is the protocol client: record CRUD on the account's PDS through indigo's `atclient`, and `Knot`, which calls a knot's XRPC methods with a service-auth token from `com.atproto.server.getServiceAuth` (`aud` is the knot's `did:web`, `lxm` the method).
- Records are sent as JSON maps rather than indigo lexicon types. `tangled.org/core/api/tangled` is avoided: it pulls in gorm and prometheus, and its `init` registers NSIDs, which panics if registered twice.
- `internal/provider/records.go` holds `recordResource`, the generic CRUD for resources that are fully described by one record. Each such resource is a `recordSpec` plus a model implementing `recordModel` (`tangled_records.go`). Updates merge into the stored record so fields the model doesn't know, such as the profile avatar, survive; `atproto_record` opts out through `wholeRecord`.
- Some Tangled state is not record-only. `atproto_tangled_repository` creates the repo on the knot (`sh.tangled.repo.create`, which mints the repo DID) before writing the record, and deletes it from the knot after deleting the record. Knot members and repository collaborators exist only in the knot's ACL on knots advertising the `knot-acl` capability, so `tangled_acl.go` calls the knot and writes no record. Knots without that capability are refused.
- Tests are Ginkgo specs that run `terraform-plugin-testing` steps through OpenTofu against the fake PDS and knot in `fake_test.go`.

## Releases

- release-please manages versioning and CHANGELOG from conventional commits; never hand-edit `VERSION`, the manifest, or CHANGELOG entries.
- Tag pushes (`v*`) trigger goreleaser, which builds, zips, checksums, and GPG-signs registry artifacts using the `GPG_PRIVATE_KEY` and `PASSPHRASE` repo secrets.
