package provider

import (
	"regexp"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("atproto_tangled_repository", func() {
	config := func(description string) string {
		return `
resource "atproto_tangled_repository" "site" {
  name           = "Site.git"
  knot           = "https://` + testKnot + `/"
  default_branch = "trunk"
  description    = "` + description + `"
  topics         = ["web"]
}
`
	}

	It("creates the repo on the knot before writing its record", func() {
		apply(
			resource.TestStep{
				Config: config("my site"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("atproto_tangled_repository.site", "rkey", "site"),
					resource.TestCheckResourceAttr("atproto_tangled_repository.site", "repo_did", "did:plc:repo-site"),
					check(func() {
						Expect(knot.Calls()).To(Equal([]string{"sh.tangled.repo.create"}))
						Expect(knot.repos).To(HaveKeyWithValue("site", "did:plc:repo-site"))
						Expect(knot.defaultBranch).To(HaveKeyWithValue("did:plc:repo-site", "trunk"))

						rec, _ := pds.record("sh.tangled.repo", "site")
						Expect(rec.Value).To(HaveKeyWithValue("repoDid", "did:plc:repo-site"))
						Expect(rec.Value).To(HaveKeyWithValue("name", "Site.git"))
						Expect(rec.Value).To(HaveKeyWithValue("description", "my site"))
					}),
				),
			},
			resource.TestStep{
				Config: config("my new site"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("atproto_tangled_repository.site", plancheck.ResourceActionUpdate),
					},
				},
				Check: check(func() {
					Expect(knot.Calls()).To(Equal([]string{"sh.tangled.repo.create"}))
					rec, _ := pds.record("sh.tangled.repo", "site")
					Expect(rec.Value).To(HaveKeyWithValue("description", "my new site"))
				}),
			},
			resource.TestStep{
				Config:                  config("my new site"),
				ResourceName:            "atproto_tangled_repository.site",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"default_branch"},
			},
		)

		Expect(knot.Calls()).To(Equal([]string{"sh.tangled.repo.create", "sh.tangled.repo.delete"}))
		Expect(knot.repos).To(BeEmpty())
		Expect(pds.keys("sh.tangled.repo")).To(BeEmpty())
	})

	It("changes the default branch through the knot", func() {
		apply(
			resource.TestStep{Config: config("x")},
			resource.TestStep{
				Config: `
resource "atproto_tangled_repository" "site" {
  name           = "Site.git"
  knot           = "https://` + testKnot + `/"
  default_branch = "main"
  description    = "x"
  topics         = ["web"]
}
`,
				Check: check(func() {
					Expect(knot.defaultBranch).To(HaveKeyWithValue("did:plc:repo-site", "main"))
				}),
			},
		)
	})
})

var _ = Describe("atproto_tangled_knot_member", func() {
	const member = `
resource "atproto_tangled_knot_member" "bob" {
  knot    = "` + testKnot + `"
  subject = "did:plc:bob"
}
`

	It("adds, detects removal, and imports", func() {
		apply(
			resource.TestStep{
				Config: member,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("atproto_tangled_knot_member.bob", "id", testKnot+"/did:plc:bob"),
					check(func() { Expect(knot.members).To(ConsistOf("did:plc:bob")) }),
				),
			},
			resource.TestStep{
				PreConfig: func() { knot.members = nil },
				Config:    member,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("atproto_tangled_knot_member.bob", plancheck.ResourceActionCreate),
					},
				},
			},
			resource.TestStep{
				Config:            member,
				ResourceName:      "atproto_tangled_knot_member.bob",
				ImportState:       true,
				ImportStateId:     testKnot + "/did:plc:bob",
				ImportStateVerify: true,
			},
		)

		Expect(knot.members).To(BeEmpty())
	})

	It("refuses knots without the knot-acl capability", func() {
		knot.capabilities = nil

		apply(resource.TestStep{
			Config:      member,
			ExpectError: regexp.MustCompile(`lacks the "knot-acl" capability`),
		})
	})
})

var _ = Describe("atproto_tangled_repository_collaborator", func() {
	It("adds and removes the collaborator", func() {
		apply(resource.TestStep{
			Config: `
resource "atproto_tangled_repository" "site" {
  name = "site"
  knot = "` + testKnot + `"
}

resource "atproto_tangled_repository_collaborator" "bob" {
  knot     = atproto_tangled_repository.site.knot
  repo_did = atproto_tangled_repository.site.repo_did
  subject  = "did:plc:bob"
}
`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("atproto_tangled_repository_collaborator.bob", "id", testKnot+"/did:plc:repo-site/did:plc:bob"),
				check(func() {
					Expect(knot.collaborators).To(HaveKeyWithValue("did:plc:repo-site", ConsistOf("did:plc:bob")))
				}),
			),
		})

		Expect(knot.collaborators["did:plc:repo-site"]).To(BeEmpty())
	})
})

var _ = Describe("data sources", func() {
	It("resolves the provider's account and other handles", func() {
		apply(resource.TestStep{
			Config: `
data "atproto_identity" "me" {}

data "atproto_account" "bob" {
  handle = "bob.test"
}
`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("data.atproto_identity.me", "did", testDID),
				resource.TestCheckResourceAttr("data.atproto_account.bob", "did", "did:plc:bob"),
			),
		})
	})
})

var _ = Describe("provider", func() {
	It("reports a failed login", func() {
		apply(resource.TestStep{
			Config: `
provider "atproto" {
  alias        = "wrong"
  handle       = "` + testHandle + `"
  app_password = "wrong"
  pds_host     = "` + pds.URL() + `"
}

data "atproto_identity" "me" {
  provider = atproto.wrong
}
`,
			ExpectError: regexp.MustCompile(`Unable to Log In`),
		})
	})
})
