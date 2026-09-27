package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/unmango/terraform-provider-atproto/internal/atproto"
)

func TestProvider(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Provider Suite")
}

var (
	pds  *fakePDS
	knot *fakeKnot
)

var _ = BeforeEach(func() {
	pds = newFakePDS()
	knot = newFakeKnot()
	DeferCleanup(pds.Close)
	DeferCleanup(knot.Close)

	realLogin := login
	login = func(ctx context.Context, cfg atproto.Config) (*atproto.Client, error) {
		c, err := realLogin(ctx, cfg)
		if err == nil {
			c.KnotURL = func(domain string) string {
				Expect(domain).To(Equal(testKnot))
				return knot.URL()
			}
		}
		return c, err
	}
	DeferCleanup(func() { login = realLogin })
})

var providerFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"atproto": providerserver.NewProtocol6WithError(New("test")()),
}

// providerConfig logs in to the fake PDS.
func providerConfig() string {
	return `
provider "atproto" {
  handle       = "` + testHandle + `"
  app_password = "app-password"
  pds_host     = "` + pds.URL() + `"
}
`
}

// apply runs the steps through OpenTofu against the fake PDS and knot.
func apply(steps ...resource.TestStep) {
	GinkgoHelper()

	for i := range steps {
		if steps[i].Config != "" {
			steps[i].Config = providerConfig() + steps[i].Config
		}
	}

	resource.UnitTest(GinkgoT(), resource.TestCase{
		ProtoV6ProviderFactories: providerFactories,
		Steps:                    steps,
	})
}
