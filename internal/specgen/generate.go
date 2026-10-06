package specgen

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Generate writes <name>_resource_gen.go for every resource in configPath and
// resources_gen.go listing the generated wrappers, and removes generated files
// for resources no longer configured. apiPath is the client package the
// mapping is checked against (apiv3).
func Generate(specPath, configPath, outDir, apiPath string) error {
	doc, err := Load(specPath)
	if err != nil {
		return err
	}
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return err
	}

	names := make([]string, 0, len(cfg.Resources))
	for name := range cfg.Resources {
		names = append(names, name)
	}
	sort.Strings(names)

	written := map[string]bool{}
	var wrappers []string
	var api *API
	for _, name := range names {
		rc := cfg.Resources[name]
		res, err := Merge(doc, name, rc)
		if err != nil {
			return err
		}
		for _, w := range res.Warnings {
			fmt.Fprintln(os.Stderr, "specgen: warning:", w)
		}
		var m *Mapping
		if rc.API.Model != "" {
			if api == nil {
				if apiPath == "" {
					return fmt.Errorf("resource %s: needs the client package's import path (-api)", name)
				}
				if api, err = LoadAPI(outDir, apiPath); err != nil {
					return err
				}
			}
			if m, err = Map(api, res, rc); err != nil {
				return err
			}
			if m.Wrapper {
				wrappers = append(wrappers, name)
			}
		}
		src, err := Render(res, m)
		if err != nil {
			return err
		}
		file := filepath.Join(outDir, name+"_resource_gen.go")
		if err := os.WriteFile(file, src, 0o644); err != nil {
			return err
		}
		written[file] = true
	}

	// Always written, even empty, so the provider's call to it compiles.
	registry, err := RenderRegistry(wrappers)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "resources_gen.go"), registry, 0o644); err != nil {
		return err
	}

	existing, err := filepath.Glob(filepath.Join(outDir, "*_resource_gen.go"))
	if err != nil {
		return err
	}
	for _, file := range existing {
		if !written[file] {
			if err := os.Remove(file); err != nil {
				return fmt.Errorf("removing stale %s: %w", file, err)
			}
		}
	}
	return nil
}
