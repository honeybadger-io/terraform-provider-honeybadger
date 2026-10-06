package provider

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// lastBody checks the most recent request body with this method and path
// pattern is exactly want.
func lastBody(f *fakeHoneybadger, method, pattern string, want map[string]any) tfresource.TestCheckFunc {
	return func(*terraform.State) error {
		bodies := f.bodies(method, pattern)
		if len(bodies) == 0 {
			return fmt.Errorf("no %s request to %s", method, pattern)
		}
		if got := bodies[len(bodies)-1]; !reflect.DeepEqual(got, want) {
			return fmt.Errorf("%s %s body = %v, want %v", method, pattern, got, want)
		}
		return nil
	}
}

func checkInConfigHCL(url, extra string) string {
	return providerConfig(url) + fmt.Sprintf(`
resource "honeybadger_check_in" "t" {
  project_id    = "p1"
  name          = "nightly"
  report_period = "1 day"
%s}
`, extra)
}

// Defaults are sent on create; unset attributes are left out; an update sends
// only what changed, and removing an attribute sends null.
func TestCheckInWireBodies(t *testing.T) {
	f := newFakeHoneybadger(t)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{
			{
				Config: checkInConfigHCL(f.srv.URL, ""),
				Check: lastBody(f, "POST", `/check_ins$`, map[string]any{
					"name": "nightly", "report_period": "1 day", "schedule_type": "simple",
					"grace_period": "00:00:00", "cron_timezone": "UTC",
				}),
			},
			{
				Config: checkInConfigHCL(f.srv.URL, "  slug = \"nightly-job\"\n  grace_period = \"5 minutes\"\n"),
				Check: lastBody(f, "PATCH", `/check_ins/`, map[string]any{
					"slug": "nightly-job", "grace_period": "5 minutes",
				}),
			},
			{
				Config: checkInConfigHCL(f.srv.URL, "  grace_period = \"5 minutes\"\n"),
				Check:  lastBody(f, "PATCH", `/check_ins/`, map[string]any{"slug": nil}),
			},
		},
	})
}

func projectConfigHCL(url, name, extra string) string {
	return providerConfig(url) + fmt.Sprintf(`
resource "honeybadger_project" "t" {
  name = %q
%s}
`, name, extra)
}

func TestProjectWireBodies(t *testing.T) {
	f := newFakeHoneybadger(t)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{
			{
				Config: projectConfigHCL(f.srv.URL, "tf-unit", "  language = \"ruby\"\n"),
				Check: lastBody(f, "POST", `^/v3/projects$`, map[string]any{
					"name": "tf-unit", "language": "ruby",
					"disable_public_links": false, "resolve_errors_on_deploy": true,
				}),
			},
			{
				// name is a pointer on update: only the change is sent.
				Config: projectConfigHCL(f.srv.URL, "tf-unit-2", "  language = \"ruby\"\n"),
				Check:  lastBody(f, "PATCH", `^/v3/projects/`, map[string]any{"name": "tf-unit-2"}),
			},
			{
				Config: projectConfigHCL(f.srv.URL, "tf-unit-2", ""),
				Check:  lastBody(f, "PATCH", `^/v3/projects/`, map[string]any{"language": nil}),
			},
		},
	})
}

func keyConfigHCL(url, extra string) string {
	return providerConfig(url) + fmt.Sprintf(`
resource "honeybadger_project_key" "t" {
  project_id = "p1"
%s}
`, extra)
}

func TestProjectKeyWireBodies(t *testing.T) {
	f := newFakeHoneybadger(t)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{
			{
				Config: keyConfigHCL(f.srv.URL, "  label = \"deploys\"\n"),
				Check:  lastBody(f, "POST", `/keys$`, map[string]any{"label": "deploys"}),
			},
			{
				Config: keyConfigHCL(f.srv.URL, ""),
				Check:  lastBody(f, "PATCH", `/keys/`, map[string]any{"label": nil}),
			},
		},
	})
}

// A 404 on read means the object was deleted outside Terraform: it leaves
// state, so the next plan re-creates it.
func TestReadNotFoundRemovesResource(t *testing.T) {
	f := newFakeHoneybadger(t)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{
			{Config: checkInConfigHCL(f.srv.URL, "")},
			{
				PreConfig:          func() { f.remove(`/check_ins/`) },
				Config:             checkInConfigHCL(f.srv.URL, ""),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// A 404 on delete counts as deleted. The scripted 404 is queued before the
// last step; no DELETE happens during the steps, so the harness's destroy
// consumes it.
func TestDeleteNotFoundSucceeds(t *testing.T) {
	f := newFakeHoneybadger(t)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{
			{Config: projectConfigHCL(f.srv.URL, "tf-unit", "")},
			{
				PreConfig: func() {
					f.script("DELETE", "*", fakeResponse{Status: 404,
						Body: `{"error":{"code":"not_found","message":"Not found"}}`})
				},
				Config: projectConfigHCL(f.srv.URL, "tf-unit", ""),
			},
		},
	})
}

// A 2xx without a usable id may still have created the check-in.
func TestCreateWithEmptyResultIsUnknownOutcome(t *testing.T) {
	f := newFakeHoneybadger(t)
	f.script("POST", "/v3/projects/p1/check_ins", fakeResponse{Status: 201, Body: `{"data":{}}`})
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{
			{
				Config:      checkInConfigHCL(f.srv.URL, ""),
				ExpectError: regexp.MustCompile("Create may have succeeded"),
			},
		},
	})
}

// A 429 is retried by api-go's policy, resending the same body.
func TestCreateRetriesRateLimit(t *testing.T) {
	f := newFakeHoneybadger(t)
	f.script("POST", "/v3/projects", fakeResponse{Status: 429, Body: `{"error":{"code":"rate_limit_exceeded","message":"slow down"}}`,
		Headers: map[string]string{"Retry-After": "0"}})
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{
			{
				Config: projectConfigHCL(f.srv.URL, "tf-unit", ""),
				Check: func(*terraform.State) error {
					posts := f.bodies("POST", `^/v3/projects$`)
					if len(posts) != 2 || !reflect.DeepEqual(posts[0], posts[1]) {
						return fmt.Errorf("POSTs = %v, want the same body twice", posts)
					}
					return nil
				},
			},
		},
	})
}

// Review Focus 4: Terraform validates config before configuring the provider,
// so a resource's Configure sees nil provider data.
func TestConfigureWithoutProviderData(t *testing.T) {
	r := newCheckInResource().(resource.ResourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	r.Configure(context.Background(), resource.ConfigureRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Errorf("diagnostics = %v, want none", resp.Diagnostics)
	}
}
