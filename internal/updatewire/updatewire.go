// Package updatewire builds the tuikit UpdateConfig used by gitstream.
// Extracting this into a tiny helper makes the update pipeline testable
// against updatetest.NewMockServer without spinning up the full TUI.
package updatewire

import (
	tuikit "github.com/moneycaringcoder/tuikit-go"
)

// New returns a UpdateConfig wired for the gitstream binary. Callers pass
// the current version string (typically set via ldflags). The mode is
// UpdateForced so release notes carrying a `minimum_version:` marker
// trigger the full-screen update gate automatically.
func New(version string) tuikit.UpdateConfig {
	return tuikit.UpdateConfig{
		Owner:      "moneycaringcoder",
		Repo:       "gitstream-tui",
		BinaryName: "gitstream",
		Version:    version,
		Mode:       tuikit.UpdateForced,
	}
}

// NewWithBaseURL is the test hook: it returns the same config as New but
// pointed at a mock server URL so update_test.go can assert the full
// CheckForUpdate flow without hitting api.github.com.
func NewWithBaseURL(version, baseURL, cacheDir string) tuikit.UpdateConfig {
	cfg := New(version)
	cfg.APIBaseURL = baseURL
	cfg.CacheDir = cacheDir
	return cfg
}
