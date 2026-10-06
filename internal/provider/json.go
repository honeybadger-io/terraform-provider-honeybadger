package provider

import (
	"encoding/json"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/honeybadger-io/terraform-provider-honeybadger/internal/provider/customtypes"
)

// JSON document attributes (json_subset, phase 6 spec 3.2) are decoded into
// the API's own field type, whatever its shape, and encoded back on read.

// decodeJSON decodes a planned JSON value strictly: a key the API type
// doesn't have is an error naming it, so a misspelling never goes unsent.
func decodeJSON[T any](dst *T, v types.String, at path.Path, diags *diag.Diagnostics) {
	if v.IsNull() || v.IsUnknown() {
		return
	}
	dec := json.NewDecoder(strings.NewReader(v.ValueString()))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		diags.AddAttributeError(at, "Invalid JSON", err.Error())
	}
}

// updateJSON sends the value only when it changed: state holds the user's
// JSON as written, so equal strings mean nothing changed.
func updateJSON[T any](dst *T, plan, state types.String, at path.Path, diags *diag.Diagnostics) {
	if plan.IsUnknown() || plan.Equal(state) {
		return
	}
	decodeJSON(dst, plan, at, diags)
}

// jsonSubsetOf encodes what the API returned. The API types always encode,
// so an error (impossible for them) reads as null.
func jsonSubsetOf(v any) customtypes.JSONSubsetValue {
	b, err := json.Marshal(v)
	if err != nil {
		return customtypes.NewJSONSubsetNull()
	}
	return customtypes.NewJSONSubsetValue(string(b))
}
