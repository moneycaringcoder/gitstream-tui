package github

import (
	"encoding/json"
	"fmt"
	"os/exec"
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
}

type Issue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
}

type Comment struct {
	Body string `json:"body"`
}

type Release struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
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

// FetchEvents fetches recent events for a repo using the gh CLI.
func FetchEvents(repo string, limit int) ([]Event, error) {
	cmd := exec.Command("gh", "api", fmt.Sprintf("repos/%s/events", repo), "--cache", "0s")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh api failed for %s: %w", repo, err)
	}

	var events []Event
	if err := json.Unmarshal(out, &events); err != nil {
		return nil, fmt.Errorf("json parse failed for %s: %w", repo, err)
	}

	if len(events) > limit {
		events = events[:limit]
	}

	return events, nil
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
	default:
		return e.Type
	}
}

// Detail returns a human-readable description of the event payload.
func (e *Event) Detail() string {
	p := e.Payload
	switch e.Type {
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
	}
	return ""
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
