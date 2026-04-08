package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tuikit "github.com/moneycaringcoder/tuikit-go"
	"github.com/moneycaringcoder/gitstream-tui/internal/config"
	"github.com/moneycaringcoder/gitstream-tui/internal/discovery"
	"github.com/moneycaringcoder/gitstream-tui/internal/github"
)

// typeFilters is the cycle order for the 't' key.
var typeFilters = []string{
	"", // all
	"LocalPushEvent",
	"PushEvent",
	"PullRequestEvent",
	"PullRequestReviewEvent",
	"IssueCommentEvent",
	"IssuesEvent",
	"CreateEvent",
	"DeleteEvent",
	"ReleaseEvent",
}

// EventStream displays a scrollable list of GitHub events.
// Implements tuikit.Component.
type EventStream struct {
	cfg      *config.Config
	debugLog *DebugLog

	allEvents     []DisplayEvent
	seen          map[string]bool
	seenLocalSHAs map[string]bool

	listView *tuikit.ListView[DisplayEvent]
	poller   *tuikit.Poller

	filter     string // repo name filter
	typeFilter string // event type filter
	newestFirst bool

	localRepos []discovery.LocalRepo
	knownRepos []string // track repo list for config change detection

	focused bool
	width   int

	DetailOverlay *tuikit.DetailOverlay[DisplayEvent]
}

func NewEventStream(cfg *config.Config, debugLog *DebugLog) *EventStream {
	s := &EventStream{
		cfg:           cfg,
		debugLog:      debugLog,
		seen:          make(map[string]bool),
		seenLocalSHAs: make(map[string]bool),
		allEvents:     make([]DisplayEvent, 0, 256),
		knownRepos:    append([]string{}, cfg.Repos()...),
	}

	s.listView = tuikit.NewListView(tuikit.ListViewOpts[DisplayEvent]{
		RenderItem: func(item DisplayEvent, idx int, isCursor bool, theme tuikit.Theme) string {
			return renderEventLine(item.Event, time.Now())
		},
		HeaderFunc: func(theme tuikit.Theme) string {
			return s.renderHeader()
		},
		DetailFunc: func(item DisplayEvent, theme tuikit.Theme) string {
			return s.renderDetailBar(item)
		},
		FlashFunc: func(item DisplayEvent, now time.Time) bool {
			return !item.AddedAt.IsZero() && now.Before(item.AddedAt.Add(flashDuration))
		},
		DetailHeight: 3,
	})

	s.poller = tuikit.NewPoller(
		time.Duration(cfg.Interval)*time.Second,
		func() tea.Cmd {
			cmds := []tea.Cmd{pollEvents(cfg, debugLog, false)}
			if len(s.localRepos) > 0 {
				cmds = append(cmds, pollGitStatus(s.localRepos))
			}
			return tea.Batch(cmds...)
		},
	)

	return s
}

func (s *EventStream) Init() tea.Cmd {
	return tea.Batch(
		pollEvents(s.cfg, s.debugLog, true),
		discoverRepos(s.cfg),
	)
}

func (s *EventStream) Update(msg tea.Msg) (tuikit.Component, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Enter opens detail overlay instead of browser
		if msg.String() == "enter" && s.DetailOverlay != nil {
			if item := s.listView.CursorItem(); item != nil {
				s.DetailOverlay.Show(*item)
				return s, tuikit.Consumed()
			}
		}
		// 'o' opens in browser
		if msg.String() == "o" {
			if item := s.listView.CursorItem(); item != nil {
				if url := item.Event.URL(); url != "" {
					OpenURL(url)
				}
				return s, tuikit.Consumed()
			}
		}
		cmd := s.listView.HandleKey(msg)
		return s, cmd

	case tuikit.TickMsg:
		s.listView.Refresh()
		s.poller.SetInterval(time.Duration(s.cfg.Interval) * time.Second)

		// Check if repos changed (config editor modified them)
		currentRepos := s.cfg.Repos()
		if !slicesEqual(currentRepos, s.knownRepos) {
			s.knownRepos = append([]string{}, currentRepos...)
			return s, discoverRepos(s.cfg)
		}

		// Periodic data polling
		cmd := s.poller.Check(msg)
		return s, cmd

	case eventsMsg:
		return s.handleEvents(msg)

	case discoveryMsg:
		s.localRepos = msg.repos
		if len(s.localRepos) > 0 {
			return s, pollGitStatus(s.localRepos)
		}
		return s, nil

	case gitStatusMsg:
		return s.handleGitStatus(msg)
	}

	return s, nil
}

