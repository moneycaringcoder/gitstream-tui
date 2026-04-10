package github

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Event struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	Actor       Actor          `json:"actor"`
	Repo        Repo           `json:"repo"`
	Payload     Payload        `json:"payload"`
	CreatedAt   time.Time      `json:"created_at"`
	CompareData *CompareResult `json:"-"`
}

type Actor struct {
	Login string `json:"login"`
}

type Repo struct {
	Name string `json:"name"`
}

type Payload struct {
	Action      string       `json:"action,omitempty"`
	Ref         string       `json:"ref,omitempty"`
	RefType     string       `json:"ref_type,omitempty"`
	Size        int          `json:"size,omitempty"`
	Before      string       `json:"before,omitempty"`
	Head        string       `json:"head,omitempty"`
	Commits     []Commit     `json:"commits,omitempty"`
	PullRequest *PullRequest `json:"pull_request,omitempty"`
	Issue       *Issue       `json:"issue,omitempty"`
	Comment     *Comment     `json:"comment,omitempty"`
	Release     *Release     `json:"release,omitempty"`
}

type Commit struct {
	SHA     string `json:"sha"`
	Message string `json:"message"`
}

type PullRequest struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
	Body   string `json:"body"`
}

type Issue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
}

type Comment struct {
	Body string `json:"body"`
}

type Release struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Body    string `json:"body"`
}

// CompareResult holds diff stats from the compare API.
type CompareResult struct {
	TotalCommits int `json:"total_commits"`
	Files        []struct {
		Filename  string `json:"filename"`
		Additions int    `json:"additions"`
		Deletions int    `json:"deletions"`
		Changes   int    `json:"changes"`
	} `json:"files"`
}

// FetchCompare gets diff stats between two commits.
func FetchCompare(repo, base, head string) (*CompareResult, error) {
	cmd := exec.Command("gh", "api", fmt.Sprintf("repos/%s/compare/%s...%s", repo, base, head),
		"--jq", `{total_commits: .total_commits, files: [.files[] | {filename, additions, deletions, changes}]}`)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var result CompareResult
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// etagCache stores ETags per URL for conditional requests.
var (
	etagMu    sync.Mutex
	etagStore = make(map[string]string) // url -> etag
)

// FetchResult holds the outcome of an ETag-aware fetch.
type FetchResult struct {
	Events      []Event
	NotModified bool // true if 304 — data unchanged, didn't cost a rate limit point
	RateRemain  int  // parsed from X-RateLimit-Remaining header
	RateLimit   int  // parsed from X-RateLimit-Limit header
}

// FetchEvents fetches recent events for a repo using the gh CLI with ETag support.
// page is 1-indexed; each page returns up to 30 events from the API.
// When the server returns 304 Not Modified, NotModified is true and Events is nil.
func FetchEvents(repo string, limit int, page int) (*FetchResult, error) {
	if page < 1 {
		page = 1
	}
	url := fmt.Sprintf("repos/%s/events?per_page=30&page=%d", repo, page)

	args := []string{"api", url, "--include", "--cache", "0s"}

	// Add ETag header if we have one cached
	etagMu.Lock()
	etag := etagStore[url]
	etagMu.Unlock()
	if etag != "" {
		args = append(args, "-H", fmt.Sprintf("If-None-Match: %s", etag))
	}

	cmd := exec.Command("gh", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		outStr := string(out)
		// gh exits non-zero on 304 — check if it's a Not Modified response
		if strings.Contains(outStr, "304 Not Modified") || strings.Contains(outStr, "HTTP/2.0 304") {
			rl := parseRateLimitHeaders(outStr)
			return &FetchResult{NotModified: true, RateRemain: rl.Remaining, RateLimit: rl.Limit}, nil
		}
		return nil, fmt.Errorf("gh api failed for %s (page %d): %w", repo, page, err)
	}

	// Parse headers and body from --include output
	outStr := string(out)
	headerEnd, body := splitHeaderBody(outStr)

	// Extract and cache the ETag
	if newEtag := parseHeader(headerEnd, "ETag"); newEtag != "" {
		etagMu.Lock()
		etagStore[url] = newEtag
		etagMu.Unlock()
	}

	// Parse rate limit from headers
	rl := parseRateLimitHeaders(headerEnd)

	var events []Event
	if err := json.Unmarshal([]byte(body), &events); err != nil {
		return nil, fmt.Errorf("json parse failed for %s: %w", repo, err)
	}

	if len(events) > limit {
		events = events[:limit]
	}

	return &FetchResult{Events: events, RateRemain: rl.Remaining, RateLimit: rl.Limit}, nil
}

// splitHeaderBody splits `gh api --include` output into headers and JSON body.
func splitHeaderBody(raw string) (headers string, body string) {
	// gh --include outputs: HTTP status line, headers, blank line, then JSON body
	// Find the first '{' or '[' that starts the JSON body
	for i, ch := range raw {
		if ch == '[' || ch == '{' {
			return raw[:i], raw[i:]
		}
	}
	return raw, ""
}

// parseHeader extracts a header value from raw header text.
func parseHeader(headers, name string) string {
	lower := strings.ToLower(name)
	for _, line := range strings.Split(headers, "\n") {
		if idx := strings.Index(line, ":"); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			if strings.ToLower(key) == lower {
				return strings.TrimSpace(line[idx+1:])
			}
		}
	}
	return ""
}

