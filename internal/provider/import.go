package provider

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

var importPlaceholder = regexp.MustCompile(`<([^>]+)>`)

// importByFormat sets the attributes an import ID names, following a format
// such as "<project_id>/<id>".
func importByFormat(ctx context.Context, format string, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	names := importPlaceholder.FindAllStringSubmatch(format, -1)
	parts := strings.Split(req.ID, "/")
	if len(parts) != len(names) || slices.Contains(parts, "") {
		resp.Diagnostics.AddError("Invalid import ID",
			fmt.Sprintf("Expected an ID like %s, got %q.", format, req.ID))
		return
	}
	for i, n := range names {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(n[1]), parts[i])...)
	}
}
