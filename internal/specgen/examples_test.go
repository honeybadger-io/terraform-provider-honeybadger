package specgen

import (
	"os"
	"path/filepath"
	"reflect"
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
