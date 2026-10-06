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
	Resources map[string]ResourceConfig `yaml:"resources"`
}

// ResourceConfig is one resource's entry.
type ResourceConfig struct {
	Create             string            `yaml:"create"`
	Read               string            `yaml:"read"`
	Update             string            `yaml:"update"`
	Delete             string            `yaml:"delete"`
	Import             string            `yaml:"import"`
	Omit               map[string]string `yaml:"omit"` // field -> reason
	Sensitive          []string          `yaml:"sensitive"`
	Replace            []string          `yaml:"replace"`
	ServerFilled       []string          `yaml:"server_filled"`
	UseStateForUnknown []string          `yaml:"use_state_for_unknown"`
	CustomTypes        map[string]string `yaml:"custom_types"`      // field -> registry key
	Service            string            `yaml:"service"`           // apiv3.Client field, e.g. CheckIns
	API                APIConfig         `yaml:"api"`               // apiv3 types the operations use
	FindHint           string            `yaml:"find_hint"`         // for the unknown-outcome error
	ConfigValidators   string            `yaml:"config_validators"` // hand-written func returning []resource.ConfigValidator
	CRUD               string            `yaml:"crud"`              // "" or "generated" (wrapper generated), or "manual"
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
