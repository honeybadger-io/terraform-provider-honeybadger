package specgen

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/getkin/kin-openapi/openapi3"
)

// Attribute is one Terraform attribute, decided but not yet rendered.
type Attribute struct {
	Name               string
	Kind               string // string, int64, float64, bool, list, set
	ElemKind           string // for list and set
	Description        string
	Pattern            string
	CustomType         string
	Required           bool
	Optional           bool
	Computed           bool
	Sensitive          bool
	RequiresReplace    bool
	UseStateForUnknown bool
	Enum               []string
	ElemEnum           []string // list and set items
	ElemFormat         string   // uuid
	Format             string   // "uuid" for a scalar string with format: uuid
	Default            any
	MinLength          int64
	MaxLength          *int64
	Minimum            *float64
	Maximum            *float64
	MinItems           int64 // lists and sets
	MaxItems           *int64
	Attributes         []Attribute // an object's attributes, sorted by name
	NotRead            bool        // write-only, never read back
}

// Resource is a resource's attributes, sorted by name, and its import format.
type Resource struct {
	Name        string
	Description string // the read model's description
	Import      string
	Attributes  []Attribute
	Warnings    []string     // overrides the spec has made redundant
	Parents     []string     // the create operation's path parameters, in order
	IDParam     string       // the read operation's last path parameter
	Variant     *VariantInfo // set for a resource generated from a union branch
}

// customTypes are the custom types resources.yaml may name.
var customTypes = map[string]bool{"period": true}

// Merge applies spec section 5.2's merge rule and table, then the config's
// overrides, to one resource's operations.
func Merge(doc *openapi3.T, name string, rc ResourceConfig) (Resource, error) {
	return mergeWith(doc, name, rc, nil)
}

