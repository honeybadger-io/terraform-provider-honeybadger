package specgen

import (
	"bytes"
	"fmt"
	"go/format"
	"sort"
	"strconv"
	"strings"
)

// kinds maps an attribute kind to its framework names.
var kinds = map[string]struct {
	schema, model, pkg, elemType string
}{
	"string":  {"StringAttribute", "types.String", "string", "types.StringType"},
	"int64":   {"Int64Attribute", "types.Int64", "int64", "types.Int64Type"},
	"float64": {"Float64Attribute", "types.Float64", "float64", "types.Float64Type"},
	"bool":    {"BoolAttribute", "types.Bool", "bool", "types.BoolType"},
	"list":    {"ListAttribute", "types.List", "list", ""},
	"set":     {"SetAttribute", "types.Set", "set", ""},
}

// customTypeCode is the Go for each custom type: its type and its value.
var customTypeCode = map[string][2]string{
	"period": {"customtypes.PeriodType{}", "customtypes.PeriodValue"},
}

// Render writes a resource's schema function and model struct, and, given a
// mapping, its mapping functions and CRUD wrapper.
func Render(r Resource, m *Mapping) ([]byte, error) {
	imports := map[string]bool{
		"github.com/hashicorp/terraform-plugin-framework/resource/schema": true,
		"github.com/hashicorp/terraform-plugin-framework/types":           true,
	}
	camel := lowerCamel(r.Name)

	var attrs bytes.Buffer
	for _, a := range r.Attributes {
		k, ok := kinds[a.Kind]
		if !ok {
			return nil, fmt.Errorf("resource %s: %s has unknown kind %q", r.Name, a.Name, a.Kind)
		}
		fmt.Fprintf(&attrs, "%q: schema.%s{\n", a.Name, k.schema)
		if a.Kind == "list" || a.Kind == "set" {
			fmt.Fprintf(&attrs, "ElementType: %s,\n", kinds[a.ElemKind].elemType)
		}
		if a.Description != "" {
			fmt.Fprintf(&attrs, "MarkdownDescription: %q,\n", a.Description)
		}
		for _, flag := range []struct {
			on   bool
			name string
		}{{a.Required, "Required"}, {a.Optional, "Optional"}, {a.Computed, "Computed"}, {a.Sensitive, "Sensitive"}} {
			if flag.on {
				fmt.Fprintf(&attrs, "%s: true,\n", flag.name)
			}
		}
		if a.CustomType != "" {
			fmt.Fprintf(&attrs, "CustomType: %s,\n", customTypeCode[a.CustomType][0])
			imports["github.com/honeybadger-io/terraform-provider-honeybadger/internal/provider/customtypes"] = true
		}
		if a.Default != nil {
			lit, err := literal(a.Kind, a.Default)
			if err != nil {
				return nil, fmt.Errorf("resource %s: %s default: %w", r.Name, a.Name, err)
			}
			fmt.Fprintf(&attrs, "Default: %sdefault.Static%s(%s),\n", k.pkg, staticName(a.Kind), lit)
			imports["github.com/hashicorp/terraform-plugin-framework/resource/schema/"+k.pkg+"default"] = true
		}
		var validators []string
		vpkg := k.pkg + "validator"
		if a.MinLength > 0 && a.MaxLength != nil {
			validators = append(validators, fmt.Sprintf("stringvalidator.LengthBetween(%d, %d)", a.MinLength, *a.MaxLength))
		} else if a.MinLength > 0 {
			validators = append(validators, fmt.Sprintf("stringvalidator.LengthAtLeast(%d)", a.MinLength))
		} else if a.MaxLength != nil {
			validators = append(validators, fmt.Sprintf("stringvalidator.LengthAtMost(%d)", *a.MaxLength))
		}
		if a.Minimum != nil || a.Maximum != nil {
			if a.Kind != "int64" && a.Kind != "float64" {
				return nil, fmt.Errorf("resource %s: %s has numeric bounds but kind %s", r.Name, a.Name, a.Kind)
			}
			num := func(f float64) string {
				if a.Kind == "int64" {
					return strconv.FormatInt(int64(f), 10)
				}
				return strconv.FormatFloat(f, 'g', -1, 64)
			}
			switch {
			case a.Minimum != nil && a.Maximum != nil:
				validators = append(validators, fmt.Sprintf("%s.Between(%s, %s)", vpkg, num(*a.Minimum), num(*a.Maximum)))
			case a.Minimum != nil:
				validators = append(validators, fmt.Sprintf("%s.AtLeast(%s)", vpkg, num(*a.Minimum)))
			default:
				validators = append(validators, fmt.Sprintf("%s.AtMost(%s)", vpkg, num(*a.Maximum)))
			}
			imports["github.com/hashicorp/terraform-plugin-framework-validators/"+vpkg] = true
		}
		if len(validators) > 0 && a.Kind == "string" {
			imports["github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"] = true
		}
		if len(a.Enum) > 0 {
			quoted := make([]string, len(a.Enum))
			for i, e := range a.Enum {
				quoted[i] = strconv.Quote(e)
			}
			validators = append(validators, "stringvalidator.OneOf("+strings.Join(quoted, ", ")+")")
			imports["github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"] = true
		}
		if a.Pattern != "" {
			validators = append(validators, fmt.Sprintf("stringvalidator.RegexMatches(regexp.MustCompile(%q), %q)",
				a.Pattern, patternMessage(a)))
			imports["github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"] = true
			imports["regexp"] = true
		}
		if len(validators) > 0 {
			fmt.Fprintf(&attrs, "Validators: []validator.%s{%s},\n", typeName(a.Kind), strings.Join(validators, ", "))
			imports["github.com/hashicorp/terraform-plugin-framework/schema/validator"] = true
		}
		var modifiers []string
		if a.RequiresReplace {
			modifiers = append(modifiers, k.pkg+"planmodifier.RequiresReplace()")
		}
		if a.UseStateForUnknown {
			modifiers = append(modifiers, k.pkg+"planmodifier.UseStateForUnknown()")
		}
		if len(modifiers) > 0 {
			fmt.Fprintf(&attrs, "PlanModifiers: []planmodifier.%s{%s},\n", typeName(a.Kind), strings.Join(modifiers, ", "))
			imports["github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"] = true
			imports["github.com/hashicorp/terraform-plugin-framework/resource/schema/"+k.pkg+"planmodifier"] = true
		}
		attrs.WriteString("},\n")
	}

	var model bytes.Buffer
	for _, a := range r.Attributes {
		goType := kinds[a.Kind].model
		if a.CustomType != "" {
			goType = customTypeCode[a.CustomType][1]
		}
		fmt.Fprintf(&model, "%s %s `tfsdk:%q`\n", upperCamel(a.Name), goType, a.Name)
	}

	if m != nil {
		imports[m.PkgPath] = true
		if m.Period {
			imports["github.com/honeybadger-io/terraform-provider-honeybadger/internal/provider/customtypes"] = true
		}
		if m.Wrapper {
			for _, p := range []string{"context", "errors", "fmt", "github.com/hashicorp/terraform-plugin-framework/resource"} {
				imports[p] = true
			}
		}
	}

	// Standard library first, then everything else, as goimports groups them.
	var std, other []string
	for p := range imports {
		if strings.Contains(strings.SplitN(p, "/", 2)[0], ".") {
			other = append(other, p)
		} else {
			std = append(std, p)
		}
	}
	sort.Strings(std)
	sort.Strings(other)

	var out bytes.Buffer
	out.WriteString("// Code generated by specgen. DO NOT EDIT.\n\npackage provider\n\nimport (\n")
	for _, p := range std {
		fmt.Fprintf(&out, "%q\n", p)
	}
	if len(std) > 0 {
		out.WriteString("\n")
	}
	for _, p := range other {
		fmt.Fprintf(&out, "%q\n", p)
	}
	out.WriteString(")\n\n")
	fmt.Fprintf(&out, "func %sResourceSchema() schema.Schema {\nreturn schema.Schema{\nAttributes: map[string]schema.Attribute{\n%s},\n}\n}\n\n",
		camel, attrs.String())
	fmt.Fprintf(&out, "type %sResourceModel struct {\n%s}\n", camel, model.String())
	if r.Import != "" {
		fmt.Fprintf(&out, "\n// %sImportFormat is the ID terraform import takes.\nconst %sImportFormat = %q\n", camel, camel, r.Import)
	}
	if m != nil {
		d := newWrapperData(r, m)
		if err := mappingTmpl.Execute(&out, d); err != nil {
			return nil, fmt.Errorf("resource %s: rendering the mapping: %w", r.Name, err)
		}
		if m.Wrapper {
			if err := wrapperTmpl.Execute(&out, d); err != nil {
				return nil, fmt.Errorf("resource %s: rendering the wrapper: %w", r.Name, err)
			}
		}
	}

	src, err := format.Source(out.Bytes())
	if err != nil {
		return nil, fmt.Errorf("resource %s: formatting generated code: %w\n%s", r.Name, err, out.String())
	}
	return src, nil
}

