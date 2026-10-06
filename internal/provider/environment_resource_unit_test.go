package provider

import (
	"fmt"
	"testing"

	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

const environmentAddr = "honeybadger_environment.t"

func environmentHCL(url, attrs string) string {
	return providerConfig(url) + fmt.Sprintf(`
resource "honeybadger_environment" "t" {
  project_id = "p1"
%s}
`, attrs)
}

func TestEnvironmentWrites(t *testing.T) {
	f := newFakeHoneybadger(t)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{
			{
				Config: environmentHCL(f.srv.URL, "  name = \"staging\"\n"),
				Check:  lastBody(f, "POST", `/environments$`, map[string]any{"name": "staging", "notifications": true}),
			},
			{
				Config: environmentHCL(f.srv.URL, "  name = \"staging\"\n  notifications = false\n"),
				Check:  lastBody(f, "PATCH", `/environments/`, map[string]any{"notifications": false}),
			},
			{
				// The API refuses a rename, so a new name replaces the environment.
				Config: environmentHCL(f.srv.URL, "  name = \"qa\"\n  notifications = false\n"),
				ConfigPlanChecks: tfresource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(environmentAddr, plancheck.ResourceActionReplace),
				}},
			},
			{ResourceName: environmentAddr, ImportState: true, ImportStateIdFunc: importID(environmentAddr), ImportStateVerify: true},
		},
	})
}
