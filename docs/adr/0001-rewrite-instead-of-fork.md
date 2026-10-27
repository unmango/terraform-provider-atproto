# Rewrite in Go instead of forking entropitor/atproto

This provider exists because of [the atproto provider by Jens Claes (entropitor)](https://entropitor.com/blog/atproto-terraform-provider), which first showed that AT Protocol records, Tangled state included, can be managed with Terraform.
We wanted that provider's design, but needed it in Go, so we rewrote it rather than forking or depending on it.

The deciding reason is that the Crossplane (Upjet) and Pulumi providers are generated from this one, and both bridges link a Terraform provider in as a Go library.
The original is TypeScript, built on its author's own TypeScript Terraform SDK, so neither bridge can consume it.

Second, we wanted the Tangled resources to do what the Tangled appview does, not only write records.
`atproto_tangled_repository` creates the repository on the knot with `sh.tangled.repo.create` before writing its record, and knot members and repository collaborators are managed through the knot's ACL rather than as records.

Forking and porting was not an option either.
The original is published under CC BY-ND 4.0 and Apache 2.0, with the condition that derivatives keep the SDK's runtime notice, which a port to another language could not honour.
So no code was copied; what carried over is the design: the generic `atproto_record` resource, the `atproto_tangled_*` resource set and its naming, and the `atproto_account` and `atproto_identity` data sources.

## Consequences

This provider is not a drop-in replacement.
It has a different source address (`unmango/atproto` rather than `entropitor/atproto`) and a different schema, and there is no state migration between them.
