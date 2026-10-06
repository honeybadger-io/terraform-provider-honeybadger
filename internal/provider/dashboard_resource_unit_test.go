package provider

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"

	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

const dashboardAddr = "honeybadger_dashboard.t"

func dashboardHCL(url, widgets string) string {
	return providerConfig(url) + fmt.Sprintf(`
resource "honeybadger_dashboard" "t" {
  project_id = "p1"
  title      = "Ops"
%s}
`, widgets)
}

const (
	errorsWidget = `{ id = "errors", type = "errors", config = { limit = 5 } }`
	visWidget    = `{ id = "rate", type = "insights_vis", grid = { x = 2 }, config = { query = "stats count()", streams = [] } }`
	alarmsWidget = `{ id = "alarms", type = "alarms", grid = { x = 0, y = 3, w = 6, h = 3 } }`
)

func widgetsHCL(ws ...string) string {
	s := "  widgets = jsonencode(["
	for i, w := range ws {
		if i > 0 {
			s += ", "
		}
		s += w
	}
	return s + "])\n"
}

// Review Focus 4: no grid, a partial grid and streams = [] all settle; the
// fake fills grids and streams the way Honeybadger does.
func TestDashboardSettles(t *testing.T) {
	f := newFakeHoneybadger(t)
	cfg := dashboardHCL(f.srv.URL, widgetsHCL(errorsWidget, visWidget))
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{
			{
				Config: cfg,
				Check: lastBodyKey(f, "POST", `/dashboards$`, "widgets", []any{
					map[string]any{"id": "errors", "type": "errors", "config": map[string]any{"limit": float64(5)}},
					map[string]any{"id": "rate", "type": "insights_vis", "grid": map[string]any{"x": float64(2)},
						"config": map[string]any{"query": "stats count()", "streams": []any{}}},
				}),
			},
			{Config: cfg, ConfigPlanChecks: emptyPlan},
		},
	})
}

// Adding, reordering and removing widgets are each an update sending the
// whole list; removing widgets sends the default [].
func TestDashboardWidgetChanges(t *testing.T) {
	f := newFakeHoneybadger(t)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{
			{Config: dashboardHCL(f.srv.URL, widgetsHCL(errorsWidget))},
			{
				Config:           dashboardHCL(f.srv.URL, widgetsHCL(errorsWidget, alarmsWidget)),
				ConfigPlanChecks: expectUpdate(dashboardAddr),
			},
			{
				Config:           dashboardHCL(f.srv.URL, widgetsHCL(alarmsWidget, errorsWidget)),
				ConfigPlanChecks: expectUpdate(dashboardAddr),
				Check: lastBodyKey(f, "PATCH", `/dashboards/`, "widgets", []any{
					map[string]any{"id": "alarms", "type": "alarms", "grid": map[string]any{"x": float64(0), "y": float64(3), "w": float64(6), "h": float64(3)}},
					map[string]any{"id": "errors", "type": "errors", "config": map[string]any{"limit": float64(5)}},
				}),
			},
			{
				Config: dashboardHCL(f.srv.URL, ""),
				Check:  lastBodyKey(f, "PATCH", `/dashboards/`, "widgets", []any{}),
			},
		},
	})
}

// Review Focus 5: a value the user set, changed in the web UI, shows in the
// next plan.
func TestDashboardDrift(t *testing.T) {
	f := newFakeHoneybadger(t)
	cfg := dashboardHCL(f.srv.URL, widgetsHCL(errorsWidget))
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{
			{Config: cfg},
			{
				PreConfig: func() {
					f.mutate(`/dashboards/`, func(o map[string]any) {
						w := o["widgets"].([]any)[0].(map[string]any)
						w["config"] = map[string]any{"limit": float64(9)}
					})
				},
				Config:             cfg,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// A widget without an id, or a misspelt top-level key, fails before any
// request; a stream outside the enum is the API's 422.
func TestDashboardErrors(t *testing.T) {
	f := newFakeHoneybadger(t)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{
			{
				Config:      dashboardHCL(f.srv.URL, widgetsHCL(`{ type = "errors" }`)),
				ExpectError: regexp.MustCompile(`widgets\[0\]: "id" is required`),
			},
			{
				Config:      dashboardHCL(f.srv.URL, widgetsHCL(`{ id = "e", type = "errors", presentaton = { title = "E" } }`)),
				ExpectError: regexp.MustCompile(`presentaton`),
			},
			{
				Config:      dashboardHCL(f.srv.URL, widgetsHCL(`{ id = "v", type = "insights_vis", config = { streams = ["default", "bogus"] } }`)),
				ExpectError: regexp.MustCompile(`isn't one of\s+default, internal`),
			},
		},
	})
	if n := len(f.bodies("POST", `/dashboards$`)); n != 1 {
		t.Errorf("POSTs = %d, want 1 (only the stream case reaches the API)", n)
	}
}

// Review Focus 3: a valid stream the project doesn't have yet (just after
// it's created) is dropped by the server, so the apply fails as
// inconsistent rather than passing.
func TestDashboardDroppedStream(t *testing.T) {
	f := newFakeHoneybadger(t)
	f.projectStreams = []string{"default"}
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{
			{
				Config:      dashboardHCL(f.srv.URL, widgetsHCL(`{ id = "v", type = "insights_vis", config = { streams = ["default", "internal"] } }`)),
				ExpectError: regexp.MustCompile(`inconsistent result after apply`),
			},
		},
	})
}

// Review Focus 1: the same widgets written another way plan an update that
// changes nothing, then settle.
func TestDashboardReformatted(t *testing.T) {
	f := newFakeHoneybadger(t)
	heredoc := "  widgets = <<-EOT\n    [{\"type\": \"errors\", \"config\": {\"limit\": 5}, \"id\": \"errors\"}]\n  EOT\n"
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{
			{Config: dashboardHCL(f.srv.URL, widgetsHCL(errorsWidget))},
			{Config: dashboardHCL(f.srv.URL, heredoc), ConfigPlanChecks: expectUpdate(dashboardAddr)},
			{Config: dashboardHCL(f.srv.URL, heredoc), ConfigPlanChecks: emptyPlan},
		},
	})
}

// Import with the widgets in config: state holds Honeybadger's JSON, so one
// update settles it to the user's, then the plan is empty. Review Focus 2:
// with widgets left out, the plan sends the default [] and removes them.
// The dashboard is made outside Terraform first (an import into an address
// already in state would fail).
func TestDashboardImport(t *testing.T) {
	f := newFakeHoneybadger(t)
	seed := `{"title":"Ops","widgets":[{"id":"errors","type":"errors","config":{"limit":5}}]}`
	resp, err := http.Post(f.srv.URL+"/v3/projects/p1/dashboards", "application/json", strings.NewReader(seed))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	cfg := dashboardHCL(f.srv.URL, widgetsHCL(errorsWidget))
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{
			{Config: cfg, ResourceName: dashboardAddr, ImportState: true, ImportStateId: "p1/x1", ImportStatePersist: true},
			{Config: cfg, ConfigPlanChecks: expectUpdate(dashboardAddr)},
			{Config: cfg, ConfigPlanChecks: emptyPlan},
			{
				Config:           dashboardHCL(f.srv.URL, ""),
				ConfigPlanChecks: expectUpdate(dashboardAddr),
				Check:            lastBodyKey(f, "PATCH", `/dashboards/`, "widgets", []any{}),
			},
		},
	})
}
