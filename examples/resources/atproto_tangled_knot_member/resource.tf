resource "atproto_tangled_knot_member" "bob" {
  knot    = atproto_tangled_knot.home.domain
  subject = data.atproto_account.bob.did
}
