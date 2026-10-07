package specgen

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// exampleLabel is the label of the first resource block of typeName in
// dir/resource.tf, so docs refer to the resource the example declares; with
// no such block, "example".
func exampleLabel(dir, typeName string) string {
	b, err := os.ReadFile(filepath.Join(dir, "resource.tf"))
	if err != nil {
		return "example"
	}
	re := regexp.MustCompile(`resource\s+"` + regexp.QuoteMeta(typeName) + `"\s+"([^"]+)"`)
	if m := re.FindSubmatch(b); m != nil {
		return string(m[1])
	}
	return "example"
}

// writeOnlyPaths names every attribute import can't read back, nested ones
// by path (config.bearer_token).
func writeOnlyPaths(attrs []Attribute, prefix string) []string {
	var out []string
	for _, a := range attrs {
		if a.NotRead {
			out = append(out, prefix+a.Name)
		}
		out = append(out, writeOnlyPaths(a.Attributes, prefix+a.Name+".")...)
	}
	return out
}

// importFiles are import-by-string-id.tf and import.sh (phase 7a spec, 3.1).
// The ID is quoted so placeholders aren't shell redirection; write-only
// attributes, which import can't read back, are named in a comment.
func importFiles(typeName, label, format string, writeOnly []string) (tf, sh string) {
	note := ""
	if len(writeOnly) > 0 {
		q := make([]string, len(writeOnly))
		for i, w := range writeOnly {
			q[i] = "`" + w + "`"
		}
		note = fmt.Sprintf("# Import can't read %s back: set it in config.\n", strings.Join(q, ", "))
	}
	tf = note + fmt.Sprintf("import {\n  to = %s.%s\n  id = %q\n}\n", typeName, label, format)
	sh = note + fmt.Sprintf("terraform import %s.%s %q\n", typeName, label, format)
	return tf, sh
}

// exampleFiles writes each resource's import files into its examples
// directory (phase 7a spec, 3.1). Without docs_examples it does nothing.
func exampleFiles(cfg Config, configPath string, resources []Resource) error {
	if cfg.DocsExamples == "" {
		return nil
	}
	root := filepath.Join(filepath.Dir(configPath), cfg.DocsExamples, "resources")
	for _, r := range resources {
		if r.Import == "" {
			continue
		}
		typeName := cfg.Provider + "_" + r.Name
		dir := filepath.Join(root, typeName)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		// A generated example is (re)written first, so the import files name
		// its label; a hand-written one is left alone.
		if path := filepath.Join(dir, "resource.tf"); generatedExample(path) {
			src, warnings := exampleConfig(r, typeName, func(param string) (string, bool) {
				x, ok := strings.CutSuffix(param, "_id")
				if _, known := cfg.Resources[x]; !ok || !known {
					return "", false
				}
				parent := cfg.Provider + "_" + x
				return fmt.Sprintf("%s.%s.id", parent, exampleLabel(filepath.Join(root, parent), parent)), true
			})
			for _, w := range warnings {
				fmt.Fprintln(os.Stderr, "specgen: warning: example:", w)
			}
			if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
				return err
			}
		}
		tf, sh := importFiles(typeName, exampleLabel(dir, typeName), r.Import, writeOnlyPaths(r.Attributes, ""))
		if err := os.WriteFile(filepath.Join(dir, "import-by-string-id.tf"), []byte(tf), 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "import.sh"), []byte(sh), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// generatedExampleHeader starts and marks a generated example (phase 7a
// spec, 3.5); it shows on the page, so it's written for readers.
const generatedExampleHeader = "# A minimal configuration, generated from the API spec's examples.\n"

var placeholder = regexp.MustCompile(`^<.*>$`)

// generatedExample reports whether path may be (re)written: missing, or
// starting with the generated header.
func generatedExample(path string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return true
	}
	return strings.HasPrefix(string(b), generatedExampleHeader)
}

// exampleVar is a variable a generated example declares: for a sensitive
// value, or a required one the spec gives no usable example for.
type exampleVar struct {
	name, typ string
	sensitive bool
}

