package ui

import (
	"fmt"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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

// eventCache stores last successful events per repo for fallback.
var (
	eventCacheMu sync.Mutex
	eventCache   = make(map[string][]github.Event)
)

// fetchWithRetries fetches events with up to 3 retries and exponential backoff.
func fetchWithRetries(repo string, limit, page int) (*github.FetchResult, error) {
	var lastErr error
	backoff := 500 * time.Millisecond
	for attempt := 0; attempt < 3; attempt++ {
		result, err := github.FetchEvents(repo, limit, page)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if attempt < 2 {
			time.Sleep(backoff)
			backoff *= 2
		}
	}
	return nil, lastErr
}

func pollEvents(cfg *config.Config, debugLog *DebugLog, initial bool) tea.Cmd {
	return func() tea.Msg {
		type result struct {
			events []github.Event
			errs   []string
		}

		repos := cfg.Repos()
		var wg sync.WaitGroup
		results := make([]result, len(repos))

		pages := 1
		if initial {
			pages = 2
		}

		var rlMu sync.Mutex
		latestRL := github.RateLimit{}

		for idx, repo := range repos {
			wg.Add(1)
			go func(i int, r string) {
				defer wg.Done()
				var allEvents []github.Event
				var errs []string
				fetchFailed := false
				notModifiedCount := 0

				for page := 1; page <= pages; page++ {
					fr, err := fetchWithRetries(r, 30, page)
					if err != nil {
						errs = append(errs, fmt.Sprintf("%s page %d: %v (3 retries exhausted)", r, page, err))
						fetchFailed = true
						continue
					}

					if fr.RateLimit > 0 {
						rlMu.Lock()
						latestRL = github.RateLimit{Remaining: fr.RateRemain, Limit: fr.RateLimit}
						rlMu.Unlock()
					}

					if fr.NotModified {
						notModifiedCount++
						debugLog.Info("304 Not Modified for %s (page %d) — no rate limit cost", r, page)
						continue
					}

					allEvents = append(allEvents, fr.Events...)
					debugLog.Info("Fetched %d events from %s (page %d)", len(fr.Events), r, page)
				}

				if notModifiedCount == pages && !fetchFailed {
					eventCacheMu.Lock()
					cached := eventCache[r]
					eventCacheMu.Unlock()
					if len(cached) > 0 {
						debugLog.RecordFetch(r, true, len(cached), false)
						results[i] = result{events: cached}
						return
					}
				}

				if len(allEvents) == 0 && fetchFailed {
					eventCacheMu.Lock()
					cached := eventCache[r]
					eventCacheMu.Unlock()
					if len(cached) > 0 {
						debugLog.Warn("Using cached events for %s (%d events)", r, len(cached))
						debugLog.RecordFetch(r, false, 0, true)
						results[i] = result{events: cached, errs: errs}
						return
					}
					debugLog.RecordFetch(r, false, 0, false)
					results[i] = result{errs: errs}
					return
				}

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

				eventCacheMu.Lock()
				eventCache[r] = deduped
				eventCacheMu.Unlock()

				debugLog.RecordFetch(r, true, len(deduped), false)

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

				results[i] = result{events: deduped, errs: errs}
			}(idx, repo)
		}
		wg.Wait()

		var all []github.Event
		var allErrors []string
		for _, r := range results {
			all = append(all, r.events...)
			allErrors = append(allErrors, r.errs...)
		}

		if latestRL.Limit > 0 {
			debugLog.SetRateLimit(latestRL.Remaining, latestRL.Limit)
		}

		return eventsMsg{events: all, errors: allErrors}
	}
}

func discoverRepos(cfg *config.Config) tea.Cmd {
	return func() tea.Msg {
		repos := discovery.Discover(cfg.Repos(), cfg.ExplicitPaths())
		return discoveryMsg{repos: repos}
	}
}

func pollGitStatus(repos []discovery.LocalRepo) tea.Cmd {
	return func() tea.Msg {
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
