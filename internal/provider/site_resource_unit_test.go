package provider

import (
	"fmt"
	"regexp"
	"testing"

	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

const siteAddr = "honeybadger_site.t"

func siteHCL(url, attrs string) string {
	return providerConfig(url) + fmt.Sprintf(`
resource "honeybadger_site" "t" {
  project_id = "p1"
  url        = "https://www.example.com/"
%s}
`, attrs)
}

var emptyPlan = tfresource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}

// Review Focus 1: headers are sent whole; removed, they plan the declared
// default {} and clear on the server. A changed default goes back on removal.
func TestSiteWrites(t *testing.T) {
	f := newFakeHoneybadger(t)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{
			{
				Config: siteHCL(f.srv.URL, "  request_headers = { Authorization = \"Bearer a\" }\n"),
				Check: lastBody(f, "POST", `/sites$`, map[string]any{
					"url": "https://www.example.com/", "frequency": float64(5), "match_type": "success",
					"request_method": "GET", "validate_ssl": true, "active": true, "locations": []any{},
					"request_headers": map[string]any{"Authorization": "Bearer a"},
				}),
			},
			{
				Config: siteHCL(f.srv.URL, "  request_headers = { Authorization = \"Bearer b\", Accept = \"text/html\" }\n"),
				Check: lastBody(f, "PATCH", `/sites/`, map[string]any{
					"request_headers": map[string]any{"Authorization": "Bearer b", "Accept": "text/html"}}),
			},
			{
				Config: siteHCL(f.srv.URL, ""),
				Check:  lastBody(f, "PATCH", `/sites/`, map[string]any{"request_headers": map[string]any{}}),
			},
			{
				Config: siteHCL(f.srv.URL, "  frequency = 15\n"),
				Check:  lastBody(f, "PATCH", `/sites/`, map[string]any{"frequency": float64(15)}),
			},
			{
				Config: siteHCL(f.srv.URL, ""),
				Check:  lastBody(f, "PATCH", `/sites/`, map[string]any{"frequency": float64(5)}),
			},
		},
	})
}

func TestSiteHeadersAreSensitive(t *testing.T) {
	if !siteResourceSchema().Attributes["request_headers"].IsSensitive() {
		t.Error("request_headers isn't Sensitive; its values often carry credentials")
	}
}

// Review Focus 3: locations are a set; reordering plans nothing.
func TestSiteLocationsAreASet(t *testing.T) {
	f := newFakeHoneybadger(t)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{
			{Config: siteHCL(f.srv.URL, "  locations = [\"London\", \"Virginia\"]\n")},
			{Config: siteHCL(f.srv.URL, "  locations = [\"Virginia\", \"London\"]\n"), ConfigPlanChecks: emptyPlan},
		},
	})
}

func TestSiteFrequencyIsValidated(t *testing.T) {
	f := newFakeHoneybadger(t)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{
			{Config: siteHCL(f.srv.URL, "  frequency = 3\n"), ExpectError: regexp.MustCompile(`must be one of`)},
		},
	})
}

// Review Focus 2: a site imports by its UUID; an ID that isn't one fails
// cleanly.
func TestSiteImport(t *testing.T) {
	f := newFakeHoneybadger(t)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{
			{Config: siteHCL(f.srv.URL, "")},
			{ResourceName: siteAddr, ImportState: true, ImportStateIdFunc: importID(siteAddr), ImportStateVerify: true},
			{ResourceName: siteAddr, ImportState: true, ImportStateId: "p1/not-a-uuid", ExpectError: regexp.MustCompile(`isn't a UUID`)},
		},
	})
}
