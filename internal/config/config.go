package config

import (
	"fmt"
	"os"

	blit "github.com/blitui/blit"
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
	var s string
	if err := unmarshal(&s); err == nil {
		r.Name = s
		return nil
	}
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

// Config holds the application configuration.
// blit struct tags enable auto-generated ConfigEditor and CLI commands.
type Config struct {
	RepoEntries []RepoEntry `yaml:"repos" blit:"label=Repos,group=Config,hint=Watched repos (owner/repo format)"`
	Interval    int         `yaml:"interval" blit:"label=Interval (sec),group=Polling,hint=Poll frequency (min 5),default=30,min=5"`
	Theme       string      `yaml:"theme,omitempty" blit:"label=Theme,group=Appearance,hint=Theme name (use ctrl+t to pick),readonly=true"`
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

// blitCfg is the blit.Config wrapper. Initialized on first Load.
var blitCfg *blit.Config[Config]

// Load loads the config using blit.Config[T] with struct tag defaults.
func Load() (*Config, error) {
	var err error
	if blitCfg == nil {
		blitCfg, err = blit.LoadConfig[Config]("gitstream")
		if err != nil {
			// Check if the file doesn't exist to give a helpful message.
			path, pathErr := blit.DefaultConfigPath("gitstream")
			if pathErr == nil {
				if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
					return nil, fmt.Errorf("no config found at %s - run 'gitstream add owner/repo' to get started", path)
				}
			}
			return nil, err
		}
	}
	return &blitCfg.Value, nil
}

// Save persists the current config value.
func Save(cfg *Config) error {
	if blitCfg != nil {
		blitCfg.Value = *cfg
		return blitCfg.Save()
	}
	path, _ := blit.DefaultConfigPath("gitstream")
	return blit.SaveYAML(path, cfg)
}

// Editor returns a blit.ConfigEditor auto-generated from struct tags.
func Editor() *blit.ConfigEditor {
	if blitCfg == nil {
		if _, err := Load(); err != nil {
			return nil
		}
	}
	return blitCfg.Editor()
}

// CLICommands returns auto-generated CLI commands for config fields.
func CLICommands() map[string]blit.CLICommand {
	if blitCfg == nil {
		if _, err := Load(); err != nil {
			return nil
		}
	}
	return blitCfg.CLICommands()
}

func AddRepo(repo string) error {
	if _, err := blit.EnsureConfigDir("gitstream"); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	cfg, err := Load()
	if err != nil {
		cfg = &Config{Interval: 30}
		blitCfg, _ = blit.LoadConfig[Config]("gitstream")
		if blitCfg != nil {
			blitCfg.Value = *cfg
		}
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
