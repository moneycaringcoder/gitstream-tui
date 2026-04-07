package discovery

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// LocalRepo maps a watched remote repo to its local clone path.
type LocalRepo struct {
	Remote string // owner/repo
	Path   string // absolute local path
}

// defaultScanDirs returns common directories where repos might live.
func defaultScanDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	candidates := []string{
		filepath.Join(home, "Documents", "GitHub"),
		filepath.Join(home, "Projects"),
		filepath.Join(home, "repos"),
		filepath.Join(home, "src"),
		filepath.Join(home, "code"),
		filepath.Join(home, "dev"),
	}

	// Also check cwd
	if cwd, err := os.Getwd(); err == nil {
		candidates = append([]string{cwd}, candidates...)
	}

	var dirs []string
	for _, d := range candidates {
		if info, err := os.Stat(d); err == nil && info.IsDir() {
			dirs = append(dirs, d)
		}
	}
	return dirs
}

// remoteForPath returns the owner/repo for a git repo at the given path,
// extracted from the origin remote URL.
func remoteForPath(path string) string {
	cmd := exec.Command("git", "-C", path, "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return extractOwnerRepo(strings.TrimSpace(string(out)))
}

// extractOwnerRepo parses owner/repo from various git remote URL formats.
func extractOwnerRepo(url string) string {
	// Handle SSH: git@github.com:owner/repo.git
	if strings.HasPrefix(url, "git@") {
		if i := strings.Index(url, ":"); i >= 0 {
			path := url[i+1:]
			path = strings.TrimSuffix(path, ".git")
			return path
		}
	}
	// Handle HTTPS: https://github.com/owner/repo.git
	url = strings.TrimSuffix(url, ".git")
	parts := strings.Split(url, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-2] + "/" + parts[len(parts)-1]
	}
	return ""
}

// Discover finds local clones for the given watched repos.
// explicitPaths is a map of remote -> local path from config overrides.
func Discover(watchedRepos []string, explicitPaths map[string]string) []LocalRepo {
	wanted := make(map[string]bool, len(watchedRepos))
	for _, r := range watchedRepos {
		wanted[r] = true
	}

	found := make(map[string]string) // remote -> path

	// Apply explicit overrides first
	for remote, path := range explicitPaths {
		if wanted[remote] {
			found[remote] = path
		}
	}

	// If all repos have explicit paths, skip scanning
	if len(found) == len(watchedRepos) {
		return toResult(found)
	}

	// Scan default directories
	scanDirs := defaultScanDirs()
	var mu sync.Mutex
	var wg sync.WaitGroup

	checkCandidate := func(path string) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			remote := remoteForPath(path)
			if remote == "" || !wanted[remote] {
				return
			}
			mu.Lock()
			if _, exists := found[remote]; !exists {
				found[remote] = path
			}
			mu.Unlock()
		}()
	}

	for _, dir := range scanDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			candidate := filepath.Join(dir, entry.Name())

			// .git can be a directory (normal repo) or a file (worktree)
			if _, err := os.Stat(filepath.Join(candidate, ".git")); err == nil {
				checkCandidate(candidate)
				continue
			}

			// Scan one level deeper for worktrees nested under a parent dir
			subEntries, err := os.ReadDir(candidate)
			if err != nil {
				continue
			}
			for _, sub := range subEntries {
				if !sub.IsDir() {
					continue
				}
				subCandidate := filepath.Join(candidate, sub.Name())
				if _, err := os.Stat(filepath.Join(subCandidate, ".git")); err == nil {
					checkCandidate(subCandidate)
				}
			}
		}
	}
	wg.Wait()

	return toResult(found)
}

func toResult(found map[string]string) []LocalRepo {
	var result []LocalRepo
	for remote, path := range found {
		result = append(result, LocalRepo{Remote: remote, Path: path})
	}
	return result
}