func (s *EventStream) handleEvents(msg eventsMsg) (tuikit.Component, tea.Cmd) {
	for _, e := range msg.errors {
		s.debugLog.Error("%s", e)
	}

	now := time.Now()
	recentThreshold := time.Duration(s.cfg.Interval) * time.Second * 2
	newCount := 0
	firstPoll := len(s.seen) == 0
	for _, ev := range msg.events {
		if s.seen[ev.ID] {
			continue
		}
		s.seen[ev.ID] = true
		addedAt := time.Time{}
		if firstPoll && now.Sub(ev.CreatedAt) < recentThreshold {
			addedAt = now
		}
		s.allEvents = append(s.allEvents, DisplayEvent{Event: ev, AddedAt: addedAt})
		newCount++
	}
	if newCount > 0 {
		sort.Slice(s.allEvents, func(i, j int) bool {
			return s.allEvents[i].Event.CreatedAt.Before(s.allEvents[j].Event.CreatedAt)
		})
		atEdge := s.isAtNewEdge()
		s.rebuildFiltered()
		if atEdge {
			if s.newestFirst {
				s.listView.ScrollToTop()
			} else {
				s.listView.ScrollToBottom()
			}
		}
	}
	return s, nil
}

func (s *EventStream) handleGitStatus(msg gitStatusMsg) (tuikit.Component, tea.Cmd) {
	now := time.Now()
	newLocal := 0
	for _, st := range msg.statuses {
		for _, c := range st.UnpushedCommits {
			key := st.Remote + ":" + c.SHA
			if s.seenLocalSHAs[key] {
				continue
			}
			s.seenLocalSHAs[key] = true
			commitTime, _ := time.Parse(time.RFC3339, c.Date)
			if commitTime.IsZero() {
				commitTime = now
			}
			ev := github.Event{
				ID:   "local-" + key,
				Type: "LocalPushEvent",
				Actor: github.Actor{Login: c.Author},
				Repo:  github.Repo{Name: st.Remote},
				Payload: github.Payload{
					Ref:     st.Branch,
					Commits: []github.Commit{{SHA: c.SHA, Message: c.Message}},
				},
				CreatedAt: commitTime,
			}
			addedAt := time.Time{}
			if len(s.seen) > 0 && now.Sub(commitTime) < 2*time.Minute {
				addedAt = now
			}
			s.allEvents = append(s.allEvents, DisplayEvent{Event: ev, AddedAt: addedAt})
			newLocal++
		}
	}
	if newLocal > 0 {
		sort.Slice(s.allEvents, func(i, j int) bool {
			return s.allEvents[i].Event.CreatedAt.Before(s.allEvents[j].Event.CreatedAt)
		})
		atEdge := s.isAtNewEdge()
		s.rebuildFiltered()
		if atEdge {
			if s.newestFirst {
				s.listView.ScrollToTop()
			} else {
				s.listView.ScrollToBottom()
			}
		}
	}
	return s, nil
}

func (s *EventStream) View() string {
	return s.listView.View()
}

func (s *EventStream) renderHeader() string {
	title := TitleStyle.Render("gitstream")
	repoList := SubtitleStyle.Render(fmt.Sprintf("Watching: %s", strings.Join(s.cfg.Repos(), ", ")))

	// Status line: poll info + health dots + rate limit
	var statusParts []string
	if s.poller.IsPaused() {
		statusParts = append(statusParts, lipgloss.NewStyle().Foreground(lipgloss.Color("#eab308")).Render("[PAUSED]"))
	} else if !s.poller.LastPoll().IsZero() {
		ago := time.Since(s.poller.LastPoll()).Truncate(time.Second)
		statusParts = append(statusParts, fmt.Sprintf("Poll %s ago", ago))
	}
	stats := s.debugLog.GetStats()
	for _, repo := range s.cfg.Repos() {
		short := repo
		if i := strings.LastIndex(repo, "/"); i >= 0 {
			short = repo[i+1:]
		}
		dot := lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280")).Render("○")
		if h, ok := stats.RepoHealth[repo]; ok {
			if h.LastSuccess {
				dot = lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e")).Render("●")
			} else if h.UsingCache && h.FailStreak < 10 {
				dot = lipgloss.NewStyle().Foreground(lipgloss.Color("#eab308")).Render("●")
			} else {
				dot = lipgloss.NewStyle().Foreground(lipgloss.Color("#ef4444")).Render("●")
			}
		}
		statusParts = append(statusParts, dot+" "+short)
	}
	if stats.RateLimit > 0 {
		ratePct := float64(stats.RateRemain) / float64(stats.RateLimit) * 100
		rateColor := lipgloss.Color("#22c55e")
		if ratePct < 20 {
			rateColor = lipgloss.Color("#ef4444")
		} else if ratePct < 50 {
			rateColor = lipgloss.Color("#eab308")
		}
		statusParts = append(statusParts, lipgloss.NewStyle().Foreground(rateColor).Render(
			fmt.Sprintf("API %d/%d", stats.RateRemain, stats.RateLimit)))
	}
	status := SubtitleStyle.Render(strings.Join(statusParts, "  "))

	return lipgloss.JoinVertical(lipgloss.Left, title, repoList, status, "")
}

