package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// RepoEntry represents a watched repo with an optional local path override.
type RepoEntry struct {
	Name string `yaml:"name"`
	Path string `yaml:"path,omitempty"`
}

// UnmarshalYAML supports both string and object formats:
//
//	repos:
//	  - owner/repo
//	  - name: owner/repo
//	    path: /home/user/projects/repo
func (r *RepoEntry) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Try plain string first
	var s string
	if err := unmarshal(&s); err == nil {
		r.Name = s
		return nil
	}
	// Otherwise struct
	type raw RepoEntry
	return unmarshal((*raw)(r))
}

// MarshalYAML writes as a plain string if no path is set.
func (r RepoEntry) MarshalYAML() (interface{}, error) {
	if r.Path == "" {
		return r.Name, nil
	}
	type raw RepoEntry
	return (raw)(r), nil
}

type Config struct {
	RepoEntries []RepoEntry `yaml:"repos"`
	Interval    int         `yaml:"interval"`
}

// Repos returns just the repo name strings for backward compatibility.
func (c *Config) Repos() []string {
	names := make([]string, len(c.RepoEntries))
	for i, r := range c.RepoEntries {
		names[i] = r.Name
	}
	return names
}

// ExplicitPaths returns a map of remote -> local path for repos with explicit paths.
func (c *Config) ExplicitPaths() map[string]string {
	m := make(map[string]string)
	for _, r := range c.RepoEntries {
		if r.Path != "" {
			m[r.Name] = r.Path
		}
	}
	return m
}

func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "gitstream", "config.yaml")
}

func Load() (*Config, error) {
	path := DefaultPath()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no config found at %s - run 'gitstream add owner/repo' to get started", path)
		}
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	if cfg.Interval <= 0 {
		cfg.Interval = 30
	}

	return &cfg, nil
}

func Save(cfg *Config) error {
	path := DefaultPath()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o644)
}

func AddRepo(repo string) error {
	cfg, err := Load()
	if err != nil {
		cfg = &Config{Interval: 30}
	}

	for _, r := range cfg.RepoEntries {
		if r.Name == repo {
			return fmt.Errorf("repo %s already in config", repo)
		}
	}

	cfg.RepoEntries = append(cfg.RepoEntries, RepoEntry{Name: repo})
	return Save(cfg)
}

func RemoveRepo(repo string) error {
	cfg, err := Load()
	if err != nil {
		return err
	}

	filtered := make([]RepoEntry, 0, len(cfg.RepoEntries))
	found := false
	for _, r := range cfg.RepoEntries {
		if r.Name == repo {
			found = true
			continue
		}
		filtered = append(filtered, r)
	}

	if !found {
		return fmt.Errorf("repo %s not in config", repo)
	}

	cfg.RepoEntries = filtered
	return Save(cfg)
}
