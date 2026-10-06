package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// accProvider is the provider block for acceptance tests: the local stack's
// URL and Kevin's token come from the environment.
func accProvider() string {
	return fmt.Sprintf("provider \"honeybadger\" {\n  api_url = %q\n}\n", os.Getenv("HONEYBADGER_API_URL"))
}

func accName() string { return accPrefix + acctest.RandString(8) }

// importID builds "<project_id>/<id>" for a project-scoped resource.
func importID(addr string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs, ok := s.RootModule().Resources[addr]
		if !ok {
			return "", fmt.Errorf("%s isn't in state", addr)
		}
		return rs.Primary.Attributes["project_id"] + "/" + rs.Primary.ID, nil
	}
}

func TestAccProject(t *testing.T) {
	name := accName()
	config := func(name, extra string) string {
		return accProvider() + fmt.Sprintf("resource \"honeybadger_project\" \"t\" {\n  name = %q\n%s}\n", name, extra)
	}
	addr := "honeybadger_project.t"
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{Config: config(name, "  language = \"ruby\"\n  disable_public_links = true\n")},
			{
				Config: config(name+"-2", "  language = \"python\"\n  disable_public_links = true\n"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate),
					plancheck.ExpectUnknownValue(addr, tfjsonpath.New("token")),
				}},
			},
			{
				// Removing them resets the default and clears the language.
				Config: config(name+"-2", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate),
					plancheck.ExpectKnownValue(addr, tfjsonpath.New("disable_public_links"), knownvalue.Bool(false)),
					plancheck.ExpectKnownValue(addr, tfjsonpath.New("language"), knownvalue.Null()),
				}},
			},
			{ResourceName: addr, ImportState: true, ImportStateVerify: true},
		},
	})
}

// A new key changes the project's token, so a project update in the same
// apply plans token as unknown and the apply succeeds.
func TestAccProjectKeyAndTokenInOneApply(t *testing.T) {
	name := accName()
	config := func(project, keys string) string {
		return accProvider() + fmt.Sprintf("resource \"honeybadger_project\" \"t\" {\n  name = %q\n}\n", project) + keys
	}
	key := func(n, label string) string {
		return fmt.Sprintf("resource \"honeybadger_project_key\" %q {\n  project_id = honeybadger_project.t.id\n  label = %q\n}\n", n, label)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{Config: config(name, key("one", "first"))},
			{
				Config: config(name+"-2", key("one", "first")+key("two", "second")),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectUnknownValue("honeybadger_project.t", tfjsonpath.New("token")),
				}},
			},
			{
				Config: config(name+"-2", key("one", "renamed")+key("two", "second")),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("honeybadger_project_key.one", plancheck.ResourceActionUpdate),
				}},
			},
			{
				ResourceName:      "honeybadger_project_key.one",
				ImportState:       true,
				ImportStateIdFunc: importID("honeybadger_project_key.one"),
				ImportStateVerify: true,
			},
		},
	})
}
