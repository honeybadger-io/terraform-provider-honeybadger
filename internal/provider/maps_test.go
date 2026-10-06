package provider

import (
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
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
