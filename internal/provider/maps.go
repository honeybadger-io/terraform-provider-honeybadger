package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Objects that api-go carries as map[string]any (integration config) are
// written and read through these (phase 4b spec, 3.2 and 3.5).

// The one write rule: a value in the plan is sent; a key set in state and
// removed from the plan is sent as null; a key null in both isn't sent.
func put(out map[string]any, key string, plan, state attr.Value, value func() any) {
	switch {
	case plan.IsUnknown():
	case !plan.IsNull():
		out[key] = value()
	case !state.IsNull() && !state.IsUnknown():
		out[key] = nil
	}
}

func putString(out map[string]any, key string, plan, state types.String) {
	put(out, key, plan, state, func() any { return plan.ValueString() })
}

func putBool(out map[string]any, key string, plan, state types.Bool) {
	put(out, key, plan, state, func() any { return plan.ValueBool() })
}

func putStrings(out map[string]any, key string, plan, state collection) {
	put(out, key, plan, state, func() any { return elements(plan, fromString[string]) })
}

func mapString(m map[string]any, key string) types.String {
	if s, ok := m[key].(string); ok {
		return types.StringValue(s)
	}
	return types.StringNull()
}

func mapBool(m map[string]any, key string) types.Bool {
	if b, ok := m[key].(bool); ok {
		return types.BoolValue(b)
	}
	return types.BoolNull()
}

// mapStrings reads a list of strings as a set or a list.
func mapStrings(m map[string]any, key string, set bool) attr.Value {
	raw, ok := m[key].([]any)
	if !ok {
		if set {
			return types.SetNull(types.StringType)
		}
		return types.ListNull(types.StringType)
	}
	vals := make([]attr.Value, 0, len(raw))
	for _, r := range raw {
		if s, ok := r.(string); ok {
			vals = append(vals, types.StringValue(s))
		}
	}
	if set {
		return types.SetValueMust(types.StringType, vals)
	}
	return types.ListValueMust(types.StringType, vals)
}
