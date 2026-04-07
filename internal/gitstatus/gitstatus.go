package gitstatus

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// CIStatus represents the latest CI workflow run status.
type CIStatus struct {
	Status     string // "completed", "in_progress", "queued"
	Conclusion string // "success", "failure", "cancelled", etc.
	Name       string // workflow name
}

// RepoStatus holds the local git status for a repo.
type RepoStatus struct {
	Remote       string // owner/repo
	Path         string // local path
	Branch       string
	Uncommitted  int  // staged + unstaged changed files
	Unpushed     int  // commits ahead of upstream
	HasUpstream  bool // whether the branch tracks a remote
	CI           *CIStatus
	Error        error
}

// Check gathers the git status for a local repo at the given path.
func Check(remote, path string) RepoStatus {
	s := RepoStatus{Remote: remote, Path: path}

	// Current branch
	branch, err := gitOutput(path, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		s.Error = err
		return s
	}
	s.Branch = branch

	// Uncommitted changes (staged + unstaged)
	status, err := gitOutput(path, "status", "--porcelain")
	if err != nil {
		s.Error = err
		return s
	}
	if status != "" {
		s.Uncommitted = len(strings.Split(strings.TrimRight(status, "\n"), "\n"))
	}

	// Unpushed commits
	count, err := gitOutput(path, "rev-list", "--count", "@{u}..HEAD")
	if err != nil {
		// No upstream configured
		s.HasUpstream = false
		return s
	}
	s.HasUpstream = true
	s.Unpushed, _ = strconv.Atoi(count)

	// CI status (non-blocking, best effort)
	s.CI = fetchCI(remote)

	return s
}

func fetchCI(remote string) *CIStatus {
	cmd := exec.Command("gh", "api",
		fmt.Sprintf("repos/%s/actions/runs?per_page=1&status=completed", remote),
		"--jq", `.workflow_runs[0] | {status, conclusion, name}`)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	var ci CIStatus
	if err := json.Unmarshal([]byte(trimmed), &ci); err != nil {
		return nil
	}
	return &ci
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
