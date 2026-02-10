package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// Config holds JIRA connection settings.
type Config struct {
	URL   string `yaml:"url"   mapstructure:"url"`
	Email string `yaml:"email" mapstructure:"email"`
	Token string `yaml:"token" mapstructure:"token"`
}

// DefaultPath returns the default config file path (~/.jira-cli.yaml).
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".jira-cli.yaml"
	}
	return filepath.Join(home, ".jira-cli.yaml")
}

// Load reads config from the YAML file and applies env var overrides.
// configPath may be empty to use the default path.
func Load(configPath string) (Config, error) {
	v := viper.New()

	if configPath == "" {
		configPath = DefaultPath()
	}

	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")

	// Env var overrides
	v.BindEnv("url", "JIRA_URL")
	v.BindEnv("email", "JIRA_EMAIL")
	v.BindEnv("token", "JIRA_TOKEN")

	// Read the config file (ignore "not found" errors so env vars still work)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			// Only ignore file-not-found; other errors (e.g. parse) are real
			if !os.IsNotExist(err) {
				return Config{}, fmt.Errorf("reading config: %w", err)
			}
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshalling config: %w", err)
	}

	return cfg, nil
}

// Validate checks that required fields are present.
func (c Config) Validate() error {
	if c.URL == "" {
		return fmt.Errorf("JIRA URL is required (set in config file or JIRA_URL env var)")
	}
	if c.Email == "" {
		return fmt.Errorf("JIRA email is required (set in config file or JIRA_EMAIL env var)")
	}
	if c.Token == "" {
		return fmt.Errorf("JIRA token is required (set in config file or JIRA_TOKEN env var)")
	}
	return nil
}

// Save writes the config to the given path (or default path if empty).
func Save(cfg Config, configPath string) error {
	if configPath == "" {
		configPath = DefaultPath()
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshalling config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}

	return nil
}
