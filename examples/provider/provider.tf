provider "atproto" {
  handle = "alice.bsky.social"
  # Prefer the ATPROTO_APP_PASSWORD environment variable.
  app_password = var.app_password
}