// RateLimit holds GitHub API rate limit info.
type RateLimit struct {
	Remaining int
	Limit     int
}

func parseRateLimitHeaders(headers string) RateLimit {
	var rl RateLimit
	if v := parseHeader(headers, "X-RateLimit-Remaining"); v != "" {
		rl.Remaining, _ = strconv.Atoi(v)
	}
	if v := parseHeader(headers, "X-RateLimit-Limit"); v != "" {
		rl.Limit, _ = strconv.Atoi(v)
	}
	return rl
}

// FetchRateLimit queries the GitHub API rate limit.
func FetchRateLimit() (*RateLimit, error) {
	cmd := exec.Command("gh", "api", "rate_limit", "--jq", ".rate | {remaining, limit}")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var rl RateLimit
	if err := json.Unmarshal(out, &rl); err != nil {
		return nil, err
	}
	return &rl, nil
}

// EnrichPushEvent fetches compare stats and populates the event's detail cache.
func EnrichPushEvent(ev *Event) {
	if ev.Type != "PushEvent" || ev.Payload.Before == "" || ev.Payload.Head == "" {
		return
	}
	// Skip if before is all zeros (new branch)
	if ev.Payload.Before == "0000000000000000000000000000000000000000" {
		return
	}
	result, err := FetchCompare(ev.Repo.Name, ev.Payload.Before, ev.Payload.Head)
	if err != nil {
		return
	}
	ev.CompareData = result
}


// Label returns a short human-readable label for an event type.
func (e *Event) Label() string {
	switch e.Type {
	case "LocalPushEvent":
		return "LOCAL"
	case "PushEvent":
		return "PUSH"
	case "PullRequestEvent":
		return "PR"
	case "PullRequestReviewEvent":
		return "REVIEW"
	case "PullRequestReviewCommentEvent":
		return "COMMENT"
	case "IssueCommentEvent":
		return "COMMENT"
	case "IssuesEvent":
		return "ISSUE"
	case "CreateEvent":
		return "CREATE"
	case "DeleteEvent":
		return "DELETE"
	case "WatchEvent":
		return "STAR"
	case "ForkEvent":
		return "FORK"
	case "ReleaseEvent":
		return "RELEASE"
	case "MemberEvent":
		return "MEMBER"
	case "GollumEvent":
		return "WIKI"
	case "PublicEvent":
		return "PUBLIC"
	case "SponsorshipEvent":
		return "SPONSOR"
	default:
		label := strings.TrimSuffix(e.Type, "Event")
		if len(label) > 8 {
			label = label[:8]
		}
		return strings.ToUpper(label)
	}
}

