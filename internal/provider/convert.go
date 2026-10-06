package provider

import (
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/oapi-codegen/nullable"

	"github.com/honeybadger-io/terraform-provider-honeybadger/internal/provider/customtypes"
)

// Conversions for apiv3 fields that aren't nullable.Nullable: pointers, plain
// values, enums and times. Generated mapping code calls these (spec 3.2).

func createStringPtr(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}

// updateStringPtr sends only a change. A pointer can't carry null, so specgen
// uses it only for attributes that can't be null.
func updateStringPtr(plan, state types.String) *string {
	if plan.IsNull() || plan.IsUnknown() || plan.Equal(state) {
		return nil
	}
	s := plan.ValueString()
	return &s
}

func stringPtrValue(p *string) types.String {
	if p == nil {
		return types.StringNull()
	}
	return types.StringValue(*p)
}

// setEnum and its pointer variants set an apiv3 enum field without naming its
// type, which is internal to api-go: T is inferred from the destination.
func setEnum[T ~string](dst *T, v types.String) {
	if v.IsNull() || v.IsUnknown() {
		return
	}
	*dst = T(v.ValueString())
}

func setEnumPtr[T ~string](dst **T, v types.String) {
	if v.IsNull() || v.IsUnknown() {
		return
	}
	e := T(v.ValueString())
	*dst = &e
}

func updateEnumPtr[T ~string](dst **T, plan, state types.String) {
	if plan.IsNull() || plan.IsUnknown() || plan.Equal(state) {
		return
	}
	e := T(plan.ValueString())
	*dst = &e
}

func periodValue(n nullable.Nullable[string]) customtypes.PeriodValue {
	if v, err := n.Get(); err == nil {
		return customtypes.NewPeriodValue(v)
	}
	return customtypes.NewPeriodNull()
}

func timeValue(t time.Time) types.String {
	return types.StringValue(t.UTC().Format(time.RFC3339))
}

func createBoolPtr(v types.Bool) *bool {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	b := v.ValueBool()
	return &b
}

// updateBoolPtr sends only a change; specgen uses it only for attributes
// that can't be null.
func updateBoolPtr(plan, state types.Bool) *bool {
	if plan.IsNull() || plan.IsUnknown() || plan.Equal(state) {
		return nil
	}
	b := plan.ValueBool()
	return &b
}

func boolPtrValue(p *bool) types.Bool {
	if p == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*p)
}