// customTypeMessages describe each custom type's format for validation errors.
var customTypeMessages = map[string]string{
	"period": `must be a period such as "5 minutes", "1 hour 30 minutes" or "01:30:00"`,
}

// patternMessage is the error users see for a value that doesn't match. A
// short pattern is readable as is; a long one isn't, so the message points at
// the description instead.
func patternMessage(a Attribute) string {
	if msg, ok := customTypeMessages[a.CustomType]; ok {
		return msg
	}
	if len(a.Pattern) <= 40 {
		return "must match " + a.Pattern
	}
	return "isn't in the format the Honeybadger API accepts; see the attribute's description"
}

func literal(kind string, v any) (string, error) {
	switch kind {
	case "string":
		return strconv.Quote(fmt.Sprint(v)), nil
	case "bool":
		b, ok := v.(bool)
		if !ok {
			return "", fmt.Errorf("%v isn't a boolean", v)
		}
		return strconv.FormatBool(b), nil
	case "int64", "float64":
		n, ok := v.(float64)
		if !ok {
			return "", fmt.Errorf("%v isn't a number", v)
		}
		if kind == "int64" {
			return strconv.FormatInt(int64(n), 10), nil
		}
		return strconv.FormatFloat(n, 'g', -1, 64), nil
	}
	return "", fmt.Errorf("no default for %s", kind)
}

