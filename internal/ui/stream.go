package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
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

	events        []DisplayEvent
	seen          map[string]bool
	seenLocalSHAs map[string]bool
	viewport      viewport.Model
	ready         bool

	streamCursor    int
	streamLineCount int
	streamEvents    []DisplayEvent

	filter     string // repo name filter
	typeFilter string // event type filter
	newestFirst bool

	paused       bool
	needsRefresh bool
	lastPoll     time.Time
	firstPoll    bool

	localRepos []discovery.LocalRepo
	knownRepos []string // track repo list for config change detection

	focused bool
	width   int
}

func NewEventStream(cfg *config.Config, debugLog *DebugLog) *EventStream {
	return &EventStream{
		cfg:           cfg,
		debugLog:      debugLog,
		seen:          make(map[string]bool),
		seenLocalSHAs: make(map[string]bool),
		events:        make([]DisplayEvent, 0, 256),
		knownRepos:    append([]string{}, cfg.Repos()...),
	}
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
		return s.handleKey(msg)

	case tuikit.TickMsg:
		s.rebuildViewport()
		// Check if repos changed (config editor modified them)
		currentRepos := s.cfg.Repos()
		if !slicesEqual(currentRepos, s.knownRepos) {
			s.knownRepos = append([]string{}, currentRepos...)
			return s, discoverRepos(s.cfg)
		}
		// Periodic data polling
		if !s.paused && time.Since(s.lastPoll) >= time.Duration(s.cfg.Interval)*time.Second {
			s.lastPoll = time.Now()
			cmds := []tea.Cmd{pollEvents(s.cfg, s.debugLog, false)}
			if len(s.localRepos) > 0 {
				cmds = append(cmds, pollGitStatus(s.localRepos))
			}
			return s, tea.Batch(cmds...)
		}
		// Manual refresh
		if s.needsRefresh {
			s.needsRefresh = false
			s.lastPoll = time.Now()
			cmds := []tea.Cmd{pollEvents(s.cfg, s.debugLog, false)}
			if len(s.localRepos) > 0 {
				cmds = append(cmds, pollGitStatus(s.localRepos))
			}
			return s, tea.Batch(cmds...)
		}
		return s, nil

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

func (s *EventStream) handleKey(msg tea.KeyMsg) (tuikit.Component, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if s.streamCursor > 0 {
			s.streamCursor--
		}
		s.rebuildViewport()
		s.ensureCursorVisible()
		return s, tuikit.Consumed()
	case "down", "j":
		if s.streamCursor < s.streamLineCount-1 {
			s.streamCursor++
		}
		s.rebuildViewport()
		s.ensureCursorVisible()
		return s, tuikit.Consumed()
	case "home", "g":
		s.streamCursor = 0
		s.rebuildViewport()
		s.ensureCursorVisible()
		return s, tuikit.Consumed()
	case "end", "G":
		s.streamCursor = max(0, s.streamLineCount-1)
		s.rebuildViewport()
		s.ensureCursorVisible()
		return s, tuikit.Consumed()
	case "enter":
		if de := s.cursorEvent(); de != nil {
			if url := de.Event.URL(); url != "" {
				openURL(url)
			}
		}
		return s, tuikit.Consumed()
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
	for _, ev := range msg.events {
		if s.seen[ev.ID] {
			continue
		}
		s.seen[ev.ID] = true
		addedAt := time.Time{}
		if s.firstPoll && now.Sub(ev.CreatedAt) < recentThreshold {
			addedAt = now
		}
		s.events = append(s.events, DisplayEvent{Event: ev, AddedAt: addedAt})
		newCount++
	}
	s.firstPoll = true
	if newCount > 0 {
		sort.Slice(s.events, func(i, j int) bool {
			return s.events[i].Event.CreatedAt.Before(s.events[j].Event.CreatedAt)
		})
		atEdge := s.isAtNewEdge()
		s.rebuildViewport()
		if atEdge {
			if s.newestFirst {
				s.streamCursor = 0
				s.viewport.GotoTop()
			} else {
				s.streamCursor = max(0, s.streamLineCount-1)
				s.viewport.GotoBottom()
			}
		}
	}
	s.lastPoll = time.Now()
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
			if s.firstPoll && now.Sub(commitTime) < 2*time.Minute {
				addedAt = now
			}
			s.events = append(s.events, DisplayEvent{Event: ev, AddedAt: addedAt})
			newLocal++
		}
	}
	if newLocal > 0 {
		sort.Slice(s.events, func(i, j int) bool {
			return s.events[i].Event.CreatedAt.Before(s.events[j].Event.CreatedAt)
		})
		atEdge := s.isAtNewEdge()
		s.rebuildViewport()
		if atEdge {
			if s.newestFirst {
				s.viewport.GotoTop()
			} else {
				s.viewport.GotoBottom()
			}
		}
	}
	return s, nil
}

func (s *EventStream) View() string {
	if !s.ready {
		return "Initializing..."
	}

	// Header
	header := s.renderHeader()

	// Focus badge
	badge := FocusBadgeInactive.Render("Stream")
	if s.focused {
		badge = FocusBadgeActive.Render("Stream")
	}

	// Detail bar
	detailBar := s.renderDetailBar()

	vpView := strings.TrimRight(s.viewport.View(), "\n")
	view := lipgloss.JoinVertical(lipgloss.Left, header, badge, vpView, detailBar)
	return lipgloss.NewStyle().MaxWidth(s.width).Render(view)
}

