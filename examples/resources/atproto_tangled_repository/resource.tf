resource "atproto_tangled_repository" "site" {
  name        = "site"
  knot        = "knot.example.com"
  description = "My website"
  topics      = ["web"]
}