func staticName(kind string) string {
	return map[string]string{"string": "String", "bool": "Bool", "int64": "Int64", "float64": "Float64"}[kind]
}

func typeName(kind string) string {
	return map[string]string{"string": "String", "int64": "Int64", "float64": "Float64", "bool": "Bool",
		"list": "List", "set": "Set"}[kind]
}

// initialisms are rendered in capitals, as Go names them.
var initialisms = map[string]string{"id": "ID", "url": "URL", "api": "API", "ip": "IP", "uuid": "UUID",
	"http": "HTTP", "https": "HTTPS", "json": "JSON", "sms": "SMS"}

func upperCamel(snake string) string {
	var b strings.Builder
	for _, part := range strings.Split(snake, "_") {
		if up, ok := initialisms[part]; ok {
			b.WriteString(up)
		} else if part != "" {
			b.WriteString(strings.ToUpper(part[:1]) + part[1:])
		}
	}
	return b.String()
}

func lowerCamel(snake string) string {
	u := upperCamel(snake)
	return strings.ToLower(u[:1]) + u[1:]
}

// RenderRegistry writes resources_gen.go: every generated wrapper, for the
// provider's Resources.
func RenderRegistry(names []string) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("// Code generated by specgen. DO NOT EDIT.\n\npackage provider\n\nimport \"github.com/hashicorp/terraform-plugin-framework/resource\"\n\n")
	b.WriteString("// generatedResources lists the resources specgen generates wrappers for.\nfunc generatedResources() []func() resource.Resource {\n\treturn []func() resource.Resource{\n")
	for _, n := range names {
		fmt.Fprintf(&b, "\t\tnew%sResource,\n", upperCamel(n))
	}
	b.WriteString("\t}\n}\n")
	return format.Source(b.Bytes())
}