func (s *EventStream) renderDetailBar(de DisplayEvent) string {
	ev := de.Event
	divider := DividerStyle.Render(strings.Repeat("─", s.width))

	repo := ev.Repo.Name
	label := ev.Label()
	actor := ev.Actor.Login
	t := ev.CreatedAt.Local().Format("2006-01-02 15:04:05")
	color := EventColor(ev.Type)

	line1 := fmt.Sprintf(" %s  %s  %s  %s",
		DetailLabelStyle(color).Render(label),
		DetailRepoStyle.Render(repo),
		DetailActorStyle.Render(actor),
		DetailTimeStyle.Render(t),
	)

	detail := ev.Detail()
	maxDetail := s.width - 20
	if maxDetail < 20 {
		maxDetail = 20
	}
	if len(detail) > maxDetail {
		detail = detail[:maxDetail-1] + "…"
	}
	urlHint := ""
	if url := ev.URL(); url != "" {
		urlHint = DetailTimeStyle.Render("  ↵ open")
	}
	line2 := " " + DetailStyle.Render(detail) + urlHint

	return divider + "\n" + line1 + "\n" + line2
}

func (s *EventStream) KeyBindings() []tuikit.KeyBind {
	bindings := s.listView.KeyBindings()
	bindings = append(bindings,
		tuikit.KeyBind{Key: "enter", Label: "Event detail", Group: "NAVIGATION"},
		tuikit.KeyBind{Key: "o", Label: "Open in browser", Group: "NAVIGATION"},
	)
	return bindings
}

func (s *EventStream) SetSize(w, h int) {
	s.width = w
	s.listView.SetSize(w, h)
}

func (s *EventStream) Focused() bool       { return s.focused }
func (s *EventStream) SetFocused(f bool) {
	s.focused = f
	s.listView.SetFocused(f)
}

// Public methods for app-level keybinding handlers.

func (s *EventStream) SetRepoFilter(repo string) {
	if s.filter == repo {
		s.filter = ""
	} else {
		s.filter = repo
	}
	s.rebuildFiltered()
}

func (s *EventStream) ClearFilters() {
	s.filter = ""
	s.typeFilter = ""
	s.rebuildFiltered()
}

func (s *EventStream) CycleTypeFilter(forward bool) {
	cur := 0
	for i, t := range typeFilters {
		if t == s.typeFilter {
			cur = i
			break
		}
	}
	if forward {
		s.typeFilter = typeFilters[(cur+1)%len(typeFilters)]
	} else {
		s.typeFilter = typeFilters[(cur-1+len(typeFilters))%len(typeFilters)]
	}
	s.rebuildFiltered()
}

func (s *EventStream) ToggleSort() {
	s.newestFirst = !s.newestFirst
	s.rebuildFiltered()
	if s.newestFirst {
		s.listView.ScrollToTop()
	} else {
		s.listView.ScrollToBottom()
	}
}

func (s *EventStream) TogglePause()   { s.poller.TogglePause() }
func (s *EventStream) ForceRefresh()  { s.poller.ForceRefresh() }
func (s *EventStream) IsPaused() bool { return s.poller.IsPaused() }
func (s *EventStream) IsNewestFirst() bool { return s.newestFirst }
func (s *EventStream) RepoFilter() string  { return s.filter }
func (s *EventStream) TypeFilter() string  { return s.typeFilter }
func (s *EventStream) DebugLog() *DebugLog { return s.debugLog }
func (s *EventStream) HasLocalRepos() bool { return len(s.localRepos) > 0 }

// Internal methods.

func (s *EventStream) skipEvent(de DisplayEvent) bool {
	if s.filter != "" && de.Event.ShortRepo() != s.filter {
		return true
	}
	if s.typeFilter != "" && de.Event.Type != s.typeFilter {
		return true
	}
	return false
}

func (s *EventStream) rebuildFiltered() {
	var filtered []DisplayEvent
	if s.newestFirst {
		for i := len(s.allEvents) - 1; i >= 0; i-- {
			if !s.skipEvent(s.allEvents[i]) {
				filtered = append(filtered, s.allEvents[i])
			}
		}
	} else {
		for _, de := range s.allEvents {
			if !s.skipEvent(de) {
				filtered = append(filtered, de)
			}
		}
	}
	s.listView.SetItems(filtered)
}

func (s *EventStream) isAtNewEdge() bool {
	if s.newestFirst {
		return s.listView.IsAtTop()
	}
	return s.listView.IsAtBottom()
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
