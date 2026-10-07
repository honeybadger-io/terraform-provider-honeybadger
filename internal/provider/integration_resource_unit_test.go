package provider

import (
	"fmt"
	"regexp"
	"testing"

	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func webHookHCL(url, config string) string {
	return providerConfig(url) + fmt.Sprintf(`
resource "honeybadger_integration_web_hook" "t" {
  project_id = "p1"
  config = {
%s  }
}
`, config)
}

// Review Focus 1 and 4: the one write rule on a merged object.
func TestWebHookConfigWrites(t *testing.T) {
	f := newFakeHoneybadger(t)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{
			{
				Config: webHookHCL(f.srv.URL, "    url = \"https://a.example/hook\"\n    bearer_token = \"s1\"\n"),
				Check: lastBody(f, "POST", `/integrations$`, map[string]any{
					"type": "WebHook", "active": true,
					"config":          map[string]any{"url": "https://a.example/hook", "bearer_token": "s1", "payload_version": "v2"},
					"alarm_alert_ids": []any{}, "alarm_ok_ids": []any{},
					"included_environments": []any{}, "excluded_environments": []any{}, "filters": []any{},
					"rate": "wday", "threshold": float64(100),
				}),
			},
			{
				// One key changes: every set key is resent.
				Config: webHookHCL(f.srv.URL, "    url = \"https://b.example/hook\"\n    bearer_token = \"s1\"\n"),
				Check: lastBody(f, "PATCH", `/integrations/`, map[string]any{
					"config": map[string]any{"url": "https://b.example/hook", "bearer_token": "s1", "payload_version": "v2"},
				}),
			},
			{
				// The secret is removed: it's sent as null.
				Config: webHookHCL(f.srv.URL, "    url = \"https://b.example/hook\"\n"),
				Check: lastBody(f, "PATCH", `/integrations/`, map[string]any{
					"config": map[string]any{"url": "https://b.example/hook", "bearer_token": nil, "payload_version": "v2"},
				}),
			},
		},
	})
}

// Review Focus 1: a secret set outside Terraform (stored on the server, never
// in config or state) isn't sent when another key changes, so it's kept.
func TestWebHookSecretSetOutsideTerraformIsKept(t *testing.T) {
	f := newFakeHoneybadger(t)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{
			{Config: webHookHCL(f.srv.URL, "    url = \"https://a.example/hook\"\n")},
			{
				PreConfig: func() {
					f.mutate(`/integrations/`, func(obj map[string]any) {
						obj["config"].(map[string]any)["bearer_token"] = "set-in-the-ui"
					})
				},
				Config: webHookHCL(f.srv.URL, "    url = \"https://c.example/hook\"\n"),
				Check: func(s *terraform.State) error {
					if err := lastBody(f, "PATCH", `/integrations/`, map[string]any{
						"config": map[string]any{"url": "https://c.example/hook", "payload_version": "v2"},
					})(s); err != nil {
						return err
					}
					var kept any
					f.mutate(`/integrations/`, func(obj map[string]any) { kept = obj["config"].(map[string]any)["bearer_token"] })
					if kept != "set-in-the-ui" {
						return fmt.Errorf("stored bearer_token = %v, want it kept", kept)
					}
					return nil
				},
			},
		},
	})
}

// Review Focus 2: another type's integration read through web_hook.
func TestIntegrationWrongType(t *testing.T) {
	f := newFakeHoneybadger(t)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{
			{Config: webHookHCL(f.srv.URL, "    url = \"https://a.example/hook\"\n")},
			{
				PreConfig: func() {
					f.script("GET", "*", fakeResponse{Status: 200,
						Body: `{"data":{"id":"x1","project_id":"p1","type":"Slack","config":{},"links":{"web":"w"}}}`})
				},
				Config:      webHookHCL(f.srv.URL, "    url = \"https://a.example/hook\"\n"),
				ExpectError: regexp.MustCompile(`honeybadger_integration_slack`),
			},
		},
	})
}

