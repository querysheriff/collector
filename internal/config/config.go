package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"

	"github.com/goccy/go-yaml"
)

type Config struct {
	Backend  BackendConfig `yaml:"backend"`
	Postgres string        `yaml:"postgres"`
	Logs     LogsConfig    `yaml:"logs"`
}

type BackendConfig struct {
	URL   string `yaml:"url"`
	Token string `yaml:"token"`
}

type LogsConfig struct {
	Path                  string `yaml:"path"`
	DeleteRotated         bool   `yaml:"delete_rotated"`
	DeleteRotatedJSONOnly bool   `yaml:"delete_rotated_json_only"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}

	var cfg Config
	if err := yaml.UnmarshalWithOptions(data, &cfg, yaml.Strict()); err != nil {
		return Config{}, fmt.Errorf("parse config %q: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("invalid config %q: %w", path, err)
	}

	return cfg, nil
}

func (c Config) Validate() error {
	if c.Backend.URL == "" {
		return errors.New("backend.url is required")
	}

	if err := validateBackendURL(c.Backend.URL); err != nil {
		return err
	}

	if c.Backend.Token == "" {
		return errors.New("backend.token is required")
	}

	if c.Postgres == "" {
		return errors.New("postgres is required")
	}

	return nil
}

func validateBackendURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("backend.url is not a valid URL: %w", err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("backend.url must use http or https, got %q", u.Scheme)
	}

	return nil
}
