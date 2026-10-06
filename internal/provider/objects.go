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

// objectAsLoose reads a prior object into its model, ignoring problems: a
// prior value only decides which keys were set before.
func objectAsLoose(o types.Object, dst any) {
	if o.IsNull() || o.IsUnknown() {
		return
	}
	_ = o.As(context.Background(), dst, basetypes.ObjectAsOptions{UnhandledNullAsEmpty: true, UnhandledUnknownAsEmpty: true})
}

// priorAttr is a write-only key's value from the prior object, which the API
// never returns (phase 4b spec, 3.2).
func priorAttr(prior types.Object, key string, null attr.Value) attr.Value {
	if prior.IsNull() || prior.IsUnknown() {
		return null
	}
	if v, ok := prior.Attributes()[key]; ok {
		return v
	}
	return null
}

// createNullableObjects and updateNullableObjects write a list of objects,
// sent whole: a removed list ([] by default) is sent as it's planned.
func createNullableObjects[T any](dst *nullable.Nullable[[]T], l types.List, conv func(types.Object, path.Path, *diag.Diagnostics) *T, at path.Path, diags *diag.Diagnostics) {
	if l.IsNull() || l.IsUnknown() {
		return
	}
	*dst = nullable.NewNullableWithValue(objectsOf(l, conv, at, diags))
}

func updateNullableObjects[T any](dst *nullable.Nullable[[]T], plan, state types.List, conv func(types.Object, path.Path, *diag.Diagnostics) *T, at path.Path, diags *diag.Diagnostics) {
	switch {
	case plan.IsUnknown() || plan.Equal(state):
	case plan.IsNull():
		*dst = nullable.NewNullNullable[[]T]()
	default:
		*dst = nullable.NewNullableWithValue(objectsOf(plan, conv, at, diags))
	}
}

func objectsOf[T any](l types.List, conv func(types.Object, path.Path, *diag.Diagnostics) *T, at path.Path, diags *diag.Diagnostics) []T {
	out := make([]T, 0, len(l.Elements()))
	for i, e := range l.Elements() {
		if v := conv(e.(types.Object), at.AtListIndex(i), diags); v != nil {
			out = append(out, *v)
		}
	}
	return out
}

// objectList reads a slice of structs; nil reads as null.
func objectList[T any](s []T, conv func(*T) types.Object, attrTypes map[string]attr.Type) types.List {
	elem := types.ObjectType{AttrTypes: attrTypes}
	if s == nil {
		return types.ListNull(elem)
	}
	vals := make([]attr.Value, len(s))
	for i := range s {
		vals[i] = conv(&s[i])
	}
	return types.ListValueMust(elem, vals)
}
