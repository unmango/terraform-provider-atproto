resource "atproto_tangled_public_key" "laptop" {
  name = "laptop"
  key  = file("~/.ssh/id_ed25519.pub")
}
