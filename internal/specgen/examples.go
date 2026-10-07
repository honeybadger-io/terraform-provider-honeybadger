package specgen

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
