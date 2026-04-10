package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	blit "github.com/blitui/blit"
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

// EventStream displays a scrollable table of GitHub events.
// Implements blit.Component.
type EventStream struct {
	cfg      *config.Config
	debugLog *DebugLog

	allEvents      []DisplayEvent
	filteredEvents []DisplayEvent // parallel slice for row-click lookup
	seen           map[string]bool
	seenLocalSHAs  map[string]bool

	table  *blit.Table
	poller *blit.Poller

	filter     string // repo name filter
	typeFilter string // event type filter
	newestFirst bool

	localRepos []discovery.LocalRepo
	knownRepos []string // track repo list for config change detection

	focused bool
	width   int

	epmWindow       []float64  // events-per-minute rolling window (last 30 points)
	lastEPMTick     time.Time
	currentMinCount int

	DetailOverlay *blit.DetailOverlay[DisplayEvent]
}

func NewEventStream(cfg *config.Config, debugLog *DebugLog) *EventStream {
	s := &EventStream{
		cfg:            cfg,
		debugLog:       debugLog,
		seen:           make(map[string]bool),
		seenLocalSHAs:  make(map[string]bool),
		allEvents:      make([]DisplayEvent, 0, 256),
		filteredEvents: make([]DisplayEvent, 0, 256),
		knownRepos:     append([]string{}, cfg.Repos()...),
	}

	columns := []blit.Column{
		{Title: "Time", Width: 2, MaxWidth: 20},
		{Title: "Repo", Width: 2, MaxWidth: 18},
		{Title: "Type", Width: 1, MaxWidth: 10},
		{Title: "Actor", Width: 2, MaxWidth: 22},
		{Title: "Detail", Width: 5},
	}

	s.table = blit.NewTable(columns, nil, blit.TableOpts{
		Filterable: true,
		CellRenderer: func(row blit.Row, colIdx int, isCursor bool, theme blit.Theme) string {
			if colIdx >= len(row) {
				return ""
			}
			switch colIdx {
			case 0: // Time - dim gray
				return lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280")).Render(row[colIdx])
			case 2: // Type - colored badge
				return blit.Badge(row[colIdx], LabelColor(row[colIdx]), true)
			default:
				return row[colIdx]
			}
		},
		RowStyler: func(row blit.Row, idx int, isCursor bool, theme blit.Theme) *lipgloss.Style {
			if idx < len(s.filteredEvents) {
				de := s.filteredEvents[idx]
				if !de.AddedAt.IsZero() && time.Now().Before(de.AddedAt.Add(flashDuration)) {
					st := lipgloss.NewStyle().Background(lipgloss.Color("#1a2a1a"))
					return &st
				}
			}
			return nil
		},
		OnRowClick: func(row blit.Row, rowIdx int) {
			if rowIdx < len(s.filteredEvents) && s.DetailOverlay != nil {
				s.DetailOverlay.Show(s.filteredEvents[rowIdx])
			}
		},
		DetailFunc: func(row blit.Row, rowIdx int, width int, theme blit.Theme) string {
			if rowIdx < len(s.filteredEvents) {
				return s.renderDetailBar(s.filteredEvents[rowIdx], theme)
			}
			return ""
		},
		DetailHeight: 3,
	})

	s.poller = blit.NewPoller(
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

func (s *EventStream) Update(msg tea.Msg, ctx blit.Context) (blit.Component, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Enter opens detail overlay instead of browser
		if msg.String() == "enter" && s.DetailOverlay != nil {
			idx := s.table.CursorIndex()
			if idx >= 0 && idx < len(s.filteredEvents) {
				s.DetailOverlay.Show(s.filteredEvents[idx])
				return s, blit.Consumed()
			}
		}
		// 'o' opens in browser
		if msg.String() == "o" {
			idx := s.table.CursorIndex()
			if idx >= 0 && idx < len(s.filteredEvents) {
				if url := s.filteredEvents[idx].Event.URL(); url != "" {
					blit.OpenURL(url)
				}
				return s, blit.Consumed()
			}
		}
		// Let table handle its own keys (cursor, search, etc.)
		comp, cmd := s.table.Update(msg, ctx)
		_ = comp
		return s, cmd

	case blit.TickMsg:
		s.poller.SetInterval(time.Duration(s.cfg.Interval) * time.Second)

		// EPM window rotation
		if s.lastEPMTick.IsZero() {
			s.lastEPMTick = msg.Time
		} else if msg.Time.Sub(s.lastEPMTick) >= time.Minute {
			s.epmWindow = append(s.epmWindow, float64(s.currentMinCount))
			if len(s.epmWindow) > 30 {
				s.epmWindow = s.epmWindow[1:]
			}
			s.currentMinCount = 0
			s.lastEPMTick = msg.Time
		}

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

func (s *EventStream) handleEvents(msg eventsMsg) (blit.Component, tea.Cmd) {
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
	s.currentMinCount += newCount
	if newCount > 0 {
		sort.Slice(s.allEvents, func(i, j int) bool {
			return s.allEvents[i].Event.CreatedAt.Before(s.allEvents[j].Event.CreatedAt)
		})
		atEdge := s.isAtNewEdge()
		s.rebuildFiltered()
		if atEdge {
			if s.newestFirst {
				s.table.SetCursor(0)
			} else {
				s.table.SetCursor(len(s.filteredEvents) - 1)
			}
		}
	}

	var cmds []tea.Cmd

	if newCount > 0 && !s.isAtNewEdge() {
		cmds = append(cmds, blit.ToastCmd(blit.SeverityInfo, "New events",
			fmt.Sprintf("%d new events arrived", newCount), 3*time.Second))
	}

	stats := s.debugLog.GetStats()
	if stats.RateLimit > 0 {
		ratePct := float64(stats.RateRemain) / float64(stats.RateLimit) * 100
		if ratePct < 20 {
			cmds = append(cmds, blit.ToastCmd(blit.SeverityWarn, "Rate limit low",
				fmt.Sprintf("API rate limit at %.0f%%", ratePct), 5*time.Second))
		}
	}

	return s, tea.Batch(cmds...)
}

func (s *EventStream) handleGitStatus(msg gitStatusMsg) (blit.Component, tea.Cmd) {
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
				s.table.SetCursor(0)
			} else {
				s.table.SetCursor(len(s.filteredEvents) - 1)
			}
		}
	}
	return s, nil
}