// Detail returns a human-readable description of the event payload.
func (e *Event) Detail() string {
	p := e.Payload
	switch e.Type {
	case "LocalPushEvent":
		msg := ""
		if len(p.Commits) > 0 {
			msg = p.Commits[0].Message
		}
		ref := p.Ref
		if ref == "" {
			ref = "unpushed"
		}
		return fmt.Sprintf("↑ %s — %s", ref, msg)
	case "PushEvent":
		ref := p.Ref
		if len(ref) > 11 && ref[:11] == "refs/heads/" {
			ref = ref[11:]
		}
		if e.CompareData != nil {
			c := e.CompareData
			adds, dels := 0, 0
			for _, f := range c.Files {
				adds += f.Additions
				dels += f.Deletions
			}
			return fmt.Sprintf("%d commit(s) to %s  [%d files +%d -%d]",
				c.TotalCommits, ref, len(c.Files), adds, dels)
		}
		if p.Size > 0 {
			return fmt.Sprintf("%d commit(s) to %s", p.Size, ref)
		}
		return fmt.Sprintf("pushed to %s", ref)
	case "PullRequestEvent":
		if p.PullRequest != nil {
			return fmt.Sprintf("%s #%d: %s", p.Action, p.PullRequest.Number, p.PullRequest.Title)
		}
	case "PullRequestReviewEvent", "PullRequestReviewCommentEvent":
		if p.PullRequest != nil {
			return fmt.Sprintf("%s on #%d", p.Action, p.PullRequest.Number)
		}
	case "IssueCommentEvent":
		if p.Issue != nil {
			return fmt.Sprintf("%s on #%d", p.Action, p.Issue.Number)
		}
	case "IssuesEvent":
		if p.Issue != nil {
			return fmt.Sprintf("%s #%d: %s", p.Action, p.Issue.Number, p.Issue.Title)
		}
	case "CreateEvent":
		return fmt.Sprintf("%s %s", p.RefType, p.Ref)
	case "DeleteEvent":
		return fmt.Sprintf("%s %s", p.RefType, p.Ref)
	case "ReleaseEvent":
		if p.Release != nil {
			return fmt.Sprintf("%s %s", p.Action, p.Release.TagName)
		}
	case "MemberEvent":
		return p.Action
	}
	if p.Action != "" {
		return p.Action
	}
	return ""
}

// URL returns a GitHub URL for the event, if applicable.
func (e *Event) URL() string {
	base := "https://github.com/" + e.Repo.Name
	p := e.Payload
	switch e.Type {
	case "PushEvent":
		if p.Head != "" {
			return base + "/commit/" + p.Head
		}
	case "PullRequestEvent", "PullRequestReviewEvent", "PullRequestReviewCommentEvent":
		if p.PullRequest != nil {
			return fmt.Sprintf("%s/pull/%d", base, p.PullRequest.Number)
		}
	case "IssueCommentEvent", "IssuesEvent":
		if p.Issue != nil {
			return fmt.Sprintf("%s/issues/%d", base, p.Issue.Number)
		}
	case "ReleaseEvent":
		if p.Release != nil {
			return fmt.Sprintf("%s/releases/tag/%s", base, p.Release.TagName)
		}
	case "CreateEvent":
		if p.RefType == "branch" && p.Ref != "" {
			return fmt.Sprintf("%s/tree/%s", base, p.Ref)
		}
		if p.RefType == "tag" && p.Ref != "" {
			return fmt.Sprintf("%s/releases/tag/%s", base, p.Ref)
		}
	}
	return base
}

// ShortRepo returns just the repo name without the owner prefix.
func (e *Event) ShortRepo() string {
	name := e.Repo.Name
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '/' {
			return name[i+1:]
		}
	}
	return name
}
