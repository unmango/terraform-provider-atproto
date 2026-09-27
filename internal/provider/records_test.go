package provider

import (
	"regexp"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// check runs Gomega assertions as a test step check.
func check(f func()) resource.TestCheckFunc {
	return func(*terraform.State) error {
		f()
		return nil
	}
}

func onlyKey(collection string) string {
	GinkgoHelper()

	keys := pds.keys(collection)
	Expect(keys).To(HaveLen(1))

	return keys[0]
}

var _ = Describe("atproto_tangled_follow", func() {
	const follow = `
resource "atproto_tangled_follow" "bob" {
  subject = "did:plc:bob"
}
`

	It("creates, detects deletion, and imports", func() {
		var rkey string

		apply(
			resource.TestStep{
				Config: follow,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("atproto_tangled_follow.bob", "subject", "did:plc:bob"),
					resource.TestCheckResourceAttrSet("atproto_tangled_follow.bob", "created_at"),
					check(func() {
						rkey = onlyKey("sh.tangled.graph.follow")
						rec, _ := pds.record("sh.tangled.graph.follow", rkey)
						Expect(rec.Value).To(HaveKeyWithValue("$type", "sh.tangled.graph.follow"))
						Expect(rec.Value).To(HaveKeyWithValue("subject", "did:plc:bob"))
						Expect(rec.Value).To(HaveKey("createdAt"))
					}),
					resource.TestCheckResourceAttrWith("atproto_tangled_follow.bob", "uri", func(v string) error {
						Expect(v).To(Equal(uri("sh.tangled.graph.follow", rkey)))
						return nil
					}),
				),
			},
			resource.TestStep{
				PreConfig: func() { pds.remove("sh.tangled.graph.follow", rkey) },
				Config:    follow,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("atproto_tangled_follow.bob", plancheck.ResourceActionCreate),
					},
				},
			},
			resource.TestStep{
				Config:            follow,
				ResourceName:      "atproto_tangled_follow.bob",
				ImportState:       true,
				ImportStateVerify: true,
			},
		)
	})

	It("imports by AT URI", func() {
		pds.put("sh.tangled.graph.follow", "3kfollow", map[string]any{"subject": "did:plc:bob", "createdAt": "2026-01-01T00:00:00.000Z"})

		apply(resource.TestStep{
			Config:             follow,
			ResourceName:       "atproto_tangled_follow.bob",
			ImportState:        true,
			ImportStateId:      uri("sh.tangled.graph.follow", "3kfollow"),
			ImportStatePersist: true,
			ImportStateCheck: func(s []*terraform.InstanceState) error {
				Expect(s).To(HaveLen(1))
				Expect(s[0].Attributes).To(HaveKeyWithValue("rkey", "3kfollow"))
				Expect(s[0].Attributes).To(HaveKeyWithValue("created_at", "2026-01-01T00:00:00.000Z"))
				return nil
			},
		})
	})

	It("rejects an AT URI in another collection", func() {
		apply(resource.TestStep{
			Config:        follow,
			ResourceName:  "atproto_tangled_follow.bob",
			ImportState:   true,
			ImportStateId: uri("sh.tangled.publicKey", "3kfollow"),
			ExpectError:   regexp.MustCompile(`not sh\.tangled\.graph\.follow`),
		})
	})
})

var _ = Describe("atproto_tangled_knot", func() {
	It("keys the record by domain", func() {
		apply(resource.TestStep{
			Config: `
resource "atproto_tangled_knot" "k" {
  domain = "` + testKnot + `"
}
`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("atproto_tangled_knot.k", "rkey", testKnot),
				resource.TestCheckResourceAttr("atproto_tangled_knot.k", "id", uri("sh.tangled.knot", testKnot)),
				check(func() {
					Expect(pds.keys("sh.tangled.knot")).To(ConsistOf(testKnot))
				}),
			),
		})
	})
})

