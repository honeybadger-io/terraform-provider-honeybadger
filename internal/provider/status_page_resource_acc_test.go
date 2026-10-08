package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func statusPageAccConfig(name, sites, attrs string) string {
	return accProvider() + fmt.Sprintf(`
resource "honeybadger_project" "a" {
  name = %[1]q
}

resource "honeybadger_site" "one" {
  project_id = honeybadger_project.a.id
  url        = "https://www.example.com/one"
}

resource "honeybadger_site" "two" {
  project_id = honeybadger_project.a.id
  url        = "https://www.example.com/two"
}

resource "honeybadger_check_in" "c" {
  project_id    = honeybadger_project.a.id
  name          = "nightly"
  report_period = "1 day"
}

resource "honeybadger_status_page" "t" {
  name      = %[1]q
  sites     = %[2]s
  check_ins = [{ check_in_id = honeybadger_check_in.c.id, display_name = "Nightly" }]
%[3]s}
`, name, sites, attrs)
}

func TestAccStatusPage(t *testing.T) {
	name := accName()
	one := "{ site_id = honeybadger_site.one.id }"
	two := "{ site_id = honeybadger_site.two.id, display_name = \"Two\" }"
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{Config: statusPageAccConfig(name, "["+one+", "+two+"]", "")},
			{Config: statusPageAccConfig(name, "["+two+", "+one+"]", ""), ConfigPlanChecks: expectUpdate(statusPageAddr)},
			{Config: statusPageAccConfig(name, "["+two+"]", ""), ConfigPlanChecks: expectUpdate(statusPageAddr)},
			{ResourceName: statusPageAddr, ImportState: true, ImportStateVerify: true},
		},
	})
}

// Needs advanced status pages on the local account (Kevin enables them).
func TestAccStatusPageAdvanced(t *testing.T) {
	name := accName()
	sites := "[{ site_id = honeybadger_site.one.id }]"
	protected := "  features = { up_caption = \"All good\" }\n  password_protected = true\n  username = \"viewer\"\n  password = \"correct-horse\"\n"
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{Config: statusPageAccConfig(name, sites, protected)},
			{
				Config:           statusPageAccConfig(name, sites, "  password_protected = true\n  username = \"viewer\"\n  password = \"correct-horse\"\n"),
				ConfigPlanChecks: expectUpdate(statusPageAddr),
			},
			{ResourceName: statusPageAddr, ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"password"}},
			// Protection removed: password_protected, username and password
			// all go.
			{Config: statusPageAccConfig(name, sites, ""), ConfigPlanChecks: expectUpdate(statusPageAddr)},
		},
	})
}
