package specgen

import (
	"errors"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is resources.yaml: per resource, the operations it uses and the
// overrides for facts the spec doesn't state (spec section 5.2).
type Config struct {
	DocsTemplates string                    `yaml:"docs_templates"` // tfplugindocs templates directory, relative to this file
	DocsExamples  string                    `yaml:"docs_examples"`  // tfplugindocs examples directory, relative to this file
	Retired       []string                  `yaml:"retired"`        // generated resources deliberately removed
	Provider      string                    `yaml:"provider"`       // the provider's type-name prefix, e.g. honeybadger
	Resources     map[string]ResourceConfig `yaml:"resources"`
}

// ResourceConfig is one resource's entry.
type ResourceConfig struct {
	Create             string              `yaml:"create"`
	Read               string              `yaml:"read"`
	Update             string              `yaml:"update"`
	Delete             string              `yaml:"delete"`
	Import             string              `yaml:"import"`
	Omit               map[string]string   `yaml:"omit"` // field -> reason
	Sensitive          []string            `yaml:"sensitive"`
	Replace            []string            `yaml:"replace"`
	ServerFilled       []string            `yaml:"server_filled"`
	UseStateForUnknown []string            `yaml:"use_state_for_unknown"`
	CustomTypes        map[string]string   `yaml:"custom_types"`       // field -> registry key
	JSONRequiredKeys   map[string][]string `yaml:"json_required_keys"` // json_subset field -> keys each array element must set
	Service            string              `yaml:"service"`            // apiv3.Client field, e.g. CheckIns
	API                APIConfig           `yaml:"api"`                // apiv3 types the operations use
	FindHint           string              `yaml:"find_hint"`          // for the unknown-outcome error
	ConfigValidators   string              `yaml:"config_validators"`  // hand-written func returning []resource.ConfigValidator
	CRUD               string              `yaml:"crud"`               // "" or "generated" (wrapper generated), or "manual"
	Variants           string              `yaml:"variants"`           // discriminator: one resource per create branch
	DocsCategory       string              `yaml:"docs_category"`      // the registry sidebar group for its pages
	Entry              string              `yaml:"-"`                  // the entry name, set by Generate
	Provider           string              `yaml:"-"`                  // the provider prefix, from Config
}

// APIConfig names the apiv3 types a resource's operations use.
type APIConfig struct {
	Model        string `yaml:"model"`
	CreateParams string `yaml:"create_params"`
	UpdateParams string `yaml:"update_params"`
}

// LoadConfig reads resources.yaml, rejecting unknown keys.
func LoadConfig(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer func() { _ = f.Close() }()
	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	var c Config
	if err := dec.Decode(&c); err != nil && !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}
