package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Bootstrap holds the minimal facts the server needs before it can reach the
// database. It is written automatically by the in-app setup wizard; users are
// never expected to author or edit it by hand.
type Bootstrap struct {
	DBDriver  string `json:"db_driver"`
	DBDSN     string `json:"db_dsn"`
	Port      int    `json:"port"`
	JWTSecret string `json:"jwt_secret,omitempty"`
}

// BootstrapFileName is the auto-managed file inside the data dir.
const BootstrapFileName = "bootstrap.json"

// DefaultDataDir returns the data directory, overridable via NODELOC_DATA_DIR.
func DefaultDataDir() string {
	if dir := os.Getenv("NODELOC_DATA_DIR"); dir != "" {
		return dir
	}
	return "data"
}

func BootstrapPathIn(dir string) string {
	return filepath.Join(dir, BootstrapFileName)
}

// ReadBootstrap returns the stored bootstrap, found=false when the wizard has
// not been completed yet.
func ReadBootstrap(path string) (Bootstrap, bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Bootstrap{}, false, nil
	}
	if err != nil {
		return Bootstrap{}, false, err
	}
	var b Bootstrap
	if err := json.Unmarshal(data, &b); err != nil {
		return Bootstrap{}, false, fmt.Errorf("parse %s: %w", path, err)
	}
	if b.Port <= 0 {
		b.Port = 8080
	}
	return b, true, nil
}

func (b Bootstrap) WriteTo(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o640); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ApplyTo folds the bootstrap into a Config before viper defaults matter.
func (b Bootstrap) ApplyTo(cfg *Config) {
	cfg.Database.Driver = b.DBDriver
	cfg.Database.DSN = b.DBDSN
	cfg.Server.Port = b.Port
	if b.JWTSecret != "" {
		cfg.JWT.Secret = b.JWTSecret
	}
}
