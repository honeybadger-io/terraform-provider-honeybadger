package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

const checkInAddr = "honeybadger_check_in.t"

// checkInAccConfig declares two projects (a and b) and one check-in in the
// named project with the given attributes.
func checkInAccConfig(projectName, project, attrs string) string {
	return accProvider() + fmt.Sprintf(`
resource "honeybadger_project" "a" {
  name = %q
}

resource "honeybadger_project" "b" {
  name = "%s-b"
}

resource "honeybadger_check_in" "t" {
  project_id = honeybadger_project.%s.id
  name       = "nightly"
%s}
`, projectName, projectName, project, attrs)
}

func expectUpdate(addr string, extra ...plancheck.PlanCheck) resource.ConfigPlanChecks {
	return resource.ConfigPlanChecks{PreApply: append([]plancheck.PlanCheck{
		plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate),
	}, extra...)}
}

func TestAccCheckIn(t *testing.T) {
	name := accName()
	simple := "  report_period = \"1 day\"\n  grace_period = \"1 week\"\n"
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			// Review Focus 1: the API stores "7 days"; the post-apply plan is
			// empty because semantic equality keeps "1 week".
			{Config: checkInAccConfig(name, "a", simple)},
			{
				Config:           checkInAccConfig(name, "a", "  report_period = \"01:30:00\"\n  grace_period = \"1 week\"\n  slug = \"nightly-job\"\n"),
				ConfigPlanChecks: expectUpdate(checkInAddr),
			},
			{
				// Two configured forms of one duration are different text: one
				// update, then an empty plan.
				Config:           checkInAccConfig(name, "a", "  report_period = \"1 hour 30 minutes\"\n  grace_period = \"1 week\"\n  slug = \"nightly-job\"\n"),
				ConfigPlanChecks: expectUpdate(checkInAddr),
			},
			{
				Config: checkInAccConfig(name, "a", "  schedule_type = \"cron\"\n  cron_schedule = \"0 3 * * *\"\n  grace_period = \"1 week\"\n"),
				ConfigPlanChecks: expectUpdate(checkInAddr,
					plancheck.ExpectKnownValue(checkInAddr, tfjsonpath.New("report_period"), knownvalue.Null()),
					plancheck.ExpectKnownValue(checkInAddr, tfjsonpath.New("cron_timezone"), knownvalue.StringExact("UTC")),
				),
			},
			{Config: checkInAccConfig(name, "a", simple), ConfigPlanChecks: expectUpdate(checkInAddr)},
			{
				Config: checkInAccConfig(name, "b", simple),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(checkInAddr, plancheck.ResourceActionReplace),
				}},
			},
			{
				// Prior state holds "1 week"; the import reads "7 days".
				ResourceName:            checkInAddr,
				ImportState:             true,
				ImportStateIdFunc:       importID(checkInAddr),
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"grace_period", "report_period"},
			},
			{
				// A plannable import of a check-in stored as "7 days" with
				// "1 week" configured plans one harmless update (spec 2b, 5.3).
				Config:             checkInAccConfig(name, "b", simple),
				ResourceName:       checkInAddr,
				ImportState:        true,
				ImportStateKind:    resource.ImportBlockWithID,
				ImportStateIdFunc:  importID(checkInAddr),
				ExpectNonEmptyPlan: true,
				ImportPlanChecks: resource.ImportPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(checkInAddr, plancheck.ResourceActionUpdate),
				}},
			},
			{
				// After it's applied, the plan is empty.
				Config: checkInAccConfig(name, "b", simple),
			},
		},
	})
}

func TestAccCheckInConcurrency(t *testing.T) {
	name := accName()
	config := accProvider() + fmt.Sprintf(`
resource "honeybadger_project" "a" {
  name = %q
}

resource "honeybadger_check_in" "many" {
  count         = 5
  project_id    = honeybadger_project.a.id
  name          = "job-${count.index}"
  report_period = "1 hour"
}
`, name)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		Steps:                    []resource.TestStep{{Config: config}},
	})
}

func TestAccCheckInDeletedOutsideTerraform(t *testing.T) {
	name := accName()
	config := checkInAccConfig(name, "a", "  report_period = \"1 day\"\n")
	var projectID, checkInID string
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: func(s *terraform.State) error {
					rs := s.RootModule().Resources[checkInAddr]
					projectID, checkInID = rs.Primary.Attributes["project_id"], rs.Primary.ID
					return nil
				},
			},
			{
				PreConfig: func() {
					if err := accClient().CheckIns.Delete(context.Background(), projectID, checkInID); err != nil {
						t.Fatalf("deleting the check-in outside Terraform: %v", err)
					}
				},
				Config:             config,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}
