package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/oapi-codegen/nullable"
)

type condModel struct {
	Operator types.String  `tfsdk:"operator"`
	Value    types.Float64 `tfsdk:"value"`
}

var condTypes = map[string]attr.Type{"operator": types.StringType, "value": types.Float64Type}

func cond(value types.Float64) types.Object {
	return types.ObjectValueMust(condTypes, map[string]attr.Value{"operator": types.StringValue("gt"), "value": value})
}

func TestObjectAs(t *testing.T) {
	var diags diag.Diagnostics
	var m condModel
	if !objectAs(cond(types.Float64Value(2.5)), &m, path.Root("c"), &diags) || m.Value.ValueFloat64() != 2.5 {
		t.Errorf("known: ok = false or value = %v (%v)", m.Value, diags)
	}
	if objectAs(types.ObjectNull(condTypes), &m, path.Root("c"), &diags) || diags.HasError() {
		t.Error("null: want false without an error")
	}
	if objectAs(cond(types.Float64Unknown()), &m, path.Root("c"), &diags) {
		t.Error("unknown child: want false")
	}
	if !diags.HasError() {
		t.Fatal("unknown child: want an error")
	}
	if d, ok := diags[0].(diag.DiagnosticWithPath); !ok || !d.Path().Equal(path.Root("c")) {
		t.Errorf("error = %v, want one at c", diags[0])
	}
}

func TestObjectWriteHelpers(t *testing.T) {
	v := 1.0
	if orZero[float64](nil) != 0 || orZero(&v) != 1 {
		t.Error("orZero")
	}
	if nullableOf[float64](nil).IsSpecified() || !nullableOf(&v).IsSpecified() {
		t.Error("nullableOf")
	}
	a, b := cond(types.Float64Value(1)), cond(types.Float64Value(2))
	if changedPtr(a, a, &v) != nil || changedPtr(b, a, &v) != &v {
		t.Error("changedPtr")
	}
	if n := updateNullableObject(types.ObjectNull(condTypes), a, &v); !n.IsNull() {
		t.Error("updateNullableObject: removed should be null")
	}
	if n := updateNullableObject(a, a, &v); n.IsSpecified() {
		t.Error("updateNullableObject: unchanged should be unset")
	}
	if nullablePtr(nullable.NewNullNullable[float64]()) != nil || *nullablePtr(nullable.NewNullableWithValue(3.0)) != 3 {
		t.Error("nullablePtr")
	}
}