func (s *EventStream) renderHeader() string {
	title := TitleStyle.Render("gitstream")
	repoList := SubtitleStyle.Render(fmt.Sprintf("Watching: %s", strings.Join(s.cfg.Repos(), ", ")))

	// Status line: poll info + health dots + rate limit
	var statusParts []string
	if s.paused {
		statusParts = append(statusParts, lipgloss.NewStyle().Foreground(lipgloss.Color("#eab308")).Render("[PAUSED]"))
	} else if !s.lastPoll.IsZero() {
		ago := time.Since(s.lastPoll).Truncate(time.Second)
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

func (s *EventStream) KeyBindings() []tuikit.KeyBind {
	return []tuikit.KeyBind{
		{Key: "up/k", Label: "Scroll up", Group: "NAVIGATION"},
		{Key: "down/j", Label: "Scroll down", Group: "NAVIGATION"},
		{Key: "home/g", Label: "Go to top", Group: "NAVIGATION"},
		{Key: "end/G", Label: "Go to bottom", Group: "NAVIGATION"},
		{Key: "enter", Label: "Open in browser", Group: "NAVIGATION"},
	}
}

func (s *EventStream) SetSize(w, h int) {
	s.width = w
	// header(4) + badge(1) + detail(3) = 8 lines of chrome
	vpHeight := h - 8
	if vpHeight < 3 {
		vpHeight = 3
	}
	if !s.ready {
		s.viewport = viewport.New(w, vpHeight)
		s.viewport.YPosition = 0
		s.ready = true
	} else {
		s.viewport.Width = w
		s.viewport.Height = vpHeight
	}
	s.rebuildViewport()
}

func (s *EventStream) Focused() bool       { return s.focused }
func (s *EventStream) SetFocused(f bool)    { s.focused = f; s.rebuildViewport() }

// Public methods for app-level keybinding handlers.

func (s *EventStream) SetRepoFilter(repo string) {
	if s.filter == repo {
		s.filter = ""
	} else {
		s.filter = repo
	}
	s.rebuildViewport()
}

func (s *EventStream) ClearFilters() {
	s.filter = ""
	s.typeFilter = ""
	s.rebuildViewport()
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
	s.rebuildViewport()
}

func (s *EventStream) ToggleSort() {
	s.newestFirst = !s.newestFirst
	s.rebuildViewport()
	if s.newestFirst {
		s.streamCursor = 0
		s.viewport.GotoTop()
	} else {
		s.streamCursor = max(0, s.streamLineCount-1)
		s.viewport.GotoBottom()
	}
}

func (s *EventStream) TogglePause()   { s.paused = !s.paused }
func (s *EventStream) ForceRefresh()  { s.needsRefresh = true }
func (s *EventStream) IsPaused() bool { return s.paused }
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

func (s *EventStream) rebuildViewport() {
	now := time.Now()
	var lines []string
	var displayEvents []DisplayEvent

	addLine := func(de DisplayEvent) {
		flash := !de.AddedAt.IsZero() && now.Before(de.AddedAt.Add(flashDuration))
		line := renderEventLine(de.Event, now)
		idx := len(lines)
		if s.focused && idx == s.streamCursor {
			line = CursorMarker.Render("▌") + " " + line
		} else if flash {
			line = FlashMarker.Render("▐") + " " + line
		} else {
			line = "  " + line
		}
		lines = append(lines, line)
		displayEvents = append(displayEvents, de)
	}

	if s.newestFirst {
		for i := len(s.events) - 1; i >= 0; i-- {
			if s.skipEvent(s.events[i]) {
				continue
			}
			addLine(s.events[i])
		}
	} else {
		for _, de := range s.events {
			if s.skipEvent(de) {
				continue
			}
			addLine(de)
		}
	}

	s.streamLineCount = len(lines)
	s.streamEvents = displayEvents
	if s.streamCursor >= s.streamLineCount && s.streamLineCount > 0 {
		s.streamCursor = s.streamLineCount - 1
	}
	s.viewport.SetContent(strings.Join(lines, "\n"))
}

func (s *EventStream) cursorEvent() *DisplayEvent {
	if s.streamCursor >= 0 && s.streamCursor < len(s.streamEvents) {
		return &s.streamEvents[s.streamCursor]
	}
	return nil
}

func (s *EventStream) isAtNewEdge() bool {
	if s.newestFirst {
		return s.viewport.YOffset == 0
	}
	return s.viewport.AtBottom()
}

func (s *EventStream) ensureCursorVisible() {
	vpHeight := s.viewport.Height
	yOffset := s.viewport.YOffset
	if s.streamCursor < yOffset {
		s.viewport.SetYOffset(s.streamCursor)
	} else if s.streamCursor >= yOffset+vpHeight {
		s.viewport.SetYOffset(s.streamCursor - vpHeight + 1)
	}
}

func (s *EventStream) renderDetailBar() string {
	divider := DividerStyle.Render(strings.Repeat("─", s.width))

	if !s.focused {
		return divider + "\n" + " \n" + " "
	}
	de := s.cursorEvent()
	if de == nil {
		return divider + "\n" + " \n" + " "
	}
	ev := de.Event

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
