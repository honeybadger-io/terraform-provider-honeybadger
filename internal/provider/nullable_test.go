package provider

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/oapi-codegen/nullable"
)

// wire mirrors how apiv3's request types declare nullable fields.
type wire struct {
	S nullable.Nullable[string] `json:"s,omitempty"`
	I nullable.Nullable[int]    `json:"i,omitempty"`
	B nullable.Nullable[bool]   `json:"b,omitempty"`
}

func marshal(t *testing.T, w wire) string {
	t.Helper()
	b, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return string(b)
}

func TestCreateSendsOnlyKnownValues(t *testing.T) {
	tests := []struct {
		name string
		w    wire
		want string
	}{
		{"known values are sent",
			wire{S: createString(types.StringValue("x")), I: createInt(types.Int64Value(7)), B: createBool(types.BoolValue(false))},
			`{"s":"x","i":7,"b":false}`},
		{"null values are left out",
			wire{S: createString(types.StringNull()), I: createInt(types.Int64Null()), B: createBool(types.BoolNull())},
			`{}`},
		{"unknown values are left out",
			wire{S: createString(types.StringUnknown()), I: createInt(types.Int64Unknown()), B: createBool(types.BoolUnknown())},
			`{}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := marshal(t, tt.w); got != tt.want {
				t.Errorf("body = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestUpdateSendsChangesAndClears(t *testing.T) {
	tests := []struct {
		name  string
		plan  types.String
		state types.String
		want  string
	}{
		{"changed value is sent", types.StringValue("new"), types.StringValue("old"), `{"s":"new"}`},
		{"new value is sent", types.StringValue("new"), types.StringNull(), `{"s":"new"}`},
		{"removed value is cleared with null", types.StringNull(), types.StringValue("old"), `{"s":null}`},
		{"unchanged value is left out", types.StringValue("same"), types.StringValue("same"), `{}`},
		{"still-null value is left out", types.StringNull(), types.StringNull(), `{}`},
		{"unknown value is left out", types.StringUnknown(), types.StringValue("old"), `{}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := marshal(t, wire{S: updateString(tt.plan, tt.state)}); got != tt.want {
				t.Errorf("body = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestUpdateIntAndBoolClearLikeStrings(t *testing.T) {
	got := marshal(t, wire{
		I: updateInt(types.Int64Null(), types.Int64Value(30)),
		B: updateBool(types.BoolValue(true), types.BoolValue(false)),
	})
	if want := `{"i":null,"b":true}`; got != want {
		t.Errorf("body = %s, want %s", got, want)
	}

	got = marshal(t, wire{
		I: updateInt(types.Int64Value(30), types.Int64Value(30)),
		B: updateBool(types.BoolNull(), types.BoolNull()),
	})
	if want := `{}`; got != want {
		t.Errorf("body = %s, want %s", got, want)
	}
}

func TestReadMapsNullAndUnsetToTerraformNull(t *testing.T) {
	var w wire
	if err := json.Unmarshal([]byte(`{"s":"x","i":null}`), &w); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got := stringValue(w.S); !got.Equal(types.StringValue("x")) {
		t.Errorf("stringValue = %v, want \"x\"", got)
	}
	if got := int64Value(w.I); !got.IsNull() {
		t.Errorf("int64Value of null = %v, want null", got)
	}
	if got := boolValue(w.B); !got.IsNull() {
		t.Errorf("boolValue of an absent field = %v, want null", got)
	}
	if got := int64Value(nullable.NewNullableWithValue(42)); !got.Equal(types.Int64Value(42)) {
		t.Errorf("int64Value = %v, want 42", got)
	}
	if got := boolValue(nullable.NewNullableWithValue(false)); !got.Equal(types.BoolValue(false)) {
		t.Errorf("boolValue = %v, want false", got)
	}
}

type testSpeed int

func TestIntEnumHelpers(t *testing.T) {
	var c nullable.Nullable[testSpeed]
	createIntEnum(&c, types.Int64Value(15))
	if v, err := c.Get(); err != nil || v != 15 {
		t.Errorf("create = %v, %v", v, err)
	}
	var unset, same, removed nullable.Nullable[testSpeed]
	createIntEnum(&unset, types.Int64Unknown())
	updateIntEnum(&same, types.Int64Value(5), types.Int64Value(5))
	updateIntEnum(&removed, types.Int64Null(), types.Int64Value(5))
	if unset.IsSpecified() || same.IsSpecified() || !removed.IsNull() {
		t.Errorf("unknown specified=%v, unchanged specified=%v, removed null=%v", unset.IsSpecified(), same.IsSpecified(), removed.IsNull())
	}
	if got := intEnumValue(nullable.NewNullableWithValue(testSpeed(2))); got.ValueInt64() != 2 {
		t.Errorf("read = %v", got)
	}
	if got := intEnumValue(nullable.NewNullNullable[testSpeed]()); !got.IsNull() {
		t.Errorf("read null = %v", got)
	}
}

type testMode string

func TestEnumHelpers(t *testing.T) {
	var c, unset, same, removed nullable.Nullable[testMode]
	createEnum(&c, types.StringValue("fast"))
	createEnum(&unset, types.StringNull())
	updateEnum(&same, types.StringValue("slow"), types.StringValue("slow"))
	updateEnum(&removed, types.StringNull(), types.StringValue("fast"))
	if v, err := c.Get(); err != nil || v != "fast" {
		t.Errorf("create = %v, %v", v, err)
	}
	if unset.IsSpecified() || same.IsSpecified() || !removed.IsNull() {
		t.Errorf("null specified=%v, unchanged specified=%v, removed null=%v", unset.IsSpecified(), same.IsSpecified(), removed.IsNull())
	}
	if got := enumValue(nullable.NewNullableWithValue(testMode("slow"))); got.ValueString() != "slow" {
		t.Errorf("read = %v", got)
	}
}
