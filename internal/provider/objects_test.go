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

type filterAPI struct {
	Event string
	Query string
}

var filterTypes = map[string]attr.Type{"event": types.StringType, "query": types.StringType}

func filterObj(e, q string) attr.Value {
	return types.ObjectValueMust(filterTypes, map[string]attr.Value{"event": types.StringValue(e), "query": types.StringValue(q)})
}

func filterTo(o types.Object, at path.Path, diags *diag.Diagnostics) *filterAPI {
	var m struct {
		Event types.String `tfsdk:"event"`
		Query types.String `tfsdk:"query"`
	}
	if !objectAs(o, &m, at, diags) {
		return nil
	}
	return &filterAPI{Event: m.Event.ValueString(), Query: m.Query.ValueString()}
}

func filterFrom(c *filterAPI) types.Object {
	return types.ObjectValueMust(filterTypes, map[string]attr.Value{"event": types.StringValue(c.Event), "query": types.StringValue(c.Query)})
}

func TestObjectLists(t *testing.T) {
	l := types.ListValueMust(types.ObjectType{AttrTypes: filterTypes}, []attr.Value{filterObj("all", "x")})
	var diags diag.Diagnostics
	var dst nullable.Nullable[[]filterAPI]
	createNullableObjects(&dst, l, filterTo, path.Root("filters"), &diags)
	got, _ := dst.Get()
	if diags.HasError() || len(got) != 1 || got[0].Query != "x" {
		t.Fatalf("create = %v, %v", got, diags)
	}
	var upd nullable.Nullable[[]filterAPI]
	empty := types.ListValueMust(types.ObjectType{AttrTypes: filterTypes}, []attr.Value{})
	updateNullableObjects(&upd, empty, l, filterTo, path.Root("filters"), &diags)
	if v, err := upd.Get(); err != nil || len(v) != 0 {
		t.Errorf("removed to []: %v, %v", v, err)
	}
	if r := objectList(got, filterFrom, filterTypes); !r.Equal(l) {
		t.Errorf("read = %v", r)
	}
}
