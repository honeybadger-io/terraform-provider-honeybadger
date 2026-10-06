package provider

import (
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
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