// Review Focus 3: lists left out plan [] and read [] with no diff.
func TestIntegrationListsDefaultEmpty(t *testing.T) {
	f := newFakeHoneybadger(t)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{
			{Config: webHookHCL(f.srv.URL, "    url = \"https://a.example/hook\"\n")},
			{
				Config:           webHookHCL(f.srv.URL, "    url = \"https://a.example/hook\"\n"),
				ConfigPlanChecks: tfresource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
		},
	})
}

func TestIntercomRejectsEvents(t *testing.T) {
	f := newFakeHoneybadger(t)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{{
			Config: providerConfig(f.srv.URL) + `
resource "honeybadger_integration_intercom" "t" {
  project_id = "p1"
  events     = ["occurred"]
  config     = { context_field = "user_id" }
}
`,
			ExpectError: regexp.MustCompile(`at most 0`),
		}},
	})
}

func TestSlackConfigOptional(t *testing.T) {
	f := newFakeHoneybadger(t)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{{
			Config: providerConfig(f.srv.URL) + `
resource "honeybadger_integration_slack" "t" {
  project_id = "p1"
}
`,
			Check: tfresource.TestCheckResourceAttrSet("honeybadger_integration_slack.t", "links.web"),
		}},
	})
}

func jiraHCL(url, extra string) string {
	return providerConfig(url) + fmt.Sprintf(`
resource "honeybadger_integration_jira" "t" {
  project_id = "p1"
  config = {
    username    = "u"
    password    = "p"
    server_url  = "https://jira.example.com"
    project_key = "DEMO"
%s  }
}
`, extra)
}

// Review Focus 5: a defaulted config key plans its default when left out,
// and removing a set value sends the default back.
func TestJiraDefaultedKey(t *testing.T) {
	f := newFakeHoneybadger(t)
	jiraBody := func(resolve string) map[string]any {
		return map[string]any{"config": map[string]any{
			"username": "u", "password": "p", "server_url": "https://jira.example.com", "project_key": "DEMO",
			"resolve_transition": resolve, "reopen_transition": "3", "issue_type": "Bug",
		}}
	}
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{
			{
				Config: jiraHCL(f.srv.URL, ""),
				Check:  tfresource.TestCheckResourceAttr("honeybadger_integration_jira.t", "config.resolve_transition", "5"),
			},
			{
				Config: jiraHCL(f.srv.URL, "    resolve_transition = \"7\"\n"),
				Check:  lastBody(f, "PATCH", `/integrations/`, jiraBody("7")),
			},
			{
				Config: jiraHCL(f.srv.URL, ""),
				Check:  lastBody(f, "PATCH", `/integrations/`, jiraBody("5")),
			},
		},
	})
}

func TestIntegrationLegacyType(t *testing.T) {
	f := newFakeHoneybadger(t)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{
			{Config: webHookHCL(f.srv.URL, "    url = \"https://a.example/hook\"\n")},
			{
				PreConfig: func() {
					f.script("GET", "*", fakeResponse{Status: 200,
						Body: `{"data":{"id":"x1","project_id":"p1","type":"HipChat","config":{},"links":{"web":"w"}}}`})
				},
				Config:      webHookHCL(f.srv.URL, "    url = \"https://a.example/hook\"\n"),
				ExpectError: regexp.MustCompile(`can't manage`),
			},
		},
	})
}

func TestIntegrationMalformedSiteID(t *testing.T) {
	f := newFakeHoneybadger(t)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{{
			Config: providerConfig(f.srv.URL) + `
resource "honeybadger_integration_web_hook" "t" {
  project_id = "p1"
  site_ids   = ["not-a-uuid"]
  config     = { url = "https://a.example/hook" }
}
`,
			ExpectError: regexp.MustCompile(`must be a lowercase UUID`),
		}},
	})
}

