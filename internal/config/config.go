package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Repos    []string `yaml:"repos"`
	Interval int      `yaml:"interval"`
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

	for _, r := range cfg.Repos {
		if r == repo {
			return fmt.Errorf("repo %s already in config", repo)
		}
	}

	cfg.Repos = append(cfg.Repos, repo)
	return Save(cfg)
}

func RemoveRepo(repo string) error {
	cfg, err := Load()
	if err != nil {
		return err
	}

	filtered := make([]string, 0, len(cfg.Repos))
	found := false
	for _, r := range cfg.Repos {
		if r == repo {
			found = true
			continue
		}
		filtered = append(filtered, r)
	}

	if !found {
		return fmt.Errorf("repo %s not in config", repo)
	}

	cfg.Repos = filtered
	return Save(cfg)
}
