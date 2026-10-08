package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func teamAccConfig(name, projectIDs string) string {
	return accProvider() + fmt.Sprintf(`
resource "honeybadger_project" "a" {
  name = "%[1]s-a"
}

resource "honeybadger_project" "b" {
  name = "%[1]s-b"
}

resource "honeybadger_team" "t" {
  name        = %[1]q
  project_ids = %[2]s
}
`, name, projectIDs)
}

func TestAccTeam(t *testing.T) {
	name := accName()
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{Config: teamAccConfig(name, "[honeybadger_project.a.id]")},
			{
				Config:           teamAccConfig(name, "[honeybadger_project.a.id, honeybadger_project.b.id]"),
				ConfigPlanChecks: expectUpdate(teamAddr),
			},
			{ResourceName: teamAddr, ImportState: true, ImportStateVerify: true},
			{Config: teamAccConfig(name, "[]"), ConfigPlanChecks: expectUpdate(teamAddr)},
			{ResourceName: teamAddr, ImportState: true, ImportStateVerify: true},
		},
	})
}
