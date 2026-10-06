// Package specgen generates each resource's Terraform schema and model from
// the OpenAPI bundle (spec section 3.3). This file reads what the bundle says
// about each schema and field.
package specgen

import (
	"fmt"

	"github.com/getkin/kin-openapi/openapi3"
)

// Load reads an OpenAPI 3.1 bundle with every $ref resolved.
func Load(path string) (*openapi3.T, error) {
	doc, err := openapi3.NewLoader().LoadFromFile(path)
	if err != nil {
		return nil, fmt.Errorf("loading %s: %w", path, err)
	}
	return doc, nil
}

// Field is what the spec says about one property.
type Field struct {
	Name             string
	Type             string // string, integer, number, boolean, array or object
	Format           string
	Description      string
	Pattern          string
	Nullable         bool
	ReadOnly         bool
	WriteOnly        bool
	UniqueItems      bool
	Secret           bool // format: password
	Enum             []string
	Default          any // nil when the spec gives none
	MinLength        int64
	MaxLength        *int64
	Minimum          *float64
	Maximum          *float64
	ExclusiveMinimum *float64 // read so Merge can refuse it; specgen doesn't render it
	ExclusiveMaximum *float64
	MinItems         int64 // 0 when the spec gives none
	MaxItems         *int64
	Items            *Field
	Properties       *Object // an object's own properties
	Values           *Field  // an object's additionalProperties schema
	Const            any     // OpenAPI 3.1 const
	Conditional      bool    // has if/then allOf parts specgen doesn't model
}

// Object is a schema's properties, with allOf flattened.
type Object struct {
	Name        string
	Description string
	Required    map[string]bool
	Fields      map[string]Field
}

// ObjectFacts flattens an object schema's allOf and reads each property.
func ObjectFacts(name string, ref *openapi3.SchemaRef) (Object, error) {
	if ref == nil || ref.Value == nil {
		return Object{}, fmt.Errorf("schema %s not found", name)
	}
	obj := Object{Name: name, Description: ref.Value.Description, Required: map[string]bool{}, Fields: map[string]Field{}}
	s := ref.Value
	for _, part := range s.AllOf {
		sub, err := ObjectFacts(name, part)
		if err != nil {
			return Object{}, err
		}
		if obj.Description == "" {
			obj.Description = sub.Description
		}
		for k, v := range sub.Fields {
			obj.Fields[k] = v
		}
		for k := range sub.Required {
			obj.Required[k] = true
		}
	}
	for _, r := range s.Required {
		obj.Required[r] = true
	}
	for prop, pref := range s.Properties {
		f, err := FieldFacts(prop, pref)
		if err != nil {
			return Object{}, fmt.Errorf("%s.%s: %w", name, prop, err)
		}
		obj.Fields[prop] = f
	}
	return obj, nil
}