// mergeWith merges a resource; createObj, when set, replaces the create
// operation's request object (a union branch's narrowed input).
func mergeWith(doc *openapi3.T, name string, rc ResourceConfig, createObj *Object) (Resource, error) {
	fail := func(format string, args ...any) (Resource, error) {
		return Resource{}, fmt.Errorf("resource %s: "+format, append([]any{name}, args...)...)
	}
	if rc.CRUD != "" && rc.CRUD != "generated" && rc.CRUD != "manual" {
		return fail("crud is %q; use generated or manual", rc.CRUD)
	}

	read, err := FindOperation(doc, rc.Read)
	if err != nil {
		return fail("read: %v", err)
	}
	if read.Response == nil {
		return fail("read operation %s has no data object in its response", rc.Read)
	}
	create, err := FindOperation(doc, rc.Create)
	if err != nil {
		return fail("create: %v", err)
	}
	if create.Request == nil || create.Response == nil {
		return fail("create operation %s needs a JSON request body and a data object in its response", rc.Create)
	}
	update := Object{Fields: map[string]Field{}, Required: map[string]bool{}}
	if rc.Update != "" {
		op, err := FindOperation(doc, rc.Update)
		if err != nil {
			return fail("update: %v", err)
		}
		if op.Request == nil || op.Response == nil {
			return fail("update operation %s needs a JSON request body and a data object in its response", rc.Update)
		}
		update = *op.Request
	}
	if rc.Delete != "" {
		if _, err := FindOperation(doc, rc.Delete); err != nil {
			return fail("delete: %v", err)
		}
	}
	R, C, U := read.Response, create.Request, update
	if createObj != nil {
		C = createObj
	}

	parents := map[string]Param{}
	var parentNames []string
	for _, p := range create.PathParams {
		parentNames = append(parentNames, p.Name)
		parents[p.Name] = p
	}

	known := map[string]bool{}
	for _, o := range []*Object{R, C, &U} {
		for f := range o.Fields {
			known[f] = true
		}
	}
	for p := range parents {
		known[p] = true
	}
	overrides := map[string][]string{
		"omit": keys(rc.Omit), "sensitive": rc.Sensitive, "replace": rc.Replace,
		"server_filled": rc.ServerFilled, "use_state_for_unknown": rc.UseStateForUnknown,
		"custom_types": keys(rc.CustomTypes),
	}
	for kind, fields := range overrides {
		for _, f := range fields {
			if !known[f] {
				return fail("%s names %q, which isn't in %s, %s or %s", kind, f, R.Name, C.Name, U.Name)
			}
		}
	}
	for f, ct := range rc.CustomTypes {
		if !customTypes[ct] {
			return fail("custom type %q for %s isn't one specgen knows", ct, f)
		}
	}

	names := make([]string, 0, len(known))
	for f := range known {
		names = append(names, f)
	}
	sort.Strings(names)

	var attrs []Attribute
	var warnings []string
	for _, f := range names {
		if _, omitted := rc.Omit[f]; omitted {
			continue
		}
		if p, ok := parents[f]; ok {
			attrs = append(attrs, Attribute{Name: f, Kind: "string", Required: true, RequiresReplace: true,
				Description: p.Description})
			continue
		}

		rf, inR := R.Fields[f]
		cf, inC := C.Fields[f]
		uf, inU := U.Fields[f]
		writable := (inC || inU) && !rf.ReadOnly
		writeOnly := (inC && cf.WriteOnly) || (inU && uf.WriteOnly)
		secret := rf.Secret || cf.Secret || uf.Secret
		listed := slices.Contains(rc.Sensitive, f)
		if !inR {
			// Never read back: a secret, kept from config, or omitted. A
			// writeOnly flag that's simply not stored isn't a secret.
			if !writeOnly || (!secret && !listed) {
				return fail("%s is in the input but never read back: mark it format: password in the spec if it's a secret, or omit it with a reason", f)
			}
		}

		if writable && inU && !inC && !writeOnly {
			return fail("%s is updatable but create can't set it: omit it, or extend the resource to set it after create", f)
		}

		a := Attribute{Name: f, NotRead: !inR}
		var src Field
		switch {
		case inC:
			src = cf
		case inU:
			src = uf
		default:
			src = rf
		}

		if !writable {
			a.Computed = true
			a.Description = stripCallerWording(rf.Description)
			a.UseStateForUnknown = f == "id"
		} else {
			a.Description = stripCallerWording(src.Description)
			if a.Description == "" && inR {
				a.Description = stripCallerWording(rf.Description)
			}
			def := cf.Default
			if def == nil {
				def = uf.Default
			}
			switch {
			case C.Required[f]:
				a.Required = true
			case def != nil:
				a.Optional, a.Computed, a.Default = true, true, def
			case slices.Contains(rc.ServerFilled, f):
				a.Optional, a.Computed, a.UseStateForUnknown = true, true, true
				// UseStateForUnknown keeps the stored value when the attribute
				// is removed, so the docs say so.
				a.Description = strings.TrimSpace(a.Description + " Removing it from config keeps the current value.")
			case inU && !uf.Nullable && uf.Type != "array":
				// A scalar that can't be sent as null: removing it from config
				// can't clear it. Arrays clear with [] instead.
				a.Optional, a.Computed, a.UseStateForUnknown = true, true, true
			default:
				a.Optional = true
			}
			if !inU {
				a.RequiresReplace = true
			}
			a.Enum = src.Enum
			a.Pattern = src.Pattern
			a.MinLength, a.MaxLength = src.MinLength, src.MaxLength
			a.Minimum, a.Maximum = src.Minimum, src.Maximum
			a.MinItems, a.MaxItems = src.MinItems, src.MaxItems
		}

		// A collection the read schema calls unordered is a set, even when an
		// input doesn't say so.
		if inR && rf.UniqueItems {
			src.UniqueItems = true
		}
		if err := setKind(&a, src); err != nil {
			return fail("%s: %v", f, err)
		}
		a.Sensitive = a.Sensitive || secret || listed
		if secret && listed {
			warnings = append(warnings, fmt.Sprintf("resource %s: sensitive on %s is redundant: the spec marks it format: password; remove it from resources.yaml", name, f))
		}
		if slices.Contains(rc.Replace, f) {
			if !writable {
				return fail("replace on %s has no effect: it isn't writable", f)
			}
			a.RequiresReplace = true
		}
		if slices.Contains(rc.ServerFilled, f) && (!writable || a.Required || a.Default != nil) {
			return fail("server_filled on %s has no effect: it's computed, required or has a default", f)
		}
		if slices.Contains(rc.UseStateForUnknown, f) {
			if !a.Computed || a.Default != nil {
				return fail("use_state_for_unknown on %s conflicts with it being configurable or having a default", f)
			}
			a.UseStateForUnknown = true
		}
		if ct, ok := rc.CustomTypes[f]; ok {
			if a.Kind != "string" {
				return fail("%s: custom type %q needs a string", f, ct)
			}
			a.CustomType = ct
		}
		attrs = append(attrs, a)
	}

	for _, m := range regexp.MustCompile(`<([^>]+)>`).FindAllStringSubmatch(rc.Import, -1) {
		if !slices.ContainsFunc(attrs, func(a Attribute) bool { return a.Name == m[1] }) {
			return fail("import format %q names %s, which isn't an attribute", rc.Import, m[1])
		}
	}
	idParam := ""
	if n := len(read.PathParams); n > 0 {
		idParam = read.PathParams[n-1].Name
	}
	return Resource{Name: name, Description: stripCallerWording(R.Description), Import: rc.Import, Attributes: attrs,
		Warnings: warnings, Parents: parentNames, IDParam: idParam}, nil
}

