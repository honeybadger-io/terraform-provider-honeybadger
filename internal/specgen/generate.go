package specgen

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Generate writes <name>_resource_gen.go for every resource in configPath, and
// removes generated files for resources no longer configured.
func Generate(specPath, configPath, outDir string) error {
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
	for _, name := range names {
		res, err := Merge(doc, name, cfg.Resources[name])
		if err != nil {
			return err
		}
		for _, w := range res.Warnings {
			fmt.Fprintln(os.Stderr, "specgen: warning:", w)
		}
		src, err := Render(res)
		if err != nil {
			return err
		}
		file := filepath.Join(outDir, name+"_resource_gen.go")
		if err := os.WriteFile(file, src, 0o644); err != nil {
			return err
		}
		written[file] = true
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
