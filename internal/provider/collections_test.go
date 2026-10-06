package provider

import (
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/oapi-codegen/nullable"
)

func strSet(vals ...string) types.Set {
	elems := make([]attr.Value, len(vals))
	for i, v := range vals {
		elems[i] = types.StringValue(v)
	}
	return types.SetValueMust(types.StringType, elems)
}

func TestSlicePtrHelpers(t *testing.T) {
	var p *[]string
	setSlicePtr(&p, types.SetNull(types.StringType), fromString)
	if p != nil {
		t.Errorf("null: p = %v, want nil", *p)
	}
	setSlicePtr(&p, strSet("s1"), fromString)
	if p == nil || !reflect.DeepEqual(*p, []string{"s1"}) {
		t.Errorf("value: p = %v", p)
	}

	var u *[]string
	updateSlicePtr(&u, strSet("a"), strSet("a"), fromString)
	if u != nil {
		t.Error("unchanged: want nil")
	}
	updateSlicePtr(&u, strSet("a", "b"), strSet("a"), fromString)
	if u == nil || len(*u) != 2 {
		t.Errorf("changed: u = %v", u)
	}
}

func TestNullableSliceHelpers(t *testing.T) {
	var c nullable.Nullable[[]string]
	createNullableSlice(&c, types.SetUnknown(types.StringType), fromString)
	if c.IsSpecified() {
		t.Error("unknown: want unset")
	}

	tests := []struct {
		name        string
		plan, state types.Set
		wantSet     bool
		wantNull    bool
	}{
		{"unchanged", strSet("a"), strSet("a"), false, false},
		{"unknown", types.SetUnknown(types.StringType), strSet("a"), false, false},
		{"removed", types.SetNull(types.StringType), strSet("a"), true, true},
		{"changed", strSet("b"), strSet("a"), true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var u nullable.Nullable[[]string]
			updateNullableSlice(&u, tt.plan, tt.state, fromString)
			if u.IsSpecified() != tt.wantSet || u.IsNull() != tt.wantNull {
				t.Errorf("specified = %v, null = %v; want %v, %v", u.IsSpecified(), u.IsNull(), tt.wantSet, tt.wantNull)
			}
		})
	}
}

func TestCollectionReads(t *testing.T) {
	if s := setOf([]string(nil), types.StringType, stringElem); !s.IsNull() {
		t.Error("nil slice: want a null set")
	}
	if s := setOf(nullableSlice(nullable.NewNullableWithValue([]string{"a"})), types.StringType, stringElem); !s.Equal(strSet("a")) {
		t.Errorf("nullable value: got %v", s)
	}
	if s := setOf(nullableSlice(nullable.NewNullNullable[[]string]()), types.StringType, stringElem); !s.IsNull() {
		t.Error("nullable null: want a null set")
	}
	if l := listOf(derefSlice(&[]string{"a"}), types.StringType, stringElem); len(l.Elements()) != 1 {
		t.Errorf("list: got %v", l)
	}
}

func TestUUIDElements(t *testing.T) {
	var p *[]uuid.UUID
	setSlicePtr(&p, strSet("0b6a3c1e-7b4f-4a8e-9f00-1c2d3e4f5a6b"), fromUUID)
	if p == nil || (*p)[0].String() != "0b6a3c1e-7b4f-4a8e-9f00-1c2d3e4f5a6b" {
		t.Errorf("p = %v", p)
	}
	if s := setOf(*p, types.StringType, uuidElem); !s.Equal(strSet("0b6a3c1e-7b4f-4a8e-9f00-1c2d3e4f5a6b")) {
		t.Errorf("read = %v", s)
	}
}

func TestParseUUIDID(t *testing.T) {
	var diags diag.Diagnostics
	u, ok := parseUUIDID(types.StringValue("0b6f2b8e-4f7c-4c1a-9a51-6c1f0e1b2a3d"), &diags)
	if !ok || u.String() != "0b6f2b8e-4f7c-4c1a-9a51-6c1f0e1b2a3d" || diags.HasError() {
		t.Errorf("valid ID = %v, %v, %v", u, ok, diags)
	}
	_, ok = parseUUIDID(types.StringValue("not-a-uuid"), &diags)
	if ok || !diags.HasError() || !strings.Contains(diags.Errors()[0].Detail(), "isn't a UUID") {
		t.Errorf("malformed ID = %v, %v; want an error saying it isn't a UUID", ok, diags)
	}
}