// exampleConfig is a minimal resource block from the spec's examples: every
// required attribute (and an object's required keys), a parent path
// parameter as a reference, a sensitive value as a variable. A required
// value with no usable example becomes a typed variable too, with a warning,
// so the example always validates.
func exampleConfig(r Resource, typeName string, parentRef func(param string) (string, bool)) (string, []string) {
	var warnings []string
	var vars []exampleVar
	var lines []string
	for _, a := range r.Attributes {
		if !a.Required && (a.Kind != "object" || !hasRequired(a.Attributes)) {
			continue
		}
		v := exampleValue(a, r.Name, a.Name, parentRef, &vars, &warnings, slices.Contains(r.Parents, a.Name))
		lines = append(lines, fmt.Sprintf("%s = %s", a.Name, v))
	}
	var b strings.Builder
	b.WriteString(generatedExampleHeader)
	fmt.Fprintf(&b, "resource %q \"example\" {\n", typeName)
	b.WriteString(alignAssignments(lines, "  "))
	b.WriteString("}\n")
	for _, v := range vars {
		if v.sensitive {
			fmt.Fprintf(&b, "\nvariable %q {\n  type      = %s\n  sensitive = true\n}\n", v.name, v.typ)
		} else {
			fmt.Fprintf(&b, "\nvariable %q {\n  type = %s\n}\n", v.name, v.typ)
		}
	}
	return b.String(), warnings
}

func hasRequired(attrs []Attribute) bool {
	return slices.ContainsFunc(attrs, func(a Attribute) bool { return a.Required })
}

// tfType is an attribute's Terraform type expression, for a variable.
func tfType(a Attribute) string {
	scalar := map[string]string{"string": "string", "int64": "number", "float64": "number", "bool": "bool"}
	switch a.Kind {
	case "list", "set":
		return a.Kind + "(" + scalar[a.ElemKind] + ")"
	case "map":
		return "map(" + scalar[a.ElemKind] + ")"
	}
	if t, ok := scalar[a.Kind]; ok {
		return t
	}
	return "any"
}

// exampleValue renders one attribute's example as HCL.
func exampleValue(a Attribute, short, path string, parentRef func(string) (string, bool),
	vars *[]exampleVar, warnings *[]string, parent bool) string {
	if parent {
		if ref, ok := parentRef(a.Name); ok {
			return ref
		}
	}
	name := short + "_" + strings.ReplaceAll(path, ".", "_")
	if a.Sensitive {
		*vars = append(*vars, exampleVar{name, tfType(a), true})
		return "var." + name
	}
	if a.Kind == "object" {
		var inner []string
		for _, c := range a.Attributes {
			if c.Required {
				inner = append(inner, fmt.Sprintf("%s = %s", c.Name,
					exampleValue(c, short, path+"."+c.Name, parentRef, vars, warnings, false)))
			}
		}
		return "{\n" + alignAssignments(inner, "    ") + "  }"
	}
	ex := a.Example
	if ex == nil {
		ex = a.Default
	}
	if ex == nil || hasPlaceholder(ex) {
		*warnings = append(*warnings, fmt.Sprintf("%s.%s has no usable example", short, path))
		*vars = append(*vars, exampleVar{name, tfType(a), false})
		return "var." + name
	}
	if a.CustomType == "json_subset" {
		return "jsonencode(" + hclLiteral(ex) + ")"
	}
	return hclLiteral(ex)
}

// hasPlaceholder reports a "<…>" placeholder anywhere in an example.
func hasPlaceholder(v any) bool {
	switch x := v.(type) {
	case string:
		return placeholder.MatchString(x)
	case []any:
		return slices.ContainsFunc(x, hasPlaceholder)
	case map[string]any:
		for _, e := range x {
			if hasPlaceholder(e) {
				return true
			}
		}
	}
	return false
}

// hclLiteral renders a JSON-decoded value as HCL; object keys are always
// quoted, so any key is valid.
func hclLiteral(v any) string {
	switch x := v.(type) {
	case string:
		return strconv.Quote(x)
	case bool:
		return strconv.FormatBool(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = hclLiteral(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case map[string]any:
		if len(x) == 0 {
			return "{}"
		}
		ks := keys(x)
		sort.Strings(ks)
		parts := make([]string, len(ks))
		for i, k := range ks {
			parts[i] = fmt.Sprintf("%q = %s", k, hclLiteral(x[k]))
		}
		return "{ " + strings.Join(parts, ", ") + " }"
	}
	return strconv.Quote(fmt.Sprint(v))
}

// alignAssignments lines up "name = value" lines as terraform fmt does.
func alignAssignments(lines []string, indent string) string {
	width := 0
	for _, l := range lines {
		if n := strings.Index(l, " = "); n > width && !strings.Contains(l, "\n") {
			width = n
		}
	}
	var b strings.Builder
	for _, l := range lines {
		name, value, _ := strings.Cut(l, " = ")
		if strings.Contains(value, "\n") {
			fmt.Fprintf(&b, "%s%s = %s\n", indent, name, value)
			continue
		}
		fmt.Fprintf(&b, "%s%-*s = %s\n", indent, width, name, value)
	}
	return b.String()
}
