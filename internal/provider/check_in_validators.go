package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/honeybadger-io/terraform-provider-honeybadger/internal/provider/customtypes"
)

// checkInConfigValidators enforce the API's schedule rules, which its spec
// states only in prose, so the API never changes a field the plan didn't
// predict (spec 2b, section 4.1).
func checkInConfigValidators() []resource.ConfigValidator {
	return []resource.ConfigValidator{checkInSchedule{}}
}

type checkInSchedule struct{}

func (checkInSchedule) Description(context.Context) string {
	return "simple check-ins need report_period; cron check-ins need cron_schedule"
}

func (v checkInSchedule) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }

func (checkInSchedule) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var scheduleType, cronSchedule, cronTimezone types.String
	var reportPeriod customtypes.PeriodValue
	for name, dst := range map[string]any{
		"schedule_type": &scheduleType, "cron_schedule": &cronSchedule,
		"cron_timezone": &cronTimezone, "report_period": &reportPeriod,
	} {
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root(name), dst)...)
	}
	if resp.Diagnostics.HasError() || scheduleType.IsUnknown() {
		return
	}
	// Config is validated before schema defaults apply, so an omitted
	// schedule_type is the default, simple.
	cron := scheduleType.ValueString() == "cron"

	// Unknown values are left to the API, which answers 422 at apply.
	known := func(v interface {
		IsNull() bool
		IsUnknown() bool
	}) bool {
		return !v.IsNull() && !v.IsUnknown()
	}
	switch {
	case cron && cronSchedule.IsNull():
		resp.Diagnostics.AddAttributeError(path.Root("cron_schedule"), "Missing cron schedule",
			"A cron check-in needs cron_schedule.")
	case cron && known(reportPeriod):
		resp.Diagnostics.AddAttributeError(path.Root("report_period"), "Unexpected report period",
			"A cron check-in doesn't take report_period: the API clears it.")
	case !cron && reportPeriod.IsNull():
		resp.Diagnostics.AddAttributeError(path.Root("report_period"), "Missing report period",
			"A simple check-in (the default schedule_type) needs report_period.")
	case !cron && known(cronSchedule):
		resp.Diagnostics.AddAttributeError(path.Root("cron_schedule"), "Unexpected cron schedule",
			"A simple check-in doesn't take cron_schedule: set schedule_type = \"cron\", or remove it.")
	}
	if !cron && !cronTimezone.IsNull() && !cronTimezone.IsUnknown() && cronTimezone.ValueString() != "UTC" {
		resp.Diagnostics.AddAttributeError(path.Root("cron_timezone"), "Unexpected cron timezone",
			"cron_timezone applies only to cron check-ins; the API resets it to UTC for simple ones.")
	}
	if !reportPeriod.IsNull() && !reportPeriod.IsUnknown() {
		if secs, err := customtypes.ParsePeriod(reportPeriod.ValueString()); err == nil && secs == 0 {
			resp.Diagnostics.AddAttributeError(path.Root("report_period"), "Zero report period",
				"report_period can't be zero.")
		}
	}
}
