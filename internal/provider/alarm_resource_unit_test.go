package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/honeybadger-io/api-go/apiv3"
)

func alarmHCL(url, value, extra string) string {
	return providerConfig(url) + fmt.Sprintf(`
resource "honeybadger_alarm" "t" {
  project_id        = "p1"
  name              = "errors"
  query             = "stats count() as count"
  evaluation_period = "5m"
  lookback_lag      = "1m"
  trigger_config = {
    type   = "alert_result_count"
    config = { operator = "gt", value = %s }
  }
%s}
`, value, extra)
}

func trigger(value float64) map[string]any {
	return map[string]any{"type": "alert_result_count", "config": map[string]any{"operator": "gt", "value": value}}
}

const alarmObject = `/alarms/`

func TestAlarmWireBodies(t *testing.T) {
	f := newFakeHoneybadger(t)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{
			{
				// stream_ids is left out, so the server fills it in.
				Config: alarmHCL(f.srv.URL, "10", ""),
				Check: lastBody(f, "POST", `/alarms$`, map[string]any{
					"name": "errors", "query": "stats count() as count", "evaluation_period": "5m",
					"lookback_lag": "1m", "trigger_config": trigger(10),
				}),
			},
			{
				// Changing one nested value sends the whole trigger, and a
				// fractional value round-trips (Review Focus 2).
				Config: alarmHCL(f.srv.URL, "2.5", ""),
				Check:  lastBody(f, "PATCH", alarmObject, map[string]any{"trigger_config": trigger(2.5)}),
			},
			{
				Config: alarmHCL(f.srv.URL, "2.5", "  description = \"spike\"\n"),
				Check:  lastBody(f, "PATCH", alarmObject, map[string]any{"description": "spike"}),
			},
			{
				Config: alarmHCL(f.srv.URL, "2.5", ""),
				Check:  lastBody(f, "PATCH", alarmObject, map[string]any{"description": nil}),
			},
			{
				Config: alarmHCL(f.srv.URL, "2.5", "  stream_ids = [\"s-default\"]\n"),
				Check:  lastBody(f, "PATCH", alarmObject, map[string]any{"stream_ids": []any{"s-default"}}),
			},
			{
				// Removing stream_ids keeps the stored list: no change.
				Config: alarmHCL(f.srv.URL, "2.5", ""),
				ConfigPlanChecks: tfresource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectEmptyPlan(),
				}},
			},
		},
	})
}

func TestAlarmCreateWithStreamIDs(t *testing.T) {
	f := newFakeHoneybadger(t)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{{
			Config: alarmHCL(f.srv.URL, "10", "  stream_ids = [\"s-internal\"]\n"),
			Check: func(s *terraform.State) error {
				bodies := f.bodies("POST", `/alarms$`)
				if got := bodies[len(bodies)-1]["stream_ids"]; !reflect.DeepEqual(got, []any{"s-internal"}) {
					return fmt.Errorf("stream_ids = %v, want [s-internal]", got)
				}
				return nil
			},
		}},
	})
}

func TestAlarmEmptyStreamIDsRejected(t *testing.T) {
	f := newFakeHoneybadger(t)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{{
			Config:      alarmHCL(f.srv.URL, "10", "  stream_ids = []\n"),
			ExpectError: regexp.MustCompile(`at least 1`),
		}},
	})
}

// A 429 while the project's streams are provisioning is retried with the
// same body; honeybadger stored nothing.
func TestAlarmCreateRetriesStreamsProvisioning(t *testing.T) {
	f := newFakeHoneybadger(t)
	f.script("POST", "/v3/projects/p1/alarms", fakeResponse{Status: 429,
		Body:    `{"error":{"code":"streams_provisioning","message":"The project's streams are still being set up"}}`,
		Headers: map[string]string{"Retry-After": "0"}})
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{{
			Config: alarmHCL(f.srv.URL, "10", ""),
			Check: func(*terraform.State) error {
				posts := f.bodies("POST", `/alarms$`)
				if len(posts) != 2 || !reflect.DeepEqual(posts[0], posts[1]) {
					return fmt.Errorf("POSTs = %v, want the same body twice", posts)
				}
				return nil
			},
		}},
	})
}

