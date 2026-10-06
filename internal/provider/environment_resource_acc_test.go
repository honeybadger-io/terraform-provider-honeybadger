package provider

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/honeybadger-io/api-go/apiv3"
)

func environmentAccConfig(project, env, attrs string) string {
	return accProvider() + fmt.Sprintf(`
resource "honeybadger_project" "a" {
  name = %q
}

resource "honeybadger_environment" "t" {
  project_id = honeybadger_project.a.id
  name       = %q
%s}
`, project, env, attrs)
}

func TestAccEnvironment(t *testing.T) {
	name := accName()
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{Config: environmentAccConfig(name, "staging", "")},
			{
				Config:           environmentAccConfig(name, "staging", "  notifications = false\n"),
				ConfigPlanChecks: expectUpdate(environmentAddr),
			},
			{
				Config: environmentAccConfig(name, "qa", "  notifications = false\n"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(environmentAddr, plancheck.ResourceActionReplace),
				}},
			},
			{ResourceName: environmentAddr, ImportState: true, ImportStateIdFunc: importID(environmentAddr), ImportStateVerify: true},
		},
	})
}

// An environment that exists before Terraform manages it, as one Honeybadger
// created for an incoming error would, is adopted with an import block (the
// way the docs show) and then plans nothing. The project and environment are
// made through the API first, so the import block's ID is known at plan time.
func TestAccEnvironmentAdopt(t *testing.T) {
	// The setup below calls the API, so skip as resource.Test would.
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
	e, err := accClient().Environments.Create(ctx, p.Id, apiv3.EnvironmentCreateParams{Name: "adopted"})
	if err != nil {
		t.Fatalf("creating the environment outside Terraform: %v", err)
	}
	config := accProvider() + fmt.Sprintf(`
import {
  to = honeybadger_environment.adopted
  id = "%[1]s/%[2]s"
}

resource "honeybadger_environment" "adopted" {
  project_id = %[1]q
  name       = "adopted"
}
`, p.Id, e.Id)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("honeybadger_environment.adopted", plancheck.ResourceActionNoop),
				}},
			},
			{Config: config, PlanOnly: true},
		},
	})
}
