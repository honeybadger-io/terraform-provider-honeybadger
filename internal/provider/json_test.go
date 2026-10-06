package provider

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type testGizmo struct {
	Id   *string `json:"id,omitempty"`
	Kind string  `json:"kind"`
}

func TestDecodeJSON(t *testing.T) {
	var dst *[]testGizmo
	var diags diag.Diagnostics
	decodeJSON(&dst, types.StringValue(`[{"id":"a","kind":"k"}]`), path.Root("gizmos"), &diags)
	if diags.HasError() || dst == nil || len(*dst) != 1 || *(*dst)[0].Id != "a" {
		t.Errorf("decoded = %v, %v", dst, diags)
	}
	var typo *[]testGizmo
	decodeJSON(&typo, types.StringValue(`[{"id":"a","knd":"k"}]`), path.Root("gizmos"), &diags)
	if !diags.HasError() || !strings.Contains(diags.Errors()[0].Detail(), `"knd"`) {
		t.Errorf("a misspelt key must be an error naming it: %v", diags)
	}
}

func TestUpdateJSON(t *testing.T) {
	var dst *[]testGizmo
	var diags diag.Diagnostics
	same := types.StringValue(`[{"id":"a","kind":"k"}]`)
	updateJSON(&dst, same, same, path.Root("gizmos"), &diags)
	if dst != nil {
		t.Error("an unchanged value must not be sent")
	}
	updateJSON(&dst, types.StringValue(`[]`), same, path.Root("gizmos"), &diags)
	if dst == nil || len(*dst) != 0 {
		t.Errorf("a change to [] must be sent as an empty list, got %v", dst)
	}
}

func TestJSONSubsetOf(t *testing.T) {
	id := "a"
	if got := jsonSubsetOf([]testGizmo{{Id: &id, Kind: "k"}}); got.ValueString() != `[{"id":"a","kind":"k"}]` {
		t.Errorf("encoded = %s", got.ValueString())
	}
}
