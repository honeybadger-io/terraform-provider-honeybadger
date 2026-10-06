package specgen

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

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
	Default            any
	MinLength          int64
	MaxLength          *int64
	Minimum            *float64
	Maximum            *float64
	NotRead            bool // write-only, never read back
}

// Resource is a resource's attributes, sorted by name, and its import format.
type Resource struct {
	Name       string
	Import     string
	Attributes []Attribute
	Warnings   []string // overrides the spec has made redundant
	Parents    []string // the create operation's path parameters, in order
	IDParam    string   // the read operation's last path parameter
}

// customTypes are the custom types resources.yaml may name.
var customTypes = map[string]bool{"period": true}

// Merge applies spec section 5.2's merge rule and table, then the config's
// overrides, to one resource's operations.
func Merge(doc *openapi3.T, name string, rc ResourceConfig) (Resource, error) {
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
			if src.ItemBounds {
				return fail("%s: minItems and maxItems aren't supported yet; extend specgen", f)
			}
		}

		// A collection the read schema calls unordered is a set, even when an
		// input doesn't say so.
		if inR && rf.UniqueItems {
			src.UniqueItems = true
		}
		if err := setKind(&a, src); err != nil {
			return fail("%s: %v", f, err)
		}
		a.Sensitive = secret || listed
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
	return Resource{Name: name, Import: rc.Import, Attributes: attrs, Warnings: warnings,
		Parents: parentNames, IDParam: idParam}, nil
}

func setKind(a *Attribute, f Field) error {
	scalar := map[string]string{"string": "string", "integer": "int64", "number": "float64", "boolean": "bool"}
	if k, ok := scalar[f.Type]; ok {
		a.Kind = k
		if len(a.Enum) > 0 && k != "string" {
			return fmt.Errorf("enums on %s aren't supported yet", f.Type)
		}
		return nil
	}
	if f.Type == "array" && f.Items != nil {
		elem, ok := scalar[f.Items.Type]
		if !ok {
			return fmt.Errorf("arrays of %s aren't supported yet; omit it or extend specgen", f.Items.Type)
		}
		if len(f.Items.Enum) > 0 || f.Items.Pattern != "" {
			return fmt.Errorf("enums and patterns on array items aren't supported yet; omit it or extend specgen")
		}
		a.Kind, a.ElemKind = "list", elem
		if f.UniqueItems {
			a.Kind = "set"
		}
		if a.Default != nil {
			return fmt.Errorf("defaults on lists and sets aren't supported yet")
		}
		return nil
	}
	return fmt.Errorf("%s fields aren't supported yet; omit it or extend specgen", f.Type)
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