// Review Focus 1: when provisioning outlasts the retries, the apply fails
// with honeybadger's message, not the unknown-outcome text.
func TestAlarmStreamsProvisioningExhausted(t *testing.T) {
	f := newFakeHoneybadger(t)
	for range 4 {
		f.script("POST", "/v3/projects/p1/alarms", fakeResponse{Status: 429,
			Body:    `{"error":{"code":"streams_provisioning","message":"The project's streams are still being set up"}}`,
			Headers: map[string]string{"Retry-After": "0"}})
	}
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{{
			Config: alarmHCL(f.srv.URL, "10", ""),
			// Terraform wraps long messages, so match the start of it.
			ExpectError: regexp.MustCompile(`429 streams_provisioning: The project's streams`),
		}},
	})
}

// Review Focus 3: a known trigger_config holding an unknown value is an
// error at that attribute, and nothing is converted.
func TestAlarmTriggerConfigNeedsKnownValues(t *testing.T) {
	obj := types.ObjectValueMust(alarmTriggerConfigAttrTypes(), map[string]attr.Value{
		"type": types.StringValue("alert_result_count"),
		"config": types.ObjectValueMust(alarmTriggerConfigConfigAttrTypes(), map[string]attr.Value{
			"operator": types.StringValue("gt"),
			"value":    types.Float64Unknown(),
		}),
	})
	var diags diag.Diagnostics
	if got := alarmTriggerConfigToAPI(obj, path.Root("trigger_config"), &diags); got != nil {
		t.Errorf("got %+v, want nil", got)
	}
	if !diags.HasError() {
		t.Fatal("want an error")
	}
	if d, ok := diags[0].(diag.DiagnosticWithPath); !ok || !d.Path().Equal(path.Root("trigger_config")) {
		t.Errorf("error = %v, want one at trigger_config", diags[0])
	}

	// The wrapper returns before calling the client when createParams or
	// updateParams reports an error, so nothing is sent.
	var paramDiags diag.Diagnostics
	alarmResourceModel{TriggerConfig: obj}.createParams(&paramDiags)
	if !paramDiags.HasError() {
		t.Error("createParams: want an error, so Create sends nothing")
	}
}

// A 422 on a nested field, parsed by apiv3 and mapped by addAPIError, is an
// error at that attribute.
func TestAlarmNestedFieldError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = io.WriteString(w, `{"error":{"code":"validation_error","message":"Validation failed",`+
			`"details":[{"field":"trigger_config.config.value","message":"must be a whole number"}]}}`)
	}))
	defer srv.Close()
	client := apiv3.NewClient().WithBaseURL(srv.URL).WithBearerToken("hbt_test")
	_, err := client.Alarms.Update(context.Background(), "p1", "a1", apiv3.AlarmUpdateParams{})
	var diags diag.Diagnostics
	addAPIError(context.Background(), &diags, alarmResourceSchema(), "Updating alarm", err)
	want := path.Root("trigger_config").AtName("config").AtName("value")
	if len(diags) != 1 {
		t.Fatalf("diags = %v, want one", diags)
	}
	if d, ok := diags[0].(diag.DiagnosticWithPath); !ok || !d.Path().Equal(want) || !strings.Contains(d.Detail(), "whole number") {
		t.Errorf("diag = %v, want one at %v saying whole number", diags[0], want)
	}
}

// The alarm service's bug: an update answered 200 with the old trigger
// surfaces as Terraform's inconsistent-result error.
func TestAlarmUpdateKeepingOldTrigger(t *testing.T) {
	f := newFakeHoneybadger(t)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{
			{Config: alarmHCL(f.srv.URL, "10", "")},
			{
				PreConfig: func() {
					f.script("PATCH", "*", fakeResponse{Status: 200, Body: f.snapshot(alarmObject)})
				},
				Config:      alarmHCL(f.srv.URL, "20", ""),
				ExpectError: regexp.MustCompile(`inconsistent result after apply`),
			},
		},
	})
}
