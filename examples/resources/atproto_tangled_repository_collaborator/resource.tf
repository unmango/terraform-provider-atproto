resource "atproto_tangled_repository_collaborator" "bob" {
  knot     = atproto_tangled_repository.site.knot
  repo_did = atproto_tangled_repository.site.repo_did
  subject  = data.atproto_account.bob.did
}
