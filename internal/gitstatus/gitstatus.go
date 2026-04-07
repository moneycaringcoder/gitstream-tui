package gitstatus

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// LocalCommit represents an unpushed local commit.
type LocalCommit struct {
	SHA     string
	Message string
	Author  string
	Date    string // ISO format
}

// CIStatus represents the latest CI workflow run status.
type CIStatus struct {
	Status     string // "completed", "in_progress", "queued"
	Conclusion string // "success", "failure", "cancelled", etc.
	Name       string // workflow name
}

// RepoStatus holds the local git status for a repo.
type RepoStatus struct {
	Remote          string // owner/repo
	Path            string // local path
	Branch          string
	Uncommitted     int  // staged + unstaged changed files
	Unpushed        int  // commits ahead of upstream
	UnpushedCommits []LocalCommit
	HasUpstream     bool // whether the branch tracks a remote
	CI              *CIStatus
	Error           error
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

	// Unpushed commits — try upstream first, fall back to origin/main or origin/master
	compareRef := "@{u}"
	count, err := gitOutput(path, "rev-list", "--count", compareRef+"..HEAD")
	if err != nil {
		s.HasUpstream = false
		// Try origin/main, then origin/master as fallback
		for _, fallback := range []string{"origin/main", "origin/master"} {
			if _, verr := gitOutput(path, "rev-parse", "--verify", fallback); verr == nil {
				compareRef = fallback
				count, err = gitOutput(path, "rev-list", "--count", compareRef+"..HEAD")
				break
			}
		}
	} else {
		s.HasUpstream = true
	}

	if err == nil {
		s.Unpushed, _ = strconv.Atoi(count)
	}

	// Fetch unpushed commit details
	if s.Unpushed > 0 {
		s.UnpushedCommits = fetchUnpushedCommits(path, compareRef)
	}

	// CI status (non-blocking, best effort)
	s.CI = fetchCI(remote)

	return s
}

func fetchUnpushedCommits(path, compareRef string) []LocalCommit {
	// Format: SHA|subject|author|date
	out, err := gitOutput(path, "log", compareRef+"..HEAD", "--format=%H|%s|%an|%aI")
	if err != nil || out == "" {
		return nil
	}
	lines := strings.Split(out, "\n")
	commits := make([]LocalCommit, 0, len(lines))
	for _, line := range lines {
		parts := strings.SplitN(line, "|", 4)
		if len(parts) < 4 {
			continue
		}
		commits = append(commits, LocalCommit{
			SHA:     parts[0][:7], // short SHA
			Message: parts[1],
			Author:  parts[2],
			Date:    parts[3],
		})
	}
	return commits
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
