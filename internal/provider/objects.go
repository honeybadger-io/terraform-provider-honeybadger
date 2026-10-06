package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/oapi-codegen/nullable"
)

// Nested objects are sent whole (phase 4a spec, 3.1). Generated converters
// use these.

// objectAs reads a known object into its model. It's false for a null or
// unknown object, and for one holding an unknown value, which it reports:
// the API replaces the object whole, so no value inside it can be left out.
func objectAs(o types.Object, dst any, at path.Path, diags *diag.Diagnostics) bool {
	if o.IsNull() || o.IsUnknown() {
		return false
	}
	if !fullyKnown(o) {
		diags.AddAttributeError(at, "Value not known",
			"Every value in this object must be known when it's sent: the Honeybadger API replaces the object whole.")
		return false
	}
	d := o.As(context.Background(), dst, basetypes.ObjectAsOptions{})
	diags.Append(d...)
	return !d.HasError()
}

// fullyKnown reports whether v and every value inside it are known.
func fullyKnown(v attr.Value) bool {
	if v.IsUnknown() {
		return false
	}
	var inner []attr.Value
	switch v := v.(type) {
	case basetypes.ObjectValue:
		for _, a := range v.Attributes() {
			inner = append(inner, a)
		}
	case basetypes.ListValue:
		inner = v.Elements()
	case basetypes.SetValue:
		inner = v.Elements()
	}
	for _, i := range inner {
		if !fullyKnown(i) {
			return false
		}
	}
	return true
}

// orZero is a value-shaped field's value: the zero value when the converter
// reported a problem, which stops the request.
func orZero[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

// nullableOf leaves a missing value out.
func nullableOf[T any](p *T) nullable.Nullable[T] {
	if p == nil {
		return nullable.Nullable[T]{}
	}
	return nullable.NewNullableWithValue(*p)
}

// changedPtr sends v only when the object changed. A pointer can't carry
// null, so specgen uses it only for objects the user can't remove.
func changedPtr[T any](plan, state attr.Value, v *T) *T {
	if plan.IsUnknown() || plan.Equal(state) {
		return nil
	}
	return v
}

// updateNullableObject sends only a change; a removed object is sent as null.
func updateNullableObject[T any](plan, state types.Object, v *T) nullable.Nullable[T] {
	switch {
	case plan.IsUnknown() || plan.Equal(state):
		return nullable.Nullable[T]{}
	case plan.IsNull():
		return nullable.NewNullNullable[T]()
	}
	return nullableOf(v)
}

// nullablePtr reads a nullable field: nil when it's unset or null.
func nullablePtr[T any](n nullable.Nullable[T]) *T {
	if v, err := n.Get(); err == nil {
		return &v
	}
	return nil
}
