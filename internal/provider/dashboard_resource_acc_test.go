package provider

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/honeybadger-io/api-go/apiv3"
)

func dashboardAccConfig(name, widgets string) string {
	return accProvider() + fmt.Sprintf(`
resource "honeybadger_project" "a" {
  name = %[1]q
}

resource "honeybadger_dashboard" "t" {
  project_id = honeybadger_project.a.id
  title      = %[1]q
  widgets    = jsonencode([%[2]s])
}
`, name, widgets)
}

func TestAccDashboard(t *testing.T) {
	name := accName()
	// The alarms widget's title uses the project's id, unknown until the
	// project exists: widgets is unknown at plan and settles on the first
	// apply.
	alarms := `{ id = "alarms", type = "alarms", grid = { x = 6, y = 3, w = 6, h = 3 }, presentation = { title = "Alarms for ${honeybadger_project.a.id}" }, config = { filter_state = "triggered" } }`
	vis := `{ id = "rate", type = "insights_vis", grid = { x = 2 }, config = { query = "stats count() as count by bin(1h) as time", vis = { view = "line", chart_config = { xField = "time", yField = "count" } } } }`
	errs := `{ id = "errors", type = "errors", grid = { x = 0, y = 3, w = 6, h = 3 }, config = { limit = 5 } }`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{Config: dashboardAccConfig(name, alarms+", "+vis)},
			{Config: dashboardAccConfig(name, alarms+", "+vis+", "+errs), ConfigPlanChecks: expectUpdate(dashboardAddr)},
			{Config: dashboardAccConfig(name, errs+", "+alarms+", "+vis), ConfigPlanChecks: expectUpdate(dashboardAddr)},
			{Config: dashboardAccConfig(name, errs+", "+alarms), ConfigPlanChecks: expectUpdate(dashboardAddr)},
			{ResourceName: dashboardAddr, ImportState: true, ImportStateIdFunc: importID(dashboardAddr), ImportStateVerify: true, ImportStateVerifyIgnore: []string{"widgets"}},
		},
	})
}

// A dashboard made outside Terraform is adopted with an import block, as the
// docs show: the plan imports it and updates widgets to the config; the
// next plan is empty.
func TestAccDashboardAdopt(t *testing.T) {
	if os.Getenv(resource.EnvTfAcc) == "" {
		t.Skipf("acceptance test: set %s", resource.EnvTfAcc)
	}
	testAccPreCheck(t)
	ctx := context.Background()
	p, err := accClient().Projects.Create(ctx, apiv3.ProjectCreateParams{Name: accName()})
	if err != nil {
		t.Fatalf("creating the project: %v", err)
	}
	t.Cleanup(func() { _ = accClient().Projects.Delete(context.Background(), p.Id) })
	id := "errors"
	widgets := []apiv3.DashboardWidget{{Id: &id, Type: "errors"}}
	d, err := accClient().Dashboards.Create(ctx, p.Id, apiv3.DashboardCreateParams{Title: "Adopted", Widgets: &widgets})
	if err != nil {
		t.Fatalf("creating the dashboard outside Terraform: %v", err)
	}
	config := accProvider() + fmt.Sprintf(`
import {
  to = honeybadger_dashboard.adopted
  id = "%[1]s/%[2]s"
}

resource "honeybadger_dashboard" "adopted" {
  project_id = %[1]q
  title      = "Adopted"
  widgets    = jsonencode([{ id = "errors", type = "errors" }])
}
`, p.Id, d.Id)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []resource.TestStep{
			{Config: config},
			{Config: config, PlanOnly: true},
		},
	})
}
