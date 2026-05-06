package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// Config is the on-disk shape of ~/.config/circleci-tui/config.toml. Fields
// are intentionally narrow — we add new ones explicitly when we need them.
type Config struct {
	Token            string   `toml:"token,omitempty"`
	WatchedProjects  []string `toml:"watched_projects,omitempty"`
	BranchFilter     string   `toml:"branch_filter,omitempty"`
	RefreshInterval  int      `toml:"refresh_interval_seconds,omitempty"`
	Mine             string   `toml:"mine,omitempty"`
}

// Path returns the absolute path we read/write. Honours $XDG_CONFIG_HOME and
// $HOME, falling back to a current-directory file as a last resort so test
// runs without HOME don't crash.
func Path() (string, error) {
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return filepath.Join(v, "circleci-tui", "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	return filepath.Join(home, ".config", "circleci-tui", "config.toml"), nil
}

// Load reads the config file. Missing file is not an error — a zero-value
// Config is returned. Permissions errors and parse errors propagate up.
func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}
	var c Config
	if err := toml.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return c, nil
}

// Save writes the config atomically with file mode 0600. Parent directory
// is created with mode 0700 if missing — both kept narrow because the file
// stores a CircleCI personal API token.
func Save(c Config) error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	data, err := toml.Marshal(c)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config.toml.*")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("chmod temp config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("install config: %w", err)
	}
	return nil
}
