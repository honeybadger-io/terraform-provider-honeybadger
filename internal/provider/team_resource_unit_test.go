package provider

import (
	"fmt"
	"testing"

	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

const teamAddr = "honeybadger_team.t"

func teamHCL(url, attrs string) string {
	return providerConfig(url) + fmt.Sprintf(`
resource "honeybadger_team" "t" {
  name = "ops"
%s}
`, attrs)
}

// Review Focus 3: project_ids is a set: reordering plans nothing, and
// removing it sends [] (the declared default).
func TestTeamWrites(t *testing.T) {
	f := newFakeHoneybadger(t)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{
			{
				Config: teamHCL(f.srv.URL, "  project_ids = [\"p1\"]\n"),
				Check:  lastBody(f, "POST", `/teams$`, map[string]any{"name": "ops", "project_ids": []any{"p1"}}),
			},
			{Config: teamHCL(f.srv.URL, "  project_ids = [\"p2\", \"p3\"]\n")},
			{Config: teamHCL(f.srv.URL, "  project_ids = [\"p3\", \"p2\"]\n"), ConfigPlanChecks: emptyPlan},
			{
				Config: teamHCL(f.srv.URL, ""),
				Check:  lastBody(f, "PATCH", `/teams/`, map[string]any{"project_ids": []any{}}),
			},
			{ResourceName: teamAddr, ImportState: true, ImportStateVerify: true},
		},
	})
}