var _ = Describe("atproto_tangled_profile", func() {
	It("adopts the existing profile and keeps fields it doesn't manage", func() {
		pds.put("sh.tangled.actor.profile", "self", map[string]any{
			"bluesky": false,
			"avatar":  map[string]any{"$type": "blob", "ref": map[string]any{"$link": "bafyavatar"}},
		})

		apply(
			resource.TestStep{
				Config: `
resource "atproto_tangled_profile" "me" {
  bluesky     = true
  description = "hello"
  stats       = ["star-count"]
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("atproto_tangled_profile.me", "rkey", "self"),
					check(func() {
						rec, _ := pds.record("sh.tangled.actor.profile", "self")
						Expect(rec.Value).To(HaveKeyWithValue("bluesky", true))
						Expect(rec.Value).To(HaveKeyWithValue("description", "hello"))
					}),
				),
			},
			resource.TestStep{
				Config: `
resource "atproto_tangled_profile" "me" {
  bluesky  = true
  location = "Earth"
}
`,
				Check: check(func() {
					rec, _ := pds.record("sh.tangled.actor.profile", "self")
					Expect(rec.Value).To(HaveKeyWithValue("location", "Earth"))
					Expect(rec.Value).NotTo(HaveKey("description"))
					Expect(rec.Value).NotTo(HaveKey("stats"))
					Expect(rec.Value).To(HaveKey("avatar"))
				}),
			},
		)
	})

	It("rejects unknown stats", func() {
		apply(resource.TestStep{
			Config: `
resource "atproto_tangled_profile" "me" {
  bluesky = true
  stats   = ["follower-count"]
}
`,
			ExpectError: regexp.MustCompile(`value must be one of`),
		})
	})
})

var _ = Describe("atproto_tangled_public_key", func() {
	It("updates the key in place", func() {
		config := func(key string) string {
			return `
resource "atproto_tangled_public_key" "laptop" {
  name = "laptop"
  key  = "` + key + `"
}
`
		}

		var rkey string
		apply(
			resource.TestStep{
				Config: config("ssh-ed25519 AAAA1"),
				Check:  check(func() { rkey = onlyKey("sh.tangled.publicKey") }),
			},
			resource.TestStep{
				Config: config("ssh-ed25519 AAAA2"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("atproto_tangled_public_key.laptop", plancheck.ResourceActionUpdate),
					},
				},
				Check: check(func() {
					Expect(onlyKey("sh.tangled.publicKey")).To(Equal(rkey))
					rec, _ := pds.record("sh.tangled.publicKey", rkey)
					Expect(rec.Value).To(HaveKeyWithValue("key", "ssh-ed25519 AAAA2"))
				}),
			},
		)
	})
})

var _ = Describe("atproto_record", func() {
	It("replaces the whole record and detects remote changes", func() {
		apply(
			resource.TestStep{
				Config: `
resource "atproto_record" "status" {
  collection = "xyz.statusphere.status"
  rkey       = "self"
  record     = jsonencode({ status = "🙂", createdAt = "2026-01-01T00:00:00.000Z" })
}
`,
				Check: check(func() {
					rec, _ := pds.record("xyz.statusphere.status", "self")
					Expect(rec.Value).To(HaveKeyWithValue("$type", "xyz.statusphere.status"))
					Expect(rec.Value).To(HaveKeyWithValue("status", "🙂"))
				}),
			},
			resource.TestStep{
				PreConfig: func() {
					pds.put("xyz.statusphere.status", "self", map[string]any{"status": "😐", "createdAt": "2026-01-01T00:00:00.000Z"})
				},
				Config: `
resource "atproto_record" "status" {
  collection = "xyz.statusphere.status"
  rkey       = "self"
  record     = jsonencode({ status = "🙂", createdAt = "2026-01-01T00:00:00.000Z" })
}
`,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("atproto_record.status", plancheck.ResourceActionUpdate),
					},
				},
			},
			resource.TestStep{
				Config: `
resource "atproto_record" "status" {
  collection = "xyz.statusphere.status"
  rkey       = "self"
  record     = jsonencode({ status = "🙃" })
}
`,
				Check: check(func() {
					rec, _ := pds.record("xyz.statusphere.status", "self")
					Expect(rec.Value).To(HaveKeyWithValue("status", "🙃"))
					Expect(rec.Value).NotTo(HaveKey("createdAt"))
				}),
			},
			resource.TestStep{
				Config:            `resource "atproto_record" "status" {}`,
				ResourceName:      "atproto_record.status",
				ImportState:       true,
				ImportStateId:     "xyz.statusphere.status/self",
				ImportStateVerify: true,
			},
		)
	})
})
