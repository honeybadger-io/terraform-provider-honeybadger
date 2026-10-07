package specgen

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The generated docs, as committed (phase 7a spec, 4): exact import syntax
// and scope lines for representative resources, and an Import section on
// every page.
func TestRegistryDocs(t *testing.T) {
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join("..", "..", "docs", "resources", name+".md"))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	for name, want := range map[string][]string{
		"project": {"terraform import honeybadger_project.app \"<id>\"", "Required scopes: `projects:create`, `projects:read`, `projects:write`."},
		"site":    {"to = honeybadger_site.api", "id = \"<project_id>/<id>\"", "Required scopes: `uptime:read`, `uptime:write`."},
		"status_page": {"terraform import honeybadger_status_page.public \"<id>\"",
			"# Import can't read `password` back: set it in config.",
			"Required scopes: `status_pages:read`, `status_pages:write`."},
		"integration_web_hook": {"terraform import honeybadger_integration_web_hook.alerts \"<project_id>/<id>\"",
			"Required scopes: `integrations:read`, `integrations:write`."},
	} {
		page := read(name)
		for _, w := range want {
			if !strings.Contains(page, w) {
				t.Errorf("%s.md lacks %q", name, w)
			}
		}
	}
	pages, _ := filepath.Glob(filepath.Join("..", "..", "docs", "resources", "*.md"))
	for _, p := range pages {
		b, _ := os.ReadFile(p)
		if !regexp.MustCompile(`(?m)^## Import$`).Match(b) {
			t.Errorf("%s has no Import section", filepath.Base(p))
		}
	}
}

// Every example, generated and hand-written, is valid together against a
// fresh build of the provider; skipped without terraform on PATH.
func TestExamplesValidate(t *testing.T) {
	tf, err := exec.LookPath("terraform")
	if err != nil {
		t.Skip("terraform isn't on PATH")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	build := exec.Command("go", "build", "-o", filepath.Join(bin, "terraform-provider-honeybadger"), ".")
	build.Dir = filepath.Join("..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	rc := filepath.Join(dir, "terraformrc")
	if err := os.WriteFile(rc, []byte("provider_installation {\n  dev_overrides {\n    \"honeybadger-io/honeybadger\" = \""+bin+"\"\n  }\n  direct {}\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(dir, "cfg")
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join("..", "..", "examples", "resources", "*", "resource.tf"))
	var all strings.Builder
	all.WriteString("terraform {\n  required_providers {\n    honeybadger = { source = \"honeybadger-io/honeybadger\" }\n  }\n}\n")
	for _, f := range files {
		b, _ := os.ReadFile(f)
		all.Write(b)
		all.WriteString("\n")
	}
	declared := map[string]bool{}
	for _, m := range regexp.MustCompile(`variable "([^"]+)"`).FindAllStringSubmatch(all.String(), -1) {
		declared[m[1]] = true
	}
	var missing []string
	for _, m := range regexp.MustCompile(`var\.(\w+)`).FindAllStringSubmatch(all.String(), -1) {
		if !declared[m[1]] {
			declared[m[1]] = true
			missing = append(missing, m[1])
		}
	}
	sort.Strings(missing)
	for _, v := range missing {
		all.WriteString("variable \"" + v + "\" {\n  type      = string\n  sensitive = true\n}\n")
	}
	if err := os.WriteFile(filepath.Join(cfg, "main.tf"), []byte(all.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(tf, "validate", "-no-color")
	cmd.Dir = cfg
	cmd.Env = append(os.Environ(), "TF_CLI_CONFIG_FILE="+rc)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("terraform validate: %v\n%s", err, out)
	}
}
