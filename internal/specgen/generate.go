package specgen

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
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
	outputs := map[string][]byte{} // written only after the rename guard passes
	var wrappers []string
	variants := map[string]map[string]string{}
	var api *API
	for _, name := range names {
		rc := cfg.Resources[name]
		rc.Entry, rc.Provider = name, cfg.Provider
		resources, err := MergeAll(doc, name, rc)
		if err != nil {
			return err
		}
		for _, res := range resources {
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
					wrappers = append(wrappers, res.Name)
				}
			}
			if res.Variant != nil {
				if variants[name] == nil {
					variants[name] = map[string]string{}
				}
				variants[name][res.Variant.Const] = cfg.Provider + "_" + res.Name
			}
			src, err := Render(res, m)
			if err != nil {
				return err
			}
			file := filepath.Join(outDir, res.Name+"_resource_gen.go")
			outputs[file] = src
			written[file] = true
		}
	}

	// A committed generated resource can't disappear unless it's retired
	// (phase 4b spec, 3.6). Check before writing anything, so a refused run
	// leaves the tree unchanged.
	existing, err := filepath.Glob(filepath.Join(outDir, "*_resource_gen.go"))
	if err != nil {
		return err
	}
	var retired []string
	for _, file := range existing {
		if written[file] {
			continue
		}
		name := strings.TrimSuffix(filepath.Base(file), "_resource_gen.go")
		if !slices.Contains(cfg.Retired, name) {
			return fmt.Errorf("generation would remove resource %s (%s); if that's intended, list %s under retired in %s",
				name, filepath.Base(file), name, configPath)
		}
		retired = append(retired, file)
	}

	for file, src := range outputs {
		if err := os.WriteFile(file, src, 0o644); err != nil {
			return err
		}
	}
	// Always written, even empty, so the provider's call to it compiles.
	registry, err := RenderRegistry(wrappers, variants)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "resources_gen.go"), registry, 0o644); err != nil {
		return err
	}
	for _, file := range retired {
		if err := os.Remove(file); err != nil {
			return fmt.Errorf("removing retired %s: %w", file, err)
		}
	}
	return nil
}