func setKind(a *Attribute, f Field) error {
	if f.ExclusiveMinimum != nil || f.ExclusiveMaximum != nil {
		return fmt.Errorf("exclusive bounds aren't supported yet; extend specgen")
	}
	scalar := map[string]string{"string": "string", "integer": "int64", "number": "float64", "boolean": "bool"}
	if k, ok := scalar[f.Type]; ok {
		a.Kind = k
		if len(a.Enum) > 0 && k != "string" && k != "int64" {
			return fmt.Errorf("enums on %s aren't supported yet", f.Type)
		}
		if k == "string" && f.Format == "uuid" {
			a.Format = "uuid"
		}
		return nil
	}
	if f.Type == "array" && f.Items != nil {
		if f.Items.Type == "object" {
			if f.Items.Properties == nil {
				return fmt.Errorf("arrays of objects without properties aren't supported")
			}
			children, err := nestedAttributes(*f.Items.Properties, false)
			if err != nil {
				return err
			}
			a.Kind, a.ElemKind, a.Attributes = "list", "object", children
			return emptyDefault(a)
		}
		elem, ok := scalar[f.Items.Type]
		if !ok {
			return fmt.Errorf("arrays of %s aren't supported yet; omit it or extend specgen", f.Items.Type)
		}
		if (len(f.Items.Enum) > 0 || f.Items.Pattern != "") && elem != "string" {
			return fmt.Errorf("enums and patterns on %s items aren't supported yet", f.Items.Type)
		}
		a.Kind, a.ElemKind = "list", elem
		if f.UniqueItems {
			a.Kind = "set"
		}
		a.ElemEnum = f.Items.Enum
		if f.Items.Format == "uuid" {
			a.ElemFormat = "uuid"
		}
		return emptyDefault(a)
	}
	if f.Type == "object" {
		if f.Properties == nil {
			if f.Values == nil || f.Values.Type != "string" {
				return fmt.Errorf("objects without properties are supported only when their values are strings; omit it or extend specgen")
			}
			a.Kind, a.ElemKind = "map", "string"
			a.Sensitive = a.Sensitive || f.Values.Secret
			return emptyDefault(a)
		}
		children, err := nestedAttributes(*f.Properties, a.Computed && !a.Optional)
		if err != nil {
			return err
		}
		a.Kind, a.Attributes = "object", children
		return emptyObjectDefault(a)
	}
	return fmt.Errorf("%s fields aren't supported yet; omit it or extend specgen", f.Type)
}

// emptyDefault allows a collection default only when it's empty: that's
// how the spec says "left out is the same as []" (phase 4b spec, 3.4).
func emptyDefault(a *Attribute) error {
	if a.Default == nil {
		return nil
	}
	if d, ok := a.Default.([]any); ok && len(d) == 0 {
		return nil
	}
	if d, ok := a.Default.(map[string]any); ok && len(d) == 0 {
		return nil
	}
	return fmt.Errorf("a non-empty default on a list or set isn't supported yet")
}

// emptyObjectDefault allows an object default only when it's {} and every
// attribute is optional with no default: then {} means each one null, which
// is how the spec says "left out is the same as {}" (phase 5 spec, 3.5).
func emptyObjectDefault(a *Attribute) error {
	if a.Default == nil {
		return nil
	}
	if d, ok := a.Default.(map[string]any); !ok || len(d) != 0 {
		return fmt.Errorf("a non-empty default on an object isn't supported yet")
	}
	for _, c := range a.Attributes {
		if c.Required || c.Default != nil || c.Kind == "object" {
			return fmt.Errorf("an empty object default needs every attribute optional, with no default and not an object; %s isn't", c.Name)
		}
	}
	return nil
}

