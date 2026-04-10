package updatewire

import (
	"testing"

	blit "github.com/blitui/blit"
	"github.com/blitui/blit/updatetest"
)

// TestNew_DefaultsAreForced asserts the consumer wiring uses UpdateForced
// so a minimum_version marker in release notes promotes the update gate.
func TestNew_DefaultsAreForced(t *testing.T) {
	cfg := New("v0.0.0")
	if cfg.Mode != blit.UpdateForced {
		t.Errorf("mode = %v, want UpdateForced", cfg.Mode)
	}
	if cfg.BinaryName != "gitstream" {
		t.Errorf("binary = %q, want gitstream", cfg.BinaryName)
	}
	if cfg.Owner != "moneycaringcoder" || cfg.Repo != "gitstream-tui" {
		t.Errorf("owner/repo = %s/%s", cfg.Owner, cfg.Repo)
	}
}

// TestCheckForUpdate_NewerVersionAvailable exercises the forced path end
// to end through updatetest.NewMockServer: a newer release with a
// minimum_version marker must set Required=true on the result.
func TestCheckForUpdate_NewerVersionAvailable(t *testing.T) {
	srv := updatetest.NewMockServer(updatetest.Release{
		Tag:            "v1.0.0",
		BinaryName:     "gitstream",
		Body:           "new stuff",
		MinimumVersion: "v1.0.0",
	})
	defer srv.Close()

	cfg := NewWithBaseURL("v0.5.0", srv.URL, t.TempDir())
	res, err := blit.CheckForUpdate(cfg)
	if err != nil {
		t.Fatalf("CheckForUpdate: %v", err)
	}
	if !res.Available {
		t.Error("expected Available=true for older current version")
	}
	if !res.Required {
		t.Error("expected Required=true when minimum_version marker is above current")
	}
}

// TestCheckForUpdate_CurrentVersionNoGate verifies the forced gate is NOT
// triggered when the consumer already runs the latest release.
func TestCheckForUpdate_CurrentVersionNoGate(t *testing.T) {
	srv := updatetest.NewMockServer(updatetest.Release{
		Tag:        "v1.0.0",
		BinaryName: "gitstream",
		Body:       "same version",
	})
	defer srv.Close()

	cfg := NewWithBaseURL("v1.0.0", srv.URL, t.TempDir())
	res, err := blit.CheckForUpdate(cfg)
	if err != nil {
		t.Fatalf("CheckForUpdate: %v", err)
	}
	if res.Available {
		t.Error("expected Available=false when running latest version")
	}
	if res.Required {
		t.Error("expected Required=false when running latest version")
	}
}

// TestCheckForUpdate_SkippedVersionNoGate verifies that writing a
// skipped-versions entry suppresses the gate for a non-required update.
// (Required updates from minimum_version markers override skip on
// purpose — tested in TestCheckForUpdate_NewerVersionAvailable.)
func TestCheckForUpdate_SkippedVersionNoGate(t *testing.T) {
	srv := updatetest.NewMockServer(updatetest.Release{
		Tag:        "v1.0.0",
		BinaryName: "gitstream",
		Body:       "skip me",
	})
	defer srv.Close()

	cfg := NewWithBaseURL("v0.5.0", srv.URL, t.TempDir())
	if err := blit.SkipVersion(cfg, "v1.0.0"); err != nil {
		t.Fatalf("SkipVersion: %v", err)
	}
	res, err := blit.CheckForUpdate(cfg)
	if err != nil {
		t.Fatalf("CheckForUpdate: %v", err)
	}
	if res.Available {
		t.Error("expected Available=false when target version is skipped")
	}
}

// TestCheckForUpdate_DisabledShortCircuits verifies the Disabled kill
// switch suppresses the gate even when a newer release exists.
func TestCheckForUpdate_DisabledShortCircuits(t *testing.T) {
	srv := updatetest.NewMockServer(updatetest.Release{
		Tag:            "v1.0.0",
		BinaryName:     "gitstream",
		MinimumVersion: "v1.0.0",
	})
	defer srv.Close()

	cfg := NewWithBaseURL("v0.5.0", srv.URL, t.TempDir())
	cfg.Disabled = true
	res, err := blit.CheckForUpdate(cfg)
	if err != nil {
		t.Fatalf("CheckForUpdate: %v", err)
	}
	if res.Available || res.Required {
		t.Errorf("expected disabled short-circuit, got %+v", res)
	}
}
