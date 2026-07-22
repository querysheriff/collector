package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/querysheriff/collector/internal/config"
)

func validConfig() config.Config {
	return config.Config{
		Backend:  config.BackendConfig{URL: "https://backend.example", Token: "qsc_token"},
		Postgres: "postgres://localhost/db",
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		mutate  func(*config.Config)
		wantErr bool
	}{
		{"valid https", func(*config.Config) {}, false},
		{"missing url", func(c *config.Config) { c.Backend.URL = "" }, true},
		{"missing token", func(c *config.Config) { c.Backend.Token = "" }, true},
		{"missing postgres", func(c *config.Config) { c.Postgres = "" }, true},
		{"http localhost allowed", func(c *config.Config) { c.Backend.URL = "http://localhost:3000" }, false},
		{"http loopback ip allowed", func(c *config.Config) { c.Backend.URL = "http://127.0.0.1:3000" }, false},
		{"http remote rejected", func(c *config.Config) { c.Backend.URL = "http://backend.example" }, true},
		{"non-http scheme rejected", func(c *config.Config) { c.Backend.URL = "ftp://backend.example" }, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			cfg := validConfig()
			c.mutate(&cfg)
			err := cfg.Validate()
			if (err != nil) != c.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, c.wantErr)
			}
		})
	}
}

func TestLoadValid(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "collector.yml")
	write(t, path, "backend:\n  url: https://backend.example\n  token: qsc_token\n"+
		"postgres: postgres://localhost/db\nlogs:\n  path: /var/log/pg\n  delete_rotated: true\n")

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Backend.URL != "https://backend.example" || cfg.Backend.Token != "qsc_token" {
		t.Errorf("backend = %+v", cfg.Backend)
	}
	if cfg.Logs.Path != "/var/log/pg" || !cfg.Logs.DeleteRotated {
		t.Errorf("logs = %+v", cfg.Logs)
	}
}

func TestLoadRejectsUnknownKey(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "collector.yml")
	write(t, path, "backend:\n  url: https://backend.example\n  token: qsc_token\n"+
		"postgres: postgres://localhost/db\nlogz: oops\n")

	if _, err := config.Load(path); err == nil {
		t.Errorf("Load should reject an unknown top-level key in strict mode")
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}
