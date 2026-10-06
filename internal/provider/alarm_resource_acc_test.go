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

const alarmAddr = "honeybadger_alarm.t"

const notices = `filter event_type::str == \"notice\" | stats count() as count`

// alarmBlock is one alarm in the named project with the given attributes.
func alarmBlock(label, project, attrs string) string {
	return fmt.Sprintf(`
resource "honeybadger_alarm" %q {
  project_id = honeybadger_project.%s.id
%s}
`, label, project, attrs)
}

func alarmAttrs(name, query, period, lag, operator, value, extra string) string {
	return fmt.Sprintf(`  name              = %q
  query             = "%s"
  evaluation_period = %q
  lookback_lag      = %q
  trigger_config = {
    type   = "alert_result_count"
    config = { operator = %q, value = %s }
  }
%s`, name, query, period, lag, operator, value, extra)
}

// alarmAccConfig declares projects a and b plus the given alarms.
func alarmAccConfig(project string, alarms ...string) string {
	cfg := accProvider() + fmt.Sprintf(`
resource "honeybadger_project" "a" {
  name = %q
}

resource "honeybadger_project" "b" {
  name = "%s-b"
}
`, project, project)
	for _, a := range alarms {
		cfg += a
	}
	return cfg
}

func TestAccAlarm(t *testing.T) {
	name := accName()
	src := alarmBlock("src", "a", alarmAttrs("source", notices, "5m", "1m", "gt", "10", ""))
	first := alarmAttrs("errors", notices, "5m", "1m", "gt", "10", "")
	updated := func(extra string) string {
		return alarmAttrs("errors-2", `filter event_type::str == \"deploy\" | stats count() as count`, "10m", "2m", "gte", "5", extra)
	}
	narrowed := "  stream_ids = [sort(tolist(honeybadger_alarm.src.stream_ids))[0]]\n"
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{Config: alarmAccConfig(name, src, alarmBlock("t", "a", first))},
			{
				Config:           alarmAccConfig(name, src, alarmBlock("t", "a", updated("  description = \"Deploy spike\"\n"))),
				ConfigPlanChecks: expectUpdate(alarmAddr),
			},
			{
				Config:           alarmAccConfig(name, src, alarmBlock("t", "a", updated("  description = \"Deploy spike\"\n"+narrowed))),
				ConfigPlanChecks: expectUpdate(alarmAddr),
				Check:            resource.TestCheckResourceAttr(alarmAddr, "stream_ids.#", "1"),
			},
			{
				// Review Focus 5: an empty description must read back as "".
				Config:           alarmAccConfig(name, src, alarmBlock("t", "a", updated("  description = \"\"\n"+narrowed))),
				ConfigPlanChecks: expectUpdate(alarmAddr),
			},
			{
				// Removing description clears it; removing stream_ids keeps the list.
				Config: alarmAccConfig(name, src, alarmBlock("t", "a", updated(""))),
				ConfigPlanChecks: expectUpdate(alarmAddr,
					plancheck.ExpectKnownValue(alarmAddr, tfjsonpath.New("description"), knownvalue.Null()),
					plancheck.ExpectKnownValue(alarmAddr, tfjsonpath.New("stream_ids"), knownvalue.SetSizeExact(1)),
				),
			},
			{
				Config: alarmAccConfig(name, src, alarmBlock("t", "b", updated(""))),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(alarmAddr, plancheck.ResourceActionReplace),
				}},
			},
			{
				ResourceName:      alarmAddr,
				ImportState:       true,
				ImportStateIdFunc: importID(alarmAddr),
				ImportStateVerify: true,
			},
		},
	})
}

// A project and an alarm in one apply: the alarm waits for both streams
// instead of storing a partial list.
func TestAccAlarmWithNewProject(t *testing.T) {
	name := accName()
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{{
			Config: alarmAccConfig(name, alarmBlock("t", "a", alarmAttrs("errors", notices, "5m", "1m", "gt", "10", ""))),
			Check:  resource.TestCheckResourceAttr(alarmAddr, "stream_ids.#", "2"),
		}},
	})
}

func TestAccAlarmDeletedOutsideTerraform(t *testing.T) {
	name := accName()
	config := alarmAccConfig(name, alarmBlock("t", "a", alarmAttrs("errors", notices, "5m", "1m", "gt", "10", "")))
	var projectID, alarmID string
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: func(s *terraform.State) error {
					rs := s.RootModule().Resources[alarmAddr]
					projectID, alarmID = rs.Primary.Attributes["project_id"], rs.Primary.ID
					return nil
				},
			},
			{
				PreConfig: func() {
					if err := accClient().Alarms.Delete(context.Background(), projectID, alarmID); err != nil {
						t.Fatalf("deleting the alarm outside Terraform: %v", err)
					}
				},
				Config:             config,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}
