package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/oapi-codegen/nullable"
)

// apiv3's request types use nullable.Nullable for fields the API can clear. It
// tells three states apart: unset (left out of the request, so the field is
// unchanged), null (sent as JSON null, which clears or resets the field), and a
// value. These helpers convert Terraform values to and from it.

func createString(v types.String) nullable.Nullable[string] {
	return createValue(v.IsNull() || v.IsUnknown(), v.ValueString())
}

func createInt(v types.Int64) nullable.Nullable[int] {
	return createValue(v.IsNull() || v.IsUnknown(), int(v.ValueInt64()))
}

func createBool(v types.Bool) nullable.Nullable[bool] {
	return createValue(v.IsNull() || v.IsUnknown(), v.ValueBool())
}

// createValue leaves a missing value out so the server's default applies.
func createValue[T any](missing bool, v T) nullable.Nullable[T] {
	if missing {
		return nullable.Nullable[T]{}
	}
	return nullable.NewNullableWithValue(v)
}

func updateString(plan, state types.String) nullable.Nullable[string] {
	return updateValue(plan.IsUnknown(), plan.IsNull(), plan.Equal(state), plan.ValueString())
}

func updateInt(plan, state types.Int64) nullable.Nullable[int] {
	return updateValue(plan.IsUnknown(), plan.IsNull(), plan.Equal(state), int(plan.ValueInt64()))
}

func updateBool(plan, state types.Bool) nullable.Nullable[bool] {
	return updateValue(plan.IsUnknown(), plan.IsNull(), plan.Equal(state), plan.ValueBool())
}

// updateValue sends only what changed. A value removed from config is sent as
// null, which clears it; v3 accepts null only on fields marked nullable, so
// callers use these helpers only for those fields.
func updateValue[T any](unknown, null, unchanged bool, v T) nullable.Nullable[T] {
	switch {
	case unknown, unchanged:
		return nullable.Nullable[T]{}
	case null:
		return nullable.NewNullNullable[T]()
	default:
		return nullable.NewNullableWithValue(v)
	}
}

func stringValue(n nullable.Nullable[string]) types.String {
	if v, err := n.Get(); err == nil {
		return types.StringValue(v)
	}
	return types.StringNull()
}

func int64Value(n nullable.Nullable[int]) types.Int64 {
	if v, err := n.Get(); err == nil {
		return types.Int64Value(int64(v))
	}
	return types.Int64Null()
}

func boolValue(n nullable.Nullable[bool]) types.Bool {
	if v, err := n.Get(); err == nil {
		return types.BoolValue(v)
	}
	return types.BoolNull()
}