func (s *EventStream) View() string {
	header := s.renderHeader()
	return header + "\n\n" + s.table.View()
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
	if len(s.epmWindow) >= 2 {
		spark, _ := blit.Sparkline(s.epmWindow, 30, nil)
		statusParts = append(statusParts, "Activity: "+spark)
	}
	status := SubtitleStyle.Render(strings.Join(statusParts, "  "))

	return lipgloss.JoinVertical(lipgloss.Left, title, repoList, status)
}

func (s *EventStream) renderDetailBar(de DisplayEvent, theme blit.Theme) string {
	ev := de.Event
	divider := blit.Divider(s.width, theme)

	repo := ev.Repo.Name
	label := ev.Label()
	actor := ev.Actor.Login
	t := ev.CreatedAt.Local().Format("2006-01-02 15:04:05")
	color := EventColor(ev.Type)

	line1 := fmt.Sprintf(" %s  %s  %s  %s",
		blit.Badge(label, color, true),
		DetailRepoStyle.Render(repo),
		DetailActorStyle.Render(actor),
		DetailTimeStyle.Render(t),
	)

	detail := blit.Truncate(ev.Detail(), s.width-20)
	urlHint := ""
	if url := ev.URL(); url != "" {
		urlHint = DetailTimeStyle.Render("  ↵ open")
	}
	line2 := " " + DetailStyle.Render(detail) + urlHint

	return divider + "\n" + line1 + "\n" + line2
}

func (s *EventStream) KeyBindings() []blit.KeyBind {
	bindings := s.table.KeyBindings()
	bindings = append(bindings,
		blit.KeyBind{Key: "enter", Label: "Event detail", Group: "NAVIGATION"},
		blit.KeyBind{Key: "o", Label: "Open in browser", Group: "NAVIGATION"},
	)
	return bindings
}

func (s *EventStream) SetSize(w, h int) {
	s.width = w
	headerHeight := 4
	s.table.SetSize(w, h-headerHeight)
}

func (s *EventStream) Focused() bool       { return s.focused }
func (s *EventStream) SetFocused(f bool) {
	s.focused = f
	s.table.SetFocused(f)
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

func (s *EventStream) SetTypeFilter(t string) {
	s.typeFilter = t
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
		s.table.SetCursor(0)
	} else {
		s.table.SetCursor(len(s.filteredEvents) - 1)
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
	s.filteredEvents = s.filteredEvents[:0]
	if s.newestFirst {
		for i := len(s.allEvents) - 1; i >= 0; i-- {
			if !s.skipEvent(s.allEvents[i]) {
				s.filteredEvents = append(s.filteredEvents, s.allEvents[i])
			}
		}
	} else {
		for _, de := range s.allEvents {
			if !s.skipEvent(de) {
				s.filteredEvents = append(s.filteredEvents, de)
			}
		}
	}
	rows := make([]blit.Row, len(s.filteredEvents))
	for i, de := range s.filteredEvents {
		rows[i] = eventToRow(de)
	}
	s.table.SetRows(rows)
}

func (s *EventStream) isAtNewEdge() bool {
	total := len(s.filteredEvents)
	idx := s.table.CursorIndex()
	if s.newestFirst {
		return idx == 0
	}
	return idx >= total-1
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
