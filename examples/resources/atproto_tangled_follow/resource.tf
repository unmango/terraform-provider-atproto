data "atproto_account" "bob" {
  handle = "bob.bsky.social"
}

resource "atproto_tangled_follow" "bob" {
  subject = data.atproto_account.bob.did
}
