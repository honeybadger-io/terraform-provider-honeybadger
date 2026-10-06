package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// integrationAccConfig declares a project and one integration of the given
// type, with the given attributes.
func integrationAccConfig(project, typ, attrs string) string {
	return accProvider() + fmt.Sprintf(`
resource "honeybadger_project" "a" {
  name = %q
}

resource "honeybadger_integration_%s" "t" {
  project_id = honeybadger_project.a.id
%s}
`, project, typ, attrs)
}

// hclObject renders a JSON object as an HCL object expression.
func hclObject(m map[string]any) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s = %s", k, hclValue(m[k])))
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}

func hclValue(v any) string {
	switch v := v.(type) {
	case bool:
		return fmt.Sprint(v)
	case string:
		return fmt.Sprintf("%q", v)
	}
	b, _ := json.Marshal(v)
	return string(b)
}

const webHookAddr = "honeybadger_integration_web_hook.t"

func TestAccIntegrationWebHook(t *testing.T) {
	name := accName()
	base := "  config = { url = \"https://93.184.216.34/hook\", bearer_token = \"s1\" }\n"
	updated := `  active                = false
  events                = ["occurred", "resolved"]
  filters               = [{ event = "all", query = "class:Foo" }]
  check_in_ids          = []
  alarm_alert_ids       = []
  included_environments = ["production"]
  rate                  = "hour"
  threshold             = 10
  notification_limit    = 5
`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{Config: integrationAccConfig(name, "web_hook", base)},
			{
				Config:           integrationAccConfig(name, "web_hook", base+updated),
				ConfigPlanChecks: expectUpdate(webHookAddr),
			},
			{
				Config: integrationAccConfig(name, "web_hook",
					"  config = { url = \"https://93.184.216.35/hook\", bearer_token = \"s1\" }\n"+updated),
				ConfigPlanChecks: expectUpdate(webHookAddr),
			},
			{
				// Removing the secret is a planned change, sent as null.
				Config: integrationAccConfig(name, "web_hook", "  config = { url = \"https://93.184.216.35/hook\" }\n"+updated),
				ConfigPlanChecks: expectUpdate(webHookAddr,
					plancheck.ExpectKnownValue(webHookAddr, tfjsonpath.New("config").AtMapKey("bearer_token"), knownvalue.Null()),
				),
			},
			{
				ResourceName:            webHookAddr,
				ImportState:             true,
				ImportStateIdFunc:       importID(webHookAddr),
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"config.bearer_token"},
			},
		},
	})
}

func TestAccIntegrationWebHookDeletedOutsideTerraform(t *testing.T) {
	name := accName()
	config := integrationAccConfig(name, "web_hook", "  config = { url = \"https://93.184.216.34/hook\" }\n")
	var projectID, id string
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: func(s *terraform.State) error {
					rs := s.RootModule().Resources[webHookAddr]
					projectID, id = rs.Primary.Attributes["project_id"], rs.Primary.ID
					return nil
				},
			},
			{
				PreConfig: func() {
					if err := accClient().Integrations.Delete(context.Background(), projectID, id); err != nil {
						t.Fatalf("deleting the integration outside Terraform: %v", err)
					}
				},
				Config:             config,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccIntegrationEmail(t *testing.T) {
	name := accName()
	addr := "honeybadger_integration_email.t"
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{Config: integrationAccConfig(name, "email", "  config = { email = \"dev@example.com\" }\n")},
			{
				Config:           integrationAccConfig(name, "email", "  config = { email = \"ops@example.com\" }\n"),
				ConfigPlanChecks: expectUpdate(addr),
			},
			{ResourceName: addr, ImportState: true, ImportStateIdFunc: importID(addr), ImportStateVerify: true},
		},
	})
}

// An OAuth type is created active and unconnected; nothing changes until
// someone connects it at links.web.
func TestAccIntegrationSlack(t *testing.T) {
	name := accName()
	addr := "honeybadger_integration_slack.t"
	config := integrationAccConfig(name, "slack", "  active = true\n")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{Config: config, Check: resource.TestCheckResourceAttrSet(addr, "links.web")},
			{
				Config:           config,
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
		},
	})
}

type integrationType struct {
	Resource string         `json:"resource"`
	Config   map[string]any `json:"config"`
	Active   *bool          `json:"active"`
	Update   struct {
		Key   string `json:"key"`
		Value any    `json:"value"`
	} `json:"update"`
}

// Every type that creates without an external call, with honeybadger's
// minimal config: a type whose read doesn't match its schema fails here
// (phase 4b spec, 5.3). Sequential, to stay within the rate limit.
func TestAccIntegrationTypes(t *testing.T) {
	raw, err := os.ReadFile("testdata/integration_types.json")
	if err != nil {
		t.Fatal(err)
	}
	var types []integrationType
	if err := json.Unmarshal(raw, &types); err != nil {
		t.Fatal(err)
	}
	for _, it := range types {
		t.Run(it.Resource, func(t *testing.T) {
			name := accName()
			attrs := func(change bool) string {
				cfg := map[string]any{}
				for k, v := range it.Config {
					cfg[k] = v
				}
				s := ""
				if it.Active != nil {
					s += fmt.Sprintf("  active = %t\n", *it.Active)
				}
				if change {
					if it.Update.Key == "active" {
						s = fmt.Sprintf("  active = %s\n", hclValue(it.Update.Value))
					} else {
						cfg[it.Update.Key] = it.Update.Value
					}
				}
				return s + "  config = " + hclObject(cfg) + "\n"
			}
			addr := "honeybadger_integration_" + it.Resource + ".t"
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testFactories,
				PreCheck:                 func() { testAccPreCheck(t) },
				Steps: []resource.TestStep{
					{Config: integrationAccConfig(name, it.Resource, attrs(false))},
					{
						Config:           integrationAccConfig(name, it.Resource, attrs(true)),
						ConfigPlanChecks: expectUpdate(addr),
					},
				},
			})
		})
	}
}
