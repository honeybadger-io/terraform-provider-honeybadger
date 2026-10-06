package provider

import (
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/oapi-codegen/nullable"
)

// The one write rule (phase 4b spec, 3.2): a value is sent, a removal sends
// null, and a key never set isn't sent.
func TestPutRule(t *testing.T) {
	out := map[string]any{}
	putString(out, "url", types.StringValue("https://a"), types.StringValue("https://a"))
	putString(out, "token", types.StringNull(), types.StringValue("s"))
	putString(out, "label", types.StringNull(), types.StringNull())
	putBool(out, "rich", types.BoolValue(true), types.BoolNull())
	putStrings(out, "ids", strSet("a"), types.SetNull(types.StringType))
	want := map[string]any{"url": "https://a", "token": nil, "rich": true, "ids": []string{"a"}}
	if !reflect.DeepEqual(out, want) {
		t.Errorf("out = %#v\nwant %#v", out, want)
	}
}

func TestMapReads(t *testing.T) {
	m := map[string]any{"url": "https://a", "rich": true, "ids": []any{"a"}, "gone": nil}
	if v := mapString(m, "url"); v.ValueString() != "https://a" {
		t.Errorf("url = %v", v)
	}
	if v := mapString(m, "gone"); !v.IsNull() {
		t.Error("null key: want null")
	}
	if v := mapString(m, "missing"); !v.IsNull() {
		t.Error("missing key: want null")
	}
	if v := mapBool(m, "rich"); !v.ValueBool() {
		t.Error("rich")
	}
	if v := mapStrings(m, "ids", true); !v.Equal(strSet("a")) {
		t.Errorf("ids = %v", v)
	}
}

func TestPriorAttrAndLooseAs(t *testing.T) {
	prior := cond(types.Float64Value(2))
	if v := priorAttr(prior, "value", types.Float64Null()); !v.Equal(types.Float64Value(2)) {
		t.Errorf("priorAttr = %v", v)
	}
	if v := priorAttr(types.ObjectNull(condTypes), "value", types.Float64Null()); !v.IsNull() {
		t.Error("null prior: want the null")
	}
	var m condModel
	objectAsLoose(cond(types.Float64Unknown()), &m)
	if m.Operator.ValueString() != "gt" {
		t.Errorf("loose As = %+v", m)
	}
}

func TestStringMapHelpers(t *testing.T) {
	m := func(kv map[string]string) types.Map {
		e := map[string]attr.Value{}
		for k, v := range kv {
			e[k] = types.StringValue(v)
		}
		return types.MapValueMust(types.StringType, e)
	}
	var c nullable.Nullable[map[string]string]
	createStringMap(&c, m(map[string]string{"A": "1"}))
	if v, err := c.Get(); err != nil || v["A"] != "1" {
		t.Errorf("create = %v, %v", v, err)
	}
	var unset nullable.Nullable[map[string]string]
	createStringMap(&unset, types.MapNull(types.StringType))
	if unset.IsSpecified() {
		t.Error("a null map must be left out on create")
	}
	var same, cleared, changed nullable.Nullable[map[string]string]
	updateStringMap(&same, m(map[string]string{"A": "1"}), m(map[string]string{"A": "1"}))
	updateStringMap(&cleared, types.MapNull(types.StringType), m(map[string]string{"A": "1"}))
	updateStringMap(&changed, m(map[string]string{}), m(map[string]string{"A": "1"}))
	if same.IsSpecified() || !cleared.IsNull() {
		t.Errorf("update: unchanged specified=%v, removed null=%v", same.IsSpecified(), cleared.IsNull())
	}
	if v, err := changed.Get(); err != nil || len(v) != 0 {
		t.Errorf("update to {} = %v, %v; want an empty map sent", v, err)
	}
	if got := stringMapValue(nullable.NewNullNullable[map[string]string]()); !got.IsNull() {
		t.Errorf("read null = %v", got)
	}
	if got := stringMapValue(nullable.NewNullableWithValue(map[string]string{})); got.IsNull() || len(got.Elements()) != 0 {
		t.Errorf("read {} = %v, want an empty, non-null map", got)
	}
}
