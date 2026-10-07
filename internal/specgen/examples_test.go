package specgen

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Review Focus 5: the label is the first block of the resource's own type.
func TestExampleLabel(t *testing.T) {
	dir := t.TempDir()
	if got := exampleLabel(dir, "honeybadger_site"); got != "example" {
		t.Errorf("no file: %q, want example", got)
	}
	src := "resource \"honeybadger_project\" \"app\" {}\n\nresource \"honeybadger_site\" \"api\" {}\n"
	if err := os.WriteFile(filepath.Join(dir, "resource.tf"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := exampleLabel(dir, "honeybadger_site"); got != "api" {
		t.Errorf("got %q, want api", got)
	}
}

func TestWriteOnlyPaths(t *testing.T) {
	attrs := []Attribute{
		{Name: "password", NotRead: true},
		{Name: "config", Attributes: []Attribute{{Name: "url"}, {Name: "bearer_token", NotRead: true}}},
	}
	if got := writeOnlyPaths(attrs, ""); !reflect.DeepEqual(got, []string{"password", "config.bearer_token"}) {
		t.Errorf("got %v", got)
	}
}

// Review Focus 1: an account-level format is quoted too.
func TestImportFiles(t *testing.T) {
	tf, sh := importFiles("honeybadger_site", "api", "<project_id>/<id>", nil)
	if tf != "import {\n  to = honeybadger_site.api\n  id = \"<project_id>/<id>\"\n}\n" {
		t.Errorf("tf = %q", tf)
	}
	if sh != "terraform import honeybadger_site.api \"<project_id>/<id>\"\n" {
		t.Errorf("sh = %q", sh)
	}
	tf, sh = importFiles("honeybadger_status_page", "public", "<id>", []string{"password"})
	if sh != "# Import can't read `password` back: set it in config.\nterraform import honeybadger_status_page.public \"<id>\"\n" {
		t.Errorf("sh = %q", sh)
	}
	if tf != "# Import can't read `password` back: set it in config.\nimport {\n  to = honeybadger_status_page.public\n  id = \"<id>\"\n}\n" {
		t.Errorf("tf = %q", tf)
	}
}

func TestExampleConfig(t *testing.T) {
	_, r := mapSprocket(t)
	src, warnings := exampleConfig(r, "honeybadger_sprocket", func(string) (string, bool) { return "", false })
	// Required attributes only: gizmos and headers are optional (defaulted),
	// so headers' placeholder example raises no warning here.
	want := generatedExampleHeader + `resource "honeybadger_sprocket" "example" {
  name      = "Main sprocket"
  parent_id = "0b6f2b8e-4f7c-4c1a-9a51-6c1f0e1b2a3d"
}
`
	if src != want || len(warnings) != 0 {
		t.Errorf("src =\n%s\nwant\n%s\nwarnings %q", src, want, warnings)
	}
}

// A parent path parameter becomes a reference to the parent's example; a
// sensitive value of any kind becomes a typed, sensitive variable; a
// placeholder or missing example becomes a typed variable with a warning,
// so the example still validates; map keys are quoted.
func TestExampleConfigReferencesAndSecrets(t *testing.T) {
	r := Resource{Name: "thing", Parents: []string{"project_id"}, Attributes: []Attribute{
		{Name: "project_id", Kind: "string", Required: true},
		{Name: "token", Kind: "string", Required: true, Sensitive: true},
		{Name: "headers", Kind: "map", ElemKind: "string", Required: true, Sensitive: true},
		{Name: "label", Kind: "string", Required: true, Example: "<Label>"},
		{Name: "count", Kind: "int64", Required: true},
		{Name: "tags", Kind: "map", ElemKind: "string", Required: true, Example: map[string]any{"a/b": "<x>"}},
		{Name: "config", Kind: "object", Required: true, Attributes: []Attribute{
			{Name: "url", Kind: "string", Required: true, Example: "https://example.com/hook"},
			{Name: "note", Kind: "string", Optional: true, Example: "skipped: optional"},
		}},
	}}
	src, warnings := exampleConfig(r, "honeybadger_thing", func(p string) (string, bool) {
		return "honeybadger_project.app.id", p == "project_id"
	})
	for _, want := range []string{
		"project_id = honeybadger_project.app.id",
		"token      = var.thing_token",
		"headers    = var.thing_headers",
		"label      = var.thing_label",
		"count      = var.thing_count",
		"tags       = var.thing_tags",
		"config = {\n    url = \"https://example.com/hook\"\n  }",
		"variable \"thing_token\" {\n  type      = string\n  sensitive = true\n}",
		"variable \"thing_headers\" {\n  type      = map(string)\n  sensitive = true\n}",
		"variable \"thing_count\" {\n  type = number\n}",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("src lacks %q:\n%s", want, src)
		}
	}
	if len(warnings) != 3 {
		t.Errorf("warnings = %q, want label, count and tags", warnings)
	}
	if got := hclLiteral(map[string]any{"a/b": "c"}); got != `{ "a/b" = "c" }` {
		t.Errorf("hclLiteral map = %s", got)
	}
}

// Hand-written examples are never overwritten; a generated one is.
func TestExampleFileOwnership(t *testing.T) {
	dir := t.TempDir()
	hand := filepath.Join(dir, "resource.tf")
	if err := os.WriteFile(hand, []byte("resource \"x\" \"y\" {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if generatedExample(hand) {
		t.Error("a hand-written example counted as generated")
	}
	if err := os.WriteFile(hand, []byte(generatedExampleHeader+"resource \"x\" \"y\" {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !generatedExample(hand) {
		t.Error("a generated example wasn't recognised")
	}
	if !generatedExample(filepath.Join(dir, "missing.tf")) {
		t.Error("a missing example should be writable")
	}
}
