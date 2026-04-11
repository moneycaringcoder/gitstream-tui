package ui

import (
	"os/exec"
	"sync"

	blit "github.com/blitui/blit"
	"github.com/moneycaringcoder/gitstream-tui/internal/config"
	"github.com/moneycaringcoder/gitstream-tui/internal/discovery"
	"github.com/moneycaringcoder/gitstream-tui/internal/github"
	"github.com/moneycaringcoder/gitstream-tui/internal/gitstatus"
)

// Messages for async data flow.
type eventsMsg struct {
	events []github.Event
	errors []string
}

type discoveryMsg struct {
	repos []discovery.LocalRepo
}

type gitStatusMsg struct {
	statuses []gitstatus.RepoStatus
}

// githubToken resolves a GitHub API token via `gh auth token` or
// the GITHUB_TOKEN environment variable. Returns "" if unavailable.
func githubToken() string {
	// Try gh CLI first
	out, err := exec.Command("gh", "auth", "token").Output()
	if err == nil {
		if token := trimNewline(string(out)); token != "" {
			return token
		}
	}
	return ""
}

func trimNewline(s string) string {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '\n' || s[i] == '\r' {
			continue
		}
		return s[:i+1]
	}
	return ""
}

// pollEvents uses blit.HTTPResource to fetch GitHub events with ETag caching,
// rate-limit tracking, and response fallback.
func pollEvents(cfg *config.Config, debugLog *DebugLog, initial bool) blit.Cmd {
	return func() blit.Msg {
		token := githubToken()
		repos := cfg.Repos()
		pages := 1
		if initial {
			pages = 2
		}

		var wg sync.WaitGroup
		type result struct {
			events []github.Event
			errs   []string
		}
		results := make([]result, len(repos))

		for idx, repo := range repos {
			wg.Add(1)
			go func(i int, r string) {
				defer wg.Done()

				hr := blit.NewHTTPResource(blit.HTTPResourceOpts{
					Name:  r,
					Pages: pages,
					BuildURL: func(page int) string {
						return blit.GitHubAPIURL("repos/"+r+"/events", 30, page)
					},
					Parse:         blit.ParseJSONSlice[github.Event](),
					ExtraHeaders: func() map[string]string {
						if token != "" {
							return map[string]string{
								"Authorization": "Bearer " + token,
								"Accept":        "application/vnd.github+json",
							}
						}
						return map[string]string{
							"Accept": "application/vnd.github+json",
						}
					},
					CacheResponses: true,
					Parallel:       true,
					OnRateLimit: func(remaining, limit int) {
						debugLog.SetRateLimit(remaining, limit)
					},
				})
				hr.SetStatsCollector(debugLog.Stats())

				msg := hr.PollCmd()()

				httpMsg := msg.(blit.HTTPResultMsg)
				var allEvents []github.Event
				for _, res := range httpMsg.Results {
					if slice, ok := res.([]github.Event); ok {
						allEvents = append(allEvents, slice...)
					}
				}

				if httpMsg.IsAllNotModified() && len(allEvents) == 0 {
					// All pages 304 — stats collector already recorded cached
					results[i] = result{}
					return
				}

				if len(httpMsg.Errors) > 0 && len(allEvents) == 0 {
					results[i] = result{errs: httpMsg.Errors}
					return
				}

				// Deduplicate by event ID
				seen := make(map[string]bool)
				var deduped []github.Event
				for _, ev := range allEvents {
					if !seen[ev.ID] {
						seen[ev.ID] = true
						deduped = append(deduped, ev)
					}
				}
				if len(deduped) > 50 {
					deduped = deduped[:50]
				}

				// Enrich push events with compare data
				var ewg sync.WaitGroup
				for j := range deduped {
					if deduped[j].Type == "PushEvent" {
						ewg.Add(1)
						go func(e *github.Event) {
							defer ewg.Done()
							github.EnrichPushEvent(e)
						}(&deduped[j])
					}
				}
				ewg.Wait()

				results[i] = result{events: deduped, errs: httpMsg.Errors}
			}(idx, repo)
		}
		wg.Wait()

		var all []github.Event
		var allErrors []string
		for _, r := range results {
			all = append(all, r.events...)
			allErrors = append(allErrors, r.errs...)
		}

		return eventsMsg{events: all, errors: allErrors}
	}
}

func discoverRepos(cfg *config.Config) blit.Cmd {
	return func() blit.Msg {
		repos := discovery.Discover(cfg.Repos(), cfg.ExplicitPaths())
		return discoveryMsg{repos: repos}
	}
}

func pollGitStatus(repos []discovery.LocalRepo) blit.Cmd {
	return func() blit.Msg {
		var wg sync.WaitGroup
		statuses := make([]gitstatus.RepoStatus, len(repos))
		for i, r := range repos {
			wg.Add(1)
			go func(idx int, repo discovery.LocalRepo) {
				defer wg.Done()
				statuses[idx] = gitstatus.Check(repo.Remote, repo.Path)
			}(i, r)
		}
		wg.Wait()
		return gitStatusMsg{statuses: statuses}
	}
}
