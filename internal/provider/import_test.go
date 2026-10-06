package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

var importSchema = schema.Schema{Attributes: map[string]schema.Attribute{
	"id":         schema.StringAttribute{Computed: true},
	"project_id": schema.StringAttribute{Required: true},
}}

func runImport(t *testing.T, format, id string) *resource.ImportStateResponse {
	t.Helper()
	ctx := context.Background()
	resp := &resource.ImportStateResponse{State: tfsdk.State{
		Schema: importSchema,
		Raw:    tftypes.NewValue(importSchema.Type().TerraformType(ctx), nil),
	}}
	importByFormat(ctx, format, resource.ImportStateRequest{ID: id}, resp)
	return resp
}

func TestImportByFormatSetsEachPart(t *testing.T) {
	resp := runImport(t, "<project_id>/<id>", "p1/c1")
	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	for attr, want := range map[string]string{"project_id": "p1", "id": "c1"} {
		var got types.String
		resp.State.GetAttribute(context.Background(), path.Root(attr), &got)
		if got.ValueString() != want {
			t.Errorf("%s = %v, want %s", attr, got, want)
		}
	}
}

// Review Focus 3: a malformed ID gets a diagnostic naming the format.
func TestImportByFormatRejectsMalformedIDs(t *testing.T) {
	for _, id := range []string{"p1", "p1/c1/x", "/c1", "p1/"} {
		resp := runImport(t, "<project_id>/<id>", id)
		if !resp.Diagnostics.HasError() || !strings.Contains(resp.Diagnostics[0].Detail(), "<project_id>/<id>") {
			t.Errorf("import %q: diagnostics = %v, want an error naming the format", id, resp.Diagnostics)
		}
	}
}
