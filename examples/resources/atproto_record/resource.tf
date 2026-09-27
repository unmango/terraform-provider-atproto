resource "atproto_record" "status" {
  collection = "xyz.statusphere.status"
  record = jsonencode({
    status    = "🦋"
    createdAt = "2026-01-01T00:00:00.000Z"
  })
}
