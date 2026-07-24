package config

import (
	"errors"
	"fmt"
	"net"
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
	if parseErr := yaml.UnmarshalWithOptions(data, &cfg, yaml.Strict()); parseErr != nil {
		return Config{}, fmt.Errorf("parse config %q: %w", path, parseErr)
	}

	if validateErr := cfg.Validate(); validateErr != nil {
		return Config{}, fmt.Errorf("invalid config %q: %w", path, validateErr)
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

// validateBackendURL requires https so the collector token is never sent in cleartext;
// http is permitted only for a loopback host, to keep local development working.
func validateBackendURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("backend.url is not a valid URL: %w", err)
	}

	switch u.Scheme {
	case "https":
		return nil
	case "http":
		if !isLoopbackHost(u.Hostname()) {
			return errors.New("backend.url must use https; http is allowed only for localhost")
		}

		return nil
	default:
		return fmt.Errorf("backend.url must use http or https, got %q", u.Scheme)
	}
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}

	ip := net.ParseIP(host)

	return ip != nil && ip.IsLoopback()
}
