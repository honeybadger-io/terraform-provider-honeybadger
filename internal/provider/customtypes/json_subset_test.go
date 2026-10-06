package customtypes

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func decode(t *testing.T, s string) any {
	t.Helper()
	v, err := parseJSON(s)
	if err != nil {
		t.Fatalf("%s: %v", s, err)
	}
	return v
}

// Each case is want (what the user wrote) against got (what the API returned).
func TestSubset(t *testing.T) {
	for _, c := range []struct {
		name, want, got string
		equal           bool
	}{
		{"server-only keys", `[{"id":"a","type":"alarms"}]`, `[{"id":"a","type":"alarms","grid":{"x":0,"y":0,"w":12,"h":3}}]`, true},
		{"partial grid", `[{"id":"a","grid":{"x":2}}]`, `[{"id":"a","grid":{"x":2,"y":0,"w":12,"h":3}}]`, true},
		{"not set: null", `{"a":null}`, `{"a":1}`, true},
		{"not set: empty string", `{"a":""}`, `{}`, true},
		{"not set: []", `{"streams":[]}`, `{"streams":["default","internal"]}`, true},
		{"not set: {}", `{"a":{}}`, `{"a":{"b":1}}`, true},
		{"not set: object of empties", `{"a":{"b":null,"c":[]}}`, `{}`, true},
		{"false is a value", `{"a":false}`, `{}`, false},
		{"zero is a value", `{"a":0}`, `{}`, false},
		{"numbers compare numerically", `{"h":3}`, `{"h":3.0}`, true},
		{"string vs number", `{"h":"3"}`, `{"h":3}`, false},
		{"changed value", `{"title":"A"}`, `{"title":"B"}`, false},
		{"objects by position", `[{"id":"a"},{"id":"b"}]`, `[{"id":"b"},{"id":"a"}]`, false},
		{"missing widget", `[{"id":"a"},{"id":"b"}]`, `[{"id":"a"}]`, false},
		{"extra widget", `[{"id":"a"}]`, `[{"id":"a"},{"id":"b"}]`, false},
		{"scalars ignore order", `["internal","default"]`, `["default","internal"]`, true},
		{"scalars count duplicates", `["a","a"]`, `["a"]`, false},
		{"scalar dropped", `["default","bogus"]`, `["default"]`, false},
		{"key the user set is missing", `{"a":1}`, `{}`, false},
		{"large integers stay exact", `{"n":9007199254740993}`, `{"n":9007199254740992}`, false},
	} {
		if got := Subset(decode(t, c.want), decode(t, c.got)); got != c.equal {
			t.Errorf("%s: Subset = %v, want %v", c.name, got, c.equal)
		}
	}
}

// The asymmetric cases hold one way only (phase 6 spec, 5.1).
func TestSubsetIsDirectional(t *testing.T) {
	for _, c := range [][2]string{
		{`[{"id":"a"}]`, `[{"id":"a","grid":{"x":0}}]`},
		{`{"streams":[]}`, `{"streams":["default"]}`},
		{`{"a":{"b":null}}`, `{"a":{"b":1}}`},
	} {
		if !Subset(decode(t, c[0]), decode(t, c[1])) || Subset(decode(t, c[1]), decode(t, c[0])) {
			t.Errorf("%s within %s must hold one way only", c[0], c[1])
		}
	}
}

// The framework calls new.StringSemanticEquals(prior): the receiver is the
// read value, the argument what the user wrote. Swapped, it must differ.
func TestSemanticEqualsDirection(t *testing.T) {
	wrote := NewJSONSubsetValue(`[{"id":"a"}]`)
	read := NewJSONSubsetValue(`[{"id":"a","grid":{"x":0,"y":0,"w":12,"h":3}}]`)
	if eq, _ := read.StringSemanticEquals(context.Background(), wrote); !eq {
		t.Error("read.StringSemanticEquals(wrote) = false, want true")
	}
	if eq, _ := wrote.StringSemanticEquals(context.Background(), read); eq {
		t.Error("wrote.StringSemanticEquals(read) = true, want false: the direction matters")
	}
	if eq, _ := read.StringSemanticEquals(context.Background(), NewJSONSubsetValue(`not json`)); eq {
		t.Error("invalid JSON compared equal")
	}
}

func TestJSONValidator(t *testing.T) {
	v := JSONValidator{Type: "array", Required: []string{"id"}}
	run := func(s types.String) []string {
		resp := &validator.StringResponse{}
		v.ValidateString(context.Background(), validator.StringRequest{Path: path.Root("widgets"), ConfigValue: s}, resp)
		var out []string
		for _, d := range resp.Diagnostics.Errors() {
			out = append(out, d.Detail())
		}
		return out
	}
	for _, c := range []struct {
		name string
		in   types.String
		want string // substring of the one error; "" for none
	}{
		{"valid", types.StringValue(`[{"id":"a"},{"id":"b"}]`), ""},
		{"empty list", types.StringValue(`[]`), ""},
		{"unknown skipped", types.StringUnknown(), ""},
		{"null skipped", types.StringNull(), ""},
		{"invalid JSON", types.StringValue(`[{`), "isn't valid JSON"},
		{"object, not array", types.StringValue(`{"id":"a"}`), "must be a JSON array"},
		{"null JSON", types.StringValue(`null`), "must be a JSON array"},
		{"missing id", types.StringValue(`[{"id":"a"},{"type":"alarms"}]`), `widgets[1]: "id" is required`},
		{"empty id", types.StringValue(`[{"id":""}]`), `widgets[0]: "id" is required`},
		{"element not an object", types.StringValue(`["a"]`), "widgets[0] must be a JSON object"},
	} {
		errs := run(c.in)
		switch {
		case c.want == "" && len(errs) > 0:
			t.Errorf("%s: unexpected errors %q", c.name, errs)
		case c.want != "" && (len(errs) != 1 || !strings.Contains(errs[0], c.want)):
			t.Errorf("%s: errors %q, want one containing %q", c.name, errs, c.want)
		}
	}
}
