resource "atproto_tangled_profile" "me" {
  bluesky     = true
  description = "Mostly Go and Nix."
  links       = ["https://example.com"]
  stats       = ["repository-count", "star-count"]
}
