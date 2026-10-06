package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func siteAccConfig(project, attrs string) string {
	return accProvider() + fmt.Sprintf(`
resource "honeybadger_project" "a" {
  name = %q
}

resource "honeybadger_site" "t" {
  project_id = honeybadger_project.a.id
  url        = "https://www.example.com/"
%s}
`, project, attrs)
}

func TestAccSite(t *testing.T) {
	name := accName()
	match := "  locations = [\"Virginia\", \"London\"]\n  match_type = \"include\"\n  match = \"Example\"\n"
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{Config: siteAccConfig(name, match+"  request_headers = { Authorization = \"Bearer a\" }\n")},
			{
				Config:           siteAccConfig(name, match+"  request_headers = { Authorization = \"Bearer b\", Accept = \"text/html\" }\n"),
				ConfigPlanChecks: expectUpdate(siteAddr),
			},
			{
				Config: siteAccConfig(name, match),
				ConfigPlanChecks: expectUpdate(siteAddr,
					plancheck.ExpectKnownValue(siteAddr, tfjsonpath.New("request_headers"), knownvalue.MapExact(map[string]knownvalue.Check{})),
				),
			},
			{ResourceName: siteAddr, ImportState: true, ImportStateIdFunc: importID(siteAddr), ImportStateVerify: true},
		},
	})
}

// match_type "success" takes no match; the API's 422 reaches the user.
func TestAccSiteMatchRefused(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{
				Config:      siteAccConfig(accName(), "  match_type = \"success\"\n  match = \"x\"\n"),
				ExpectError: regexp.MustCompile(`(?i)match`),
			},
		},
	})
}

// Needs custom uptime timeouts on the local account (Kevin enables them).
func TestAccSiteTimeout(t *testing.T) {
	name := accName()
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{Config: siteAccConfig(name, "  timeout = 10\n")},
			{Config: siteAccConfig(name, ""), ConfigPlanChecks: expectUpdate(siteAddr)},
		},
	})
}