// A non-empty filters list round-trips, and events left out are filled by
// the server.
func TestIntegrationFiltersAndEvents(t *testing.T) {
	f := newFakeHoneybadger(t)
	config := providerConfig(f.srv.URL) + `
resource "honeybadger_integration_web_hook" "t" {
  project_id = "p1"
  filters    = [{ event = "all", query = "class:Foo" }]
  config     = { url = "https://a.example/hook" }
}
`
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{
			{
				Config: config,
				Check: tfresource.ComposeTestCheckFunc(
					tfresource.TestCheckResourceAttr("honeybadger_integration_web_hook.t", "filters.0.query", "class:Foo"),
					tfresource.TestCheckResourceAttr("honeybadger_integration_web_hook.t", "events.#", "1"),
				),
			},
			{
				Config:           config,
				ConfigPlanChecks: tfresource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
		},
	})
}

// UUIDs read back lowercase, so an uppercase one in config would never match.
func TestIntegrationUppercaseSiteID(t *testing.T) {
	f := newFakeHoneybadger(t)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{{
			Config: providerConfig(f.srv.URL) + `
resource "honeybadger_integration_web_hook" "t" {
  project_id = "p1"
  site_ids   = ["0B6A3C1E-7B4F-4A8E-9F00-1C2D3E4F5A6B"]
  config     = { url = "https://a.example/hook" }
}
`,
			ExpectError: regexp.MustCompile(`lowercase UUID`),
		}},
	})
}

// links is stable, so an update doesn't plan it as unknown.
func TestIntegrationLinksStableOnUpdate(t *testing.T) {
	f := newFakeHoneybadger(t)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{
			{Config: webHookHCL(f.srv.URL, "    url = \"https://a.example/hook\"\n")},
			{
				Config: webHookHCL(f.srv.URL, "    url = \"https://b.example/hook\"\n"),
				ConfigPlanChecks: tfresource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectKnownValue("honeybadger_integration_web_hook.t", tfjsonpath.New("links").AtMapKey("web"),
						knownvalue.StringRegexp(regexp.MustCompile(`/integrations/`))),
				}},
			},
		},
	})
}

// site_ids (and check_in_ids) mean three things (honeybadger 854fd20da7):
// left out, every site; [] none; a list, just those. Removing the list from
// config sends null, which goes back to every site.
func TestIntegrationSiteIDs(t *testing.T) {
	f := newFakeHoneybadger(t)
	hcl := func(siteIDs string) string {
		return providerConfig(f.srv.URL) + fmt.Sprintf(`
resource "honeybadger_integration_web_hook" "t" {
  project_id = "p1"
%s  config = { url = "https://a.example/hook" }
}
`, siteIDs)
	}
	const site = "0b6f2b8e-4f7c-4c1a-9a51-6c1f0e1b2a3d"
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{
			{
				// Left out: not sent, so every site; the read's null matches.
				Config: hcl(""),
				Check:  noBodyKey(f, "POST", `/integrations$`, "site_ids"),
			},
			{
				Config: hcl(fmt.Sprintf("  site_ids = [%q]\n", site)),
				Check:  lastBodyKey(f, "PATCH", `/integrations/`, "site_ids", []any{site}),
			},
			{
				Config: hcl("  site_ids = []\n"),
				Check:  lastBodyKey(f, "PATCH", `/integrations/`, "site_ids", []any{}),
			},
			{
				// Removed from config: null, back to every site.
				Config: hcl(""),
				Check:  lastBodyKey(f, "PATCH", `/integrations/`, "site_ids", nil),
			},
		},
	})
}

// noBodyKey checks the last matching request's body leaves key out.
func noBodyKey(f *fakeHoneybadger, method, pattern, key string) tfresource.TestCheckFunc {
	return func(*terraform.State) error {
		bodies := f.bodies(method, pattern)
		if len(bodies) == 0 {
			return fmt.Errorf("no %s request to %s", method, pattern)
		}
		if v, ok := bodies[len(bodies)-1][key]; ok {
			return fmt.Errorf("%s %s sent %s = %v, want it left out", method, pattern, key, v)
		}
		return nil
	}
}