// nestedAttributes decides an object's attributes. A read-only (computed)
// object's attributes are all Computed. Otherwise an optional attribute
// must be nullable, or removing it couldn't clear it; a defaulted one is
// Optional and Computed; a write-only one is never read back (phase 4b
// spec, 3.2).
func nestedAttributes(obj Object, computed bool) ([]Attribute, error) {
	names := keys(obj.Fields)
	sort.Strings(names)
	var out []Attribute
	for _, n := range names {
		f := obj.Fields[n]
		if computed {
			a := Attribute{Name: n, Computed: true, Description: stripCallerWording(f.Description)}
			if err := setKind(&a, f); err != nil {
				return nil, fmt.Errorf("%s: %w", n, err)
			}
			out = append(out, a)
			continue
		}
		switch {
		case f.ReadOnly:
			return nil, fmt.Errorf("%s: read-only attributes inside objects aren't supported yet; extend specgen", n)
		case !obj.Required[n] && !f.Nullable && f.Default == nil:
			return nil, fmt.Errorf("%s: an optional attribute inside an object must be nullable, or removing it can't clear it", n)
		}
		a := Attribute{Name: n, Description: stripCallerWording(f.Description), Sensitive: f.Secret || f.WriteOnly,
			Enum: f.Enum, Pattern: f.Pattern, MinLength: f.MinLength, MaxLength: f.MaxLength,
			Minimum: f.Minimum, Maximum: f.Maximum, MinItems: f.MinItems, MaxItems: f.MaxItems}
		switch {
		case obj.Required[n]:
			a.Required = true
		case f.Default != nil:
			a.Optional, a.Computed, a.Default = true, true, f.Default
		default:
			a.Optional = true
		}
		if f.WriteOnly {
			a.NotRead = true
		}
		if err := setKind(&a, f); err != nil {
			return nil, fmt.Errorf("%s: %w", n, err)
		}
		out = append(out, a)
	}
	return out, nil
}

// stripCallerWording drops "Send null to …" instructions: in Terraform,
// removing the attribute is how a value is cleared or reset (spec section 3.3).
// A sentence that continues after a semicolon keeps its continuation.
func stripCallerWording(s string) string {
	var kept []string
	for _, sentence := range splitSentences(strings.TrimSpace(s)) {
		if rest, ok := strings.CutPrefix(sentence, "Send null to"); ok {
			_, after, found := strings.Cut(rest, ";")
			after = strings.TrimSpace(after)
			if !found || after == "" {
				continue
			}
			sentence = strings.ToUpper(after[:1]) + after[1:]
		}
		kept = append(kept, sentence)
	}
	return strings.Join(kept, " ")
}

// splitSentences splits after a period followed by whitespace or the end,
// never inside a `code span`.
func splitSentences(s string) []string {
	var out []string
	inCode, start := false, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '`':
			inCode = !inCode
		case '.':
			if !inCode && (i+1 == len(s) || s[i+1] == ' ' || s[i+1] == '\n') {
				out = append(out, strings.TrimSpace(s[start:i+1]))
				start = i + 1
			}
		}
	}
	if rest := strings.TrimSpace(s[start:]); rest != "" {
		out = append(out, rest)
	}
	return out
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// VariantInfo is the discriminator a variant resource sends and checks.
type VariantInfo struct{ Field, Const string }

