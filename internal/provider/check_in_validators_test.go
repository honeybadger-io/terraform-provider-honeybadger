package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// checkInConfig builds a check-in config with every attribute null except
// those given.
func checkInConfig(t *testing.T, set map[string]tftypes.Value) tfsdk.Config {
	t.Helper()
	ctx := context.Background()
	s := checkInResourceSchema()
	vals := map[string]tftypes.Value{}
	for name, a := range s.Attributes {
		vals[name] = tftypes.NewValue(a.GetType().TerraformType(ctx), nil)
	}
	for k, v := range set {
		vals[k] = v
	}
	return tfsdk.Config{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), vals)}
}

func str(s string) tftypes.Value { return tftypes.NewValue(tftypes.String, s) }

var unknownStr = tftypes.NewValue(tftypes.String, tftypes.UnknownValue)

func validateCheckIn(t *testing.T, set map[string]tftypes.Value) bool {
	t.Helper()
	req := resource.ValidateConfigRequest{Config: checkInConfig(t, set)}
	var errs int
	for _, v := range checkInConfigValidators() {
		resp := &resource.ValidateConfigResponse{}
		v.ValidateResource(context.Background(), req, resp)
		errs += resp.Diagnostics.ErrorsCount()
	}
	return errs == 0
}

func TestCheckInScheduleRules(t *testing.T) {
	tests := []struct {
		name string
		set  map[string]tftypes.Value
		ok   bool
	}{
		// Review Focus 5: an omitted schedule_type is simple.
		{"omitted type needs report_period", map[string]tftypes.Value{}, false},
		{"omitted type with report_period", map[string]tftypes.Value{"report_period": str("1 day")}, true},
		{"simple with cron_schedule", map[string]tftypes.Value{"schedule_type": str("simple"), "report_period": str("1 day"), "cron_schedule": str("0 3 * * *")}, false},
		{"cron needs cron_schedule", map[string]tftypes.Value{"schedule_type": str("cron")}, false},
		{"cron with report_period", map[string]tftypes.Value{"schedule_type": str("cron"), "cron_schedule": str("0 3 * * *"), "report_period": str("1 day")}, false},
		{"cron valid", map[string]tftypes.Value{"schedule_type": str("cron"), "cron_schedule": str("0 3 * * *"), "cron_timezone": str("Eastern Time (US & Canada)")}, true},
		{"simple with non-UTC timezone", map[string]tftypes.Value{"report_period": str("1 day"), "cron_timezone": str("Eastern Time (US & Canada)")}, false},
		{"simple with UTC timezone", map[string]tftypes.Value{"report_period": str("1 day"), "cron_timezone": str("UTC")}, true},
		{"zero report_period", map[string]tftypes.Value{"report_period": str("00:00:00")}, false},
		{"unknown schedule_type skips", map[string]tftypes.Value{"schedule_type": unknownStr}, true},
		{"unknown report_period skips", map[string]tftypes.Value{"report_period": unknownStr}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateCheckIn(t, tt.set); got != tt.ok {
				t.Errorf("valid = %v, want %v", got, tt.ok)
			}
		})
	}
}
