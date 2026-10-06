package provider

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/oapi-codegen/nullable"
)

// Conversions between Terraform lists and sets and apiv3 slices. Generated
// mapping code calls these (phase 4a spec, 3.2). The element conversions are
// generic, so the slice's element type is inferred from the destination.

// collection is a types.List or a types.Set.
type collection interface {
	attr.Value
	Elements() []attr.Value
}

func fromString[T ~string](v attr.Value) T { return T(v.(types.String).ValueString()) }

func stringElem[T ~string](v T) attr.Value { return types.StringValue(string(v)) }

func elements[T any](c collection, conv func(attr.Value) T) []T {
	out := make([]T, 0, len(c.Elements()))
	for _, e := range c.Elements() {
		out = append(out, conv(e))
	}
	return out
}

// setSlicePtr leaves a missing collection out, so the server fills it in.
func setSlicePtr[T any](dst **[]T, c collection, conv func(attr.Value) T) {
	if c.IsNull() || c.IsUnknown() {
		return
	}
	s := elements(c, conv)
	*dst = &s
}

// updateSlicePtr sends only a change. A pointer can't carry null, so specgen
// uses it only for attributes the user can't remove.
func updateSlicePtr[T any](dst **[]T, plan, state collection, conv func(attr.Value) T) {
	if plan.IsNull() || plan.IsUnknown() || plan.Equal(state) {
		return
	}
	s := elements(plan, conv)
	*dst = &s
}

func createNullableSlice[T any](dst *nullable.Nullable[[]T], c collection, conv func(attr.Value) T) {
	if c.IsNull() || c.IsUnknown() {
		return
	}
	*dst = nullable.NewNullableWithValue(elements(c, conv))
}

// updateNullableSlice sends only a change; a collection removed from config
// is sent as null.
func updateNullableSlice[T any](dst *nullable.Nullable[[]T], plan, state collection, conv func(attr.Value) T) {
	switch {
	case plan.IsUnknown() || plan.Equal(state):
	case plan.IsNull():
		*dst = nullable.NewNullNullable[[]T]()
	default:
		*dst = nullable.NewNullableWithValue(elements(plan, conv))
	}
}

// setOf and listOf read a slice; nil reads as null.
func setOf[T any](s []T, elemType attr.Type, conv func(T) attr.Value) types.Set {
	if s == nil {
		return types.SetNull(elemType)
	}
	return types.SetValueMust(elemType, values(s, conv))
}

func listOf[T any](s []T, elemType attr.Type, conv func(T) attr.Value) types.List {
	if s == nil {
		return types.ListNull(elemType)
	}
	return types.ListValueMust(elemType, values(s, conv))
}

func values[T any](s []T, conv func(T) attr.Value) []attr.Value {
	out := make([]attr.Value, len(s))
	for i, v := range s {
		out[i] = conv(v)
	}
	return out
}

func derefSlice[T any](p *[]T) []T {
	if p == nil {
		return nil
	}
	return *p
}

// nullableSlice is nil when the field is unset or null.
func nullableSlice[T any](n nullable.Nullable[[]T]) []T {
	v, err := n.Get()
	if err != nil {
		return nil
	}
	return v
}

// fromUUID and uuidElem convert format: uuid elements; api-go's
// openapi_types.UUID is an alias of uuid.UUID.
func fromUUID(v attr.Value) uuid.UUID {
	u, _ := uuid.Parse(v.(types.String).ValueString())
	return u
}

func uuidElem(u uuid.UUID) attr.Value { return types.StringValue(u.String()) }

// parseUUIDID reads a UUID resource ID from state. A malformed one, such as
// a mistyped import ID, is an error rather than a request for the zero UUID.
func parseUUIDID(v types.String, diags *diag.Diagnostics) (uuid.UUID, bool) {
	u, err := uuid.Parse(v.ValueString())
	if err != nil {
		diags.AddError("Invalid ID", fmt.Sprintf("%q isn't a UUID: %v", v.ValueString(), err))
		return uuid.UUID{}, false
	}
	return u, true
}
