package provider

import (
	"context"
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
			{Config: config(name, `  language = "ruby"
  disable_public_links = true
  resolve_errors_on_deploy = false
  customer_throttle = 100
  source_url = "https://example.com/a/[file]#L[line]"
  user_url = "https://example.com/a/users/[user_id]"
  user_search_field = "context.account_id"
`)},
			{
				Config: config(name+"-2", `  language = "python"
  disable_public_links = true
  resolve_errors_on_deploy = false
  customer_throttle = 200
  source_url = "https://example.com/b/[file]#L[line]"
  user_url = "https://example.com/b/users/[user_id]"
  user_search_field = "context.uid"
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate),
				}},
			},
			{
				// Removing them resets the defaults and clears the rest.
				Config: config(name+"-2", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate),
					plancheck.ExpectKnownValue(addr, tfjsonpath.New("disable_public_links"), knownvalue.Bool(false)),
					plancheck.ExpectKnownValue(addr, tfjsonpath.New("resolve_errors_on_deploy"), knownvalue.Bool(true)),
					plancheck.ExpectKnownValue(addr, tfjsonpath.New("language"), knownvalue.Null()),
					plancheck.ExpectKnownValue(addr, tfjsonpath.New("customer_throttle"), knownvalue.Null()),
					plancheck.ExpectKnownValue(addr, tfjsonpath.New("source_url"), knownvalue.Null()),
					plancheck.ExpectKnownValue(addr, tfjsonpath.New("user_url"), knownvalue.Null()),
					plancheck.ExpectKnownValue(addr, tfjsonpath.New("user_search_field"), knownvalue.Null()),
				}},
			},
			{ResourceName: addr, ImportState: true, ImportStateVerify: true},
		},
	})
}

// A project and its Ingestion Keys change together in one apply. Configs get a
// key's value from honeybadger_ingestion_key: project reads don't include one.
func TestAccProjectAndKeysInOneApply(t *testing.T) {
	name := accName()
	config := func(project, keys string) string {
		return accProvider() + fmt.Sprintf("resource \"honeybadger_project\" \"t\" {\n  name = %q\n}\n", project) + keys
	}
	keyIn := func(n, project, label string) string {
		attrs := "  project_id = honeybadger_project." + project + ".id\n"
		if label != "" {
			attrs += fmt.Sprintf("  label = %q\n", label)
		}
		return fmt.Sprintf("resource \"honeybadger_ingestion_key\" %q {\n%s}\n", n, attrs)
	}
	key := func(n, label string) string { return keyIn(n, "t", label) }
	other := fmt.Sprintf("resource \"honeybadger_project\" \"u\" {\n  name = %q\n}\n", name+"-u")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{Config: config(name, key("one", "first"))},
			{
				Config: config(name+"-2", key("one", "first")+key("two", "second")),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("honeybadger_project.t", plancheck.ResourceActionUpdate),
					plancheck.ExpectResourceAction("honeybadger_ingestion_key.two", plancheck.ResourceActionCreate),
				}},
			},
			{
				Config: config(name+"-2", key("one", "renamed")+key("two", "second")),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("honeybadger_ingestion_key.one", plancheck.ResourceActionUpdate),
				}},
			},
			{
				// Removing the label clears it.
				Config: config(name+"-2", key("one", "")+key("two", "second")),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("honeybadger_ingestion_key.one", plancheck.ResourceActionUpdate),
					plancheck.ExpectKnownValue("honeybadger_ingestion_key.one", tfjsonpath.New("label"), knownvalue.Null()),
				}},
			},
			{
				// A key can't move between projects: it's replaced.
				Config: config(name+"-2", key("one", "")+keyIn("two", "u", "second")+other),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("honeybadger_ingestion_key.two", plancheck.ResourceActionReplace),
				}},
			},
			{
				ResourceName:      "honeybadger_ingestion_key.one",
				ImportState:       true,
				ImportStateIdFunc: importID("honeybadger_ingestion_key.one"),
				ImportStateVerify: true,
			},
		},
	})
}

// A project deleted outside Terraform takes its children with it: refreshing
// a child under the missing project removes it from state (a 404), so the
// plan recreates both instead of failing.
func TestAccProjectDeletedWithChildren(t *testing.T) {
	name := accName()
	config := accProvider() + fmt.Sprintf(`
resource "honeybadger_project" "t" {
  name = %q
}

resource "honeybadger_check_in" "t" {
  project_id    = honeybadger_project.t.id
  name          = "nightly"
  report_period = "1 day"
}
`, name)
	var projectID string
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: func(s *terraform.State) error {
					projectID = s.RootModule().Resources["honeybadger_project.t"].Primary.ID
					return nil
				},
			},
			{
				PreConfig: func() {
					if err := accClient().Projects.Delete(context.Background(), projectID); err != nil {
						t.Fatalf("deleting the project outside Terraform: %v", err)
					}
				},
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("honeybadger_project.t", plancheck.ResourceActionCreate),
					plancheck.ExpectResourceAction(checkInAddr, plancheck.ResourceActionCreate),
				}},
			},
		},
	})
}