// FieldFacts reads one property. A property written as allOf: [$ref] takes the
// referenced schema's facts, with the property's own description, default and
// flags winning. A oneOf or anyOf of one schema and null is that schema, nullable.
func FieldFacts(name string, ref *openapi3.SchemaRef) (Field, error) {
	if ref == nil || ref.Value == nil {
		return Field{}, fmt.Errorf("no schema")
	}
	s := ref.Value

	// if/then parts are conditional constraints (per-type shapes): recorded,
	// so setKind can refuse to type them, and otherwise left to the API.
	var structural openapi3.SchemaRefs
	conditional := false
	for _, p := range s.AllOf {
		if p.Value != nil && (p.Value.If != nil || p.Value.Then != nil) {
			conditional = true
			continue
		}
		structural = append(structural, p)
	}

	var f Field
	switch {
	case len(structural) == 1:
		base, err := FieldFacts(name, structural[0])
		if err != nil {
			return Field{}, err
		}
		f = base
	case len(s.OneOf) > 0 || len(s.AnyOf) > 0:
		branches := s.OneOf
		if len(branches) == 0 {
			branches = s.AnyOf
		}
		inner, ok := nullableBranch(branches)
		if !ok {
			return Field{}, fmt.Errorf("unsupported union: only one schema plus null is supported")
		}
		base, err := FieldFacts(name, inner)
		if err != nil {
			return Field{}, err
		}
		f = base
		f.Nullable = true
	case len(structural) > 1:
		return Field{}, fmt.Errorf("unsupported allOf with %d parts on a property", len(structural))
	}
	f.Conditional = f.Conditional || conditional

	f.Name = name
	if s.Type != nil {
		for _, t := range s.Type.Slice() {
			if t == "null" {
				f.Nullable = true
			} else {
				f.Type = t
			}
		}
	}
	if s.Format != "" {
		f.Format = s.Format
	}
	if s.Description != "" {
		f.Description = s.Description
	}
	if s.Pattern != "" {
		f.Pattern = s.Pattern
	}
	if s.Default != nil {
		f.Default = s.Default
	}
	if s.Const != nil {
		f.Const = s.Const
	}
	if len(s.Enum) > 0 {
		f.Enum = nil
		for _, e := range s.Enum {
			if e != nil {
				f.Enum = append(f.Enum, fmt.Sprint(e))
			}
		}
	}
	f.ReadOnly = f.ReadOnly || s.ReadOnly
	f.WriteOnly = f.WriteOnly || s.WriteOnly
	f.UniqueItems = f.UniqueItems || s.UniqueItems
	f.Secret = f.Format == "password"
	if s.MinLength > 0 {
		f.MinLength = int64(s.MinLength)
	}
	if s.MaxLength != nil {
		n := int64(*s.MaxLength)
		f.MaxLength = &n
	}
	if s.Min != nil {
		f.Minimum = s.Min
	}
	if s.Max != nil {
		f.Maximum = s.Max
	}
	if s.ExclusiveMin.Value != nil {
		f.ExclusiveMinimum = s.ExclusiveMin.Value
	}
	if s.ExclusiveMax.Value != nil {
		f.ExclusiveMaximum = s.ExclusiveMax.Value
	}
	// OpenAPI 3.0's boolean form makes minimum or maximum exclusive.
	if b := s.ExclusiveMin.Bool; b != nil && *b && f.Minimum != nil {
		f.ExclusiveMinimum, f.Minimum = f.Minimum, nil
	}
	if b := s.ExclusiveMax.Bool; b != nil && *b && f.Maximum != nil {
		f.ExclusiveMaximum, f.Maximum = f.Maximum, nil
	}
	if s.MinItems > 0 {
		f.MinItems = int64(s.MinItems)
	}
	if s.MaxItems != nil {
		n := int64(*s.MaxItems)
		f.MaxItems = &n
	}
	if s.Items != nil {
		item, err := FieldFacts(name, s.Items)
		if err != nil {
			return Field{}, fmt.Errorf("items: %w", err)
		}
		f.Items = &item
	}
	if ap := s.AdditionalProperties.Schema; ap != nil {
		v, err := FieldFacts(name, ap)
		if err != nil {
			return Field{}, fmt.Errorf("additionalProperties: %w", err)
		}
		f.Values = &v
	}
	if f.Type == "object" && len(s.Properties) > 0 {
		obj, err := ObjectFacts(name, &openapi3.SchemaRef{Value: s})
		if err != nil {
			return Field{}, err
		}
		f.Properties = &obj
	}
	return f, nil
}

// nullableBranch returns the non-null schema of a two-branch union with null.
func nullableBranch(branches openapi3.SchemaRefs) (*openapi3.SchemaRef, bool) {
	if len(branches) != 2 {
		return nil, false
	}
	isNull := func(r *openapi3.SchemaRef) bool {
		return r.Value != nil && r.Value.Type != nil && r.Value.Type.Is("null")
	}
	switch {
	case isNull(branches[0]) && !isNull(branches[1]):
		return branches[1], true
	case isNull(branches[1]) && !isNull(branches[0]):
		return branches[0], true
	}
	return nil, false
}

// Variant is one branch of a union: the discriminator's value and the
// branch's schema, which narrows the shared input.
type Variant struct {
	Const string
	Then  Object
}

// Variants reads a schema's allOf branches of the form
// if {properties: {<field>: {const: X}}} then {…}, in spec order. A branch
// whose if isn't a const on field is an error.
func Variants(input *openapi3.SchemaRef, field string) ([]Variant, error) {
	if input == nil || input.Value == nil {
		return nil, fmt.Errorf("no schema")
	}
	var out []Variant
	seen := map[string]bool{}
	for i, part := range input.Value.AllOf {
		p := part.Value
		if p == nil || (p.If == nil && p.Then == nil) {
			continue
		}
		if p.If == nil || p.Then == nil {
			return nil, fmt.Errorf("allOf[%d]: a branch needs both if and then", i)
		}
		disc := p.If.Value.Properties[field]
		if disc == nil || disc.Value == nil || disc.Value.Const == nil {
			return nil, fmt.Errorf("allOf[%d]: its if doesn't fix %s with a const", i, field)
		}
		c, ok := disc.Value.Const.(string)
		if !ok {
			return nil, fmt.Errorf("allOf[%d]: %s const %v isn't a string", i, field, disc.Value.Const)
		}
		if seen[c] {
			return nil, fmt.Errorf("allOf[%d]: a second branch for %s %s", i, field, c)
		}
		seen[c] = true
		then, err := ObjectFacts(c, p.Then)
		if err != nil {
			return nil, fmt.Errorf("%s branch: %w", c, err)
		}
		out = append(out, Variant{Const: c, Then: then})
	}
	return out, nil
}