// MergeAll merges a config entry: one resource, or one per branch when the
// entry has variants (phase 4b spec, 3.1).
func MergeAll(doc *openapi3.T, name string, rc ResourceConfig) ([]Resource, error) {
	if rc.Variants == "" {
		r, err := Merge(doc, name, rc)
		if err != nil {
			return nil, err
		}
		return []Resource{r}, nil
	}
	create, err := FindOperation(doc, rc.Create)
	if err != nil {
		return nil, fmt.Errorf("resource %s: create: %v", name, err)
	}
	vs, err := Variants(create.RequestSchema, rc.Variants)
	if err != nil {
		return nil, fmt.Errorf("resource %s: %v", name, err)
	}
	allowed := create.Request.Fields[rc.Variants].Enum
	for _, v := range vs {
		if len(allowed) > 0 && !slices.Contains(allowed, v.Const) {
			return nil, fmt.Errorf("resource %s: branch %s isn't one of %s's values %v", name, v.Const, rc.Variants, allowed)
		}
	}
	if len(vs) == 0 {
		return nil, fmt.Errorf("resource %s: the create input has no branches on %s", name, rc.Variants)
	}
	var out []Resource
	for _, v := range vs {
		vrc := rc
		vrc.Omit = map[string]string{rc.Variants: "the discriminator, fixed per resource"}
		for k, why := range rc.Omit {
			vrc.Omit[k] = why
		}
		narrowed, err := narrow(*create.Request, v.Then)
		if err != nil {
			return nil, fmt.Errorf("resource %s, %s branch: %v", name, v.Const, err)
		}
		r, err := mergeWith(doc, name+"_"+snake(v.Const), vrc, &narrowed)
		if err != nil {
			return nil, err
		}
		r.Variant = &VariantInfo{Field: rc.Variants, Const: v.Const}
		if d := stripCallerWording(v.Then.Description); d != "" {
			r.Description = d
		}
		out = append(out, r)
	}
	return out, nil
}

// narrow applies a branch to the shared create input. A branch narrows: it
// can require more, type an untyped object, restrict an item enum, bound
// sizes and declare defaults. Anything else is an error.
func narrow(base, branch Object) (Object, error) {
	out := Object{Name: base.Name, Description: base.Description, Required: map[string]bool{}, Fields: map[string]Field{}}
	for k, v := range base.Required {
		out.Required[k] = v
	}
	for k := range branch.Required {
		if _, ok := base.Fields[k]; !ok {
			return Object{}, fmt.Errorf("requires %s, which the shared input doesn't have", k)
		}
		out.Required[k] = true
	}
	for k, v := range base.Fields {
		out.Fields[k] = v
	}
	for k, b := range branch.Fields {
		f, ok := base.Fields[k]
		if !ok {
			return Object{}, fmt.Errorf("%s isn't in the shared input", k)
		}
		if b.Pattern != "" || b.MinLength > 0 || b.MaxLength != nil || len(b.Enum) > 0 || b.Format != "" ||
			b.Minimum != nil || b.Maximum != nil || b.ExclusiveMinimum != nil || b.ExclusiveMaximum != nil ||
			b.ReadOnly || b.WriteOnly {
			return Object{}, fmt.Errorf("%s: the branch sets a constraint specgen doesn't apply to branches yet; extend narrow", k)
		}
		if b.Type != "" && b.Type != f.Type {
			return Object{}, fmt.Errorf("%s: the branch changes its type from %s to %s", k, f.Type, b.Type)
		}
		if b.Properties != nil {
			if f.Type != "object" || f.Properties != nil {
				return Object{}, fmt.Errorf("%s: the branch types an object that's already typed or isn't one", k)
			}
			f.Properties = b.Properties
			if b.Description != "" {
				f.Description = b.Description
			}
		}
		if b.Items != nil && len(b.Items.Enum) > 0 {
			if f.Items == nil {
				return Object{}, fmt.Errorf("%s: the branch restricts items it doesn't have", k)
			}
			if len(f.Items.Enum) > 0 {
				for _, e := range b.Items.Enum {
					if !slices.Contains(f.Items.Enum, e) {
						return Object{}, fmt.Errorf("%s: the branch allows %q, which the shared input doesn't", k, e)
					}
				}
			}
			items := *f.Items
			items.Enum = b.Items.Enum
			f.Items = &items
		}
		if b.MaxItems != nil {
			if f.MaxItems != nil && *b.MaxItems > *f.MaxItems {
				return Object{}, fmt.Errorf("%s: the branch raises maxItems above the shared input's", k)
			}
			f.MaxItems = b.MaxItems
		}
		if b.MinItems > 0 || f.MinItems > 0 {
			if b.MinItems < f.MinItems {
				return Object{}, fmt.Errorf("%s: the branch lowers minItems below the shared input's", k)
			}
			f.MinItems = b.MinItems
		}
		if b.Default != nil {
			f.Default = b.Default
		}
		if b.Description != "" && b.Properties == nil {
			f.Description = b.Description
		}
		out.Fields[k] = f
	}
	return out, nil
}

// snake turns a type name into a resource name suffix: each capital starts
// a word, and a version (V2) stays one word.
func snake(camel string) string {
	var b strings.Builder
	for i, r := range camel {
		if unicode.IsUpper(r) && i > 0 {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}
