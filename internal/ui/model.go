package ui

import (
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/moneycaringcoder/gitstream-tui/internal/config"
	"github.com/moneycaringcoder/gitstream-tui/internal/discovery"
	"github.com/moneycaringcoder/gitstream-tui/internal/github"
	"github.com/moneycaringcoder/gitstream-tui/internal/gitstatus"
)

const flashDuration = 3 * time.Second

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

// DisplayEvent holds a parsed event for display.
type DisplayEvent struct {
	Event   github.Event
	AddedAt time.Time
}

const statusPanelWidth = 32

type focusPane int

const (
	focusStream focusPane = iota
	focusPanel
)

// Model is the main Bubble Tea model.
type Model struct {
	cfg           *config.Config
	events        []DisplayEvent
	seen          map[string]bool
	viewport      viewport.Model
	panelViewport viewport.Model
	focus         focusPane
	width         int
	height        int
	ready         bool
	err           error
	lastPoll      time.Time
	paused        bool
	filter        string // filter by repo name
	typeFilter    string // filter by event type label
	newestFirst   bool   // sort order: true = newest on top
	firstPoll     bool   // true after first poll completes
	streamCursor    int // cursor row index in filtered events
	streamLineCount int // total visible lines in stream
	streamEvents    []DisplayEvent // filtered events in display order (parallel to lines)
	panelCursor     int // cursor row index in panel content
	panelLineCount  int // total visible lines in panel
	localRepos      []discovery.LocalRepo
	repoStatus      []gitstatus.RepoStatus
	seenLocalSHAs   map[string]bool
	configUI        configState
	debugLog        *DebugLog
	debugUI         debugState
}

// Messages
type tickMsg struct{}
type uiTickMsg struct{}
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

func NewModel(cfg *config.Config) Model {
	return Model{
		cfg:           cfg,
		seen:          make(map[string]bool),
		seenLocalSHAs: make(map[string]bool),
		events:        make([]DisplayEvent, 0, 256),
		debugLog:      NewDebugLog(),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		pollEvents(m.cfg, m.debugLog, true),
		tickCmd(time.Duration(m.cfg.Interval)*time.Second),
		uiTickCmd(),
		discoverRepos(m.cfg),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Debug overlay takes over input when active
	if m.debugUI.active {
		if msg, ok := msg.(tea.KeyMsg); ok {
			switch msg.String() {
			case "D", "esc":
				m.debugUI.active = false
				return m, nil
			case "q", "ctrl+c":
				return m, tea.Quit
			}
			return m, nil
		}
	}

	// Config editor takes over input when active
	if m.configUI.active {
		if msg, ok := msg.(tea.KeyMsg); ok {
			return m.updateConfig(msg)
		}
		// Still handle non-key messages (ticks, window resize, etc.)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "c":
			m.configUI = configState{active: true}
			return m, nil
		case "D":
			m.debugUI.active = !m.debugUI.active
			return m, nil
		case "p":
			m.paused = !m.paused
		case "r":
			return m, pollEvents(m.cfg, m.debugLog, false)
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			idx := int(msg.String()[0]-'0') - 1
			if idx < len(m.cfg.Repos()) {
				repo := m.cfg.Repos()[idx]
				short := repo
				if i := strings.LastIndex(repo, "/"); i >= 0 {
					short = repo[i+1:]
				}
				if m.filter == short {
					m.filter = "" // toggle off
				} else {
					m.filter = short
				}
				m.rebuildViewport()
			}
		case "0":
			m.filter = ""
			m.typeFilter = ""
			m.rebuildViewport()
		case "s":
			m.newestFirst = !m.newestFirst
			m.rebuildViewport()
			// Snap cursor to newest edge after sort flip
			if m.newestFirst {
				m.streamCursor = 0
				m.viewport.GotoTop()
			} else {
				m.streamCursor = max(0, m.streamLineCount-1)
				m.viewport.GotoBottom()
			}
		case "t":
			// Cycle through type filters
			cur := 0
			for i, t := range typeFilters {
				if t == m.typeFilter {
					cur = i
					break
				}
			}
			m.typeFilter = typeFilters[(cur+1)%len(typeFilters)]
			m.rebuildViewport()
		case "T":
			// Cycle backward
			cur := 0
			for i, t := range typeFilters {
				if t == m.typeFilter {
					cur = i
					break
				}
			}
			m.typeFilter = typeFilters[(cur-1+len(typeFilters))%len(typeFilters)]
			m.rebuildViewport()
		case "left", "h":
			m.focus = focusStream
			m.rebuildViewport()
			m.rebuildPanelContent()
		case "right", "l":
			if m.hasPanel() {
				m.focus = focusPanel
				m.rebuildViewport()
				m.rebuildPanelContent()
			}
		case "up", "k":
			if m.focus == focusPanel {
				if m.panelCursor > 0 {
					m.panelCursor--
				}
			} else {
				if m.streamCursor > 0 {
					m.streamCursor--
				}
			}
			m.rebuildViewport()
			m.rebuildPanelContent()
			m.ensureCursorVisible()
			return m, nil
		case "down", "j":
			if m.focus == focusPanel {
				if m.panelCursor < m.panelLineCount-1 {
					m.panelCursor++
				}
			} else {
				if m.streamCursor < m.streamLineCount-1 {
					m.streamCursor++
				}
			}
			m.rebuildViewport()
			m.rebuildPanelContent()
			m.ensureCursorVisible()
			return m, nil
		case "home", "g":
			if m.focus == focusPanel {
				m.panelCursor = 0
			} else {
				m.streamCursor = 0
			}
			m.rebuildViewport()
			m.rebuildPanelContent()
			m.ensureCursorVisible()
			return m, nil
		case "end", "G":
			if m.focus == focusPanel {
				m.panelCursor = max(0, m.panelLineCount-1)
			} else {
				m.streamCursor = max(0, m.streamLineCount-1)
			}
			m.rebuildViewport()
			m.rebuildPanelContent()
			m.ensureCursorVisible()
			return m, nil
		case "enter":
			if m.focus == focusStream {
				if de := m.cursorEvent(); de != nil {
					url := de.Event.URL()
					if url != "" {
						openURL(url)
					}
				}
			}
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		ch := m.contentHeight()
		streamWidth := m.streamWidth()
		if !m.ready {
			m.viewport = viewport.New(streamWidth, ch)
			m.viewport.YPosition = 0
			m.panelViewport = viewport.New(statusPanelWidth, ch)
			m.ready = true
		} else {
			m.viewport.Width = streamWidth
			m.viewport.Height = ch
			m.panelViewport.Width = statusPanelWidth
			m.panelViewport.Height = ch
		}
		m.rebuildViewport()
		m.rebuildPanelContent()

	case eventsMsg:
		for _, e := range msg.errors {
			m.debugLog.Error("%s", e)
		}
		{
			now := time.Now()
			recentThreshold := time.Duration(m.cfg.Interval) * time.Second * 2
			newCount := 0
			for _, ev := range msg.events {
				if m.seen[ev.ID] {
					continue
				}
				m.seen[ev.ID] = true
				addedAt := time.Time{} // zero = no flash by default
				// Only flash if this is a genuinely recent event
				if m.firstPoll && now.Sub(ev.CreatedAt) < recentThreshold {
					addedAt = now
				}
				de := DisplayEvent{Event: ev, AddedAt: addedAt}
				m.events = append(m.events, de)
				newCount++
			}
			m.firstPoll = true
			if newCount > 0 {
				sort.Slice(m.events, func(i, j int) bool {
					return m.events[i].Event.CreatedAt.Before(m.events[j].Event.CreatedAt)
				})
				// Auto-scroll to show newest events if user hasn't scrolled away
				atEdge := m.isStreamAtNewEdge()
				m.rebuildViewport()
				if atEdge {
					if m.newestFirst {
						m.streamCursor = 0
						m.viewport.GotoTop()
					} else {
						m.streamCursor = max(0, m.streamLineCount-1)
						m.viewport.GotoBottom()
					}
				}
			}
		}
		m.lastPoll = time.Now()

	case discoveryMsg:
		m.localRepos = msg.repos
		if len(m.localRepos) > 0 {
			m.viewport.Width = m.streamWidth()
			m.rebuildViewport()
			return m, pollGitStatus(m.localRepos)
		}

	case gitStatusMsg:
		m.repoStatus = msg.statuses
		m.rebuildPanelContent()
		// Inject unpushed commits into the event stream
		now := time.Now()
		newLocal := 0
		for _, s := range msg.statuses {
			for _, c := range s.UnpushedCommits {
				key := s.Remote + ":" + c.SHA
				if m.seenLocalSHAs[key] {
					continue
				}
				m.seenLocalSHAs[key] = true
				commitTime, _ := time.Parse(time.RFC3339, c.Date)
				if commitTime.IsZero() {
					commitTime = now
				}
				ev := github.Event{
					ID:   "local-" + key,
					Type: "LocalPushEvent",
					Actor: github.Actor{Login: c.Author},
					Repo:  github.Repo{Name: s.Remote},
					Payload: github.Payload{
						Ref:     s.Branch,
						Commits: []github.Commit{{SHA: c.SHA, Message: c.Message}},
					},
					CreatedAt: commitTime,
				}
				addedAt := time.Time{}
				if m.firstPoll && now.Sub(commitTime) < 2*time.Minute {
					addedAt = now
				}
				m.events = append(m.events, DisplayEvent{Event: ev, AddedAt: addedAt})
				newLocal++
			}
		}
		if newLocal > 0 {
			sort.Slice(m.events, func(i, j int) bool {
				return m.events[i].Event.CreatedAt.Before(m.events[j].Event.CreatedAt)
			})
			atEdge := m.isStreamAtNewEdge()
			m.rebuildViewport()
			if atEdge {
				if m.newestFirst {
					m.viewport.GotoTop()
				} else {
					m.viewport.GotoBottom()
				}
			}
		}

	case uiTickMsg:
		m.rebuildViewport()
		return m, uiTickCmd()

	case tickMsg:
		cmds := []tea.Cmd{tickCmd(time.Duration(m.cfg.Interval) * time.Second)}
		if !m.paused {
			cmds = append(cmds, pollEvents(m.cfg, m.debugLog, false))
			if len(m.localRepos) > 0 {
				cmds = append(cmds, pollGitStatus(m.localRepos))
			}
		}
		return m, tea.Batch(cmds...)
	}

	var cmd tea.Cmd
	if m.focus == focusPanel && m.hasPanel() {
		m.panelViewport, cmd = m.panelViewport.Update(msg)
	} else {
		m.viewport, cmd = m.viewport.Update(msg)
	}
	return m, cmd
}

// contentHeight returns the viewport height after subtracting chrome.
// header(4) + badge(1) + detailBar(3) + help(1) = 9 lines of chrome.
func (m *Model) contentHeight() int {
	h := m.height - 9
	if h < 3 {
		h = 3
	}
	return h
}

func (m *Model) streamWidth() int {
	if len(m.localRepos) > 0 {
		w := m.width - statusPanelWidth - 1 // 1 for divider
		if w < 40 {
			return m.width // too narrow, skip panel
		}
		return w
	}
	return m.width
}

func (m *Model) hasPanel() bool {
	return len(m.localRepos) > 0 && m.width-statusPanelWidth-3 >= 40
}

// isStreamAtNewEdge returns true if the viewport is scrolled to where new events appear.
func (m *Model) isStreamAtNewEdge() bool {
	if m.newestFirst {
		return m.viewport.YOffset == 0
	}
	return m.viewport.AtBottom()
}

func (m *Model) ensureCursorVisible() {
	if m.focus == focusPanel {
		vpHeight := m.panelViewport.Height
		yOffset := m.panelViewport.YOffset
		if m.panelCursor < yOffset {
			m.panelViewport.SetYOffset(m.panelCursor)
		} else if m.panelCursor >= yOffset+vpHeight {
			m.panelViewport.SetYOffset(m.panelCursor - vpHeight + 1)
		}
	} else {
		vpHeight := m.viewport.Height
		yOffset := m.viewport.YOffset
		if m.streamCursor < yOffset {
			m.viewport.SetYOffset(m.streamCursor)
		} else if m.streamCursor >= yOffset+vpHeight {
			m.viewport.SetYOffset(m.streamCursor - vpHeight + 1)
		}
	}
}

func (m *Model) skipEvent(de DisplayEvent) bool {
	if m.filter != "" && de.Event.ShortRepo() != m.filter {
		return true
	}
	if m.typeFilter != "" && de.Event.Type != m.typeFilter {
		return true
	}
	return false
}

func (m *Model) rebuildViewport() {
	now := time.Now()
	var lines []string
	var displayEvents []DisplayEvent
	isFocused := m.focus == focusStream

	addLine := func(de DisplayEvent) {
		flash := !de.AddedAt.IsZero() && now.Before(de.AddedAt.Add(flashDuration))
		line := renderEventLine(de.Event, now)
		idx := len(lines)
		if isFocused && idx == m.streamCursor {
			line = CursorMarker.Render("▌") + " " + line
		} else if flash {
			line = FlashMarker.Render("▐") + " " + line
		} else {
			line = "  " + line
		}
		lines = append(lines, line)
		displayEvents = append(displayEvents, de)
	}

	if m.newestFirst {
		for i := len(m.events) - 1; i >= 0; i-- {
			de := m.events[i]
			if m.skipEvent(de) {
				continue
			}
			addLine(de)
		}
	} else {
		for _, de := range m.events {
			if m.skipEvent(de) {
				continue
			}
			addLine(de)
		}
	}
	m.streamLineCount = len(lines)
	m.streamEvents = displayEvents
	if m.streamCursor >= m.streamLineCount && m.streamLineCount > 0 {
		m.streamCursor = m.streamLineCount - 1
	}
	m.viewport.SetContent(strings.Join(lines, "\n"))
}

// cursorEvent returns the DisplayEvent under the stream cursor, if any.
func (m *Model) cursorEvent() *DisplayEvent {
	if m.streamCursor >= 0 && m.streamCursor < len(m.streamEvents) {
		return &m.streamEvents[m.streamCursor]
	}
	return nil
}

func (m Model) updateConfig(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if m.configUI.editing {
		switch key {
		case "enter":
			f := configFields[m.configUI.cursor]
			if err := f.set(m.cfg, m.configUI.editBuf); err != nil {
				m.configUI.editErr = err.Error()
			} else {
				m.configUI.editing = false
				m.configUI.editErr = ""
				m.configUI.dirty = true
			}
		case "esc":
			m.configUI.editing = false
			m.configUI.editBuf = ""
			m.configUI.editErr = ""
		case "backspace":
			if len(m.configUI.editBuf) > 0 {
				m.configUI.editBuf = m.configUI.editBuf[:len(m.configUI.editBuf)-1]
			}
		default:
			if len(key) == 1 && key[0] >= 32 && key[0] <= 126 {
				m.configUI.editBuf += key
			}
		}
		return m, nil
	}

	// Navigation mode
	switch key {
	case "j", "down":
		if m.configUI.cursor < len(configFields)-1 {
			m.configUI.cursor++
		}
	case "k", "up":
		if m.configUI.cursor > 0 {
			m.configUI.cursor--
		}
	case "enter":
		f := configFields[m.configUI.cursor]
		m.configUI.editing = true
		m.configUI.editBuf = f.get(m.cfg)
		m.configUI.editErr = ""
	case "ctrl+s":
		config.Save(m.cfg)
		m.configUI.dirty = false
		m.configUI.savedNotice = 20
	case "esc", "c":
		if m.configUI.dirty {
			config.Save(m.cfg)
		}
		m.configUI.active = false
		// Re-trigger discovery in case repos changed
		return m, discoverRepos(m.cfg)
	case "q", "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) View() string {
	if !m.ready {
		return "Initializing..."
	}

	if m.debugUI.active {
		return m.renderDebugView()
	}

	if m.configUI.active {
		return m.renderConfigView()
	}

	// Header
	title := TitleStyle.Render("gitstream")
	repoList := SubtitleStyle.Render(fmt.Sprintf("Watching: %s", strings.Join(m.cfg.Repos(), ", ")))

	// Status line: poll info + health dots + rate limit (single line)
	var statusParts []string
	if m.paused {
		statusParts = append(statusParts, lipgloss.NewStyle().Foreground(lipgloss.Color("#eab308")).Render("[PAUSED]"))
	} else if !m.lastPoll.IsZero() {
		ago := time.Since(m.lastPoll).Truncate(time.Second)
		statusParts = append(statusParts, fmt.Sprintf("Poll %s ago", ago))
	}
	stats := m.debugLog.GetStats()
	for _, repo := range m.cfg.Repos() {
		short := repo
		if i := strings.LastIndex(repo, "/"); i >= 0 {
			short = repo[i+1:]
		}
		dot := lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280")).Render("○")
		if h, ok := stats.RepoHealth[repo]; ok {
			if h.LastSuccess {
				// Green: live data
				dot = lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e")).Render("●")
			} else if h.UsingCache && h.FailStreak < cacheStaleThreshold {
				// Yellow: serving cached data
				dot = lipgloss.NewStyle().Foreground(lipgloss.Color("#eab308")).Render("●")
			} else {
				// Red: no data or cache stale (10+ failures)
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

	header := lipgloss.JoinVertical(lipgloss.Left, title, repoList, status, "")

	// Footer
	extra := ""
	if m.filter != "" {
		extra += fmt.Sprintf(" | repo: %s", m.filter)
	}
	if m.typeFilter != "" {
		// Show the label for the active type filter
		ev := github.Event{Type: m.typeFilter}
		extra += fmt.Sprintf(" | type: %s", ev.Label())
	}
	sortLabel := "oldest first"
	if m.newestFirst {
		sortLabel = "newest first"
	}
	focusLabel := "stream"
	if m.focus == focusPanel {
		focusLabel = "panel"
	}
	help := HelpStyle.PaddingLeft(1).Render(
		fmt.Sprintf("q quit | p pause | r refresh | c config | D debug | s sort (%s) | t type | ↵ open | ←/→ focus (%s) | 1-%d repo | 0 clear%s",
			sortLabel, focusLabel, len(m.cfg.Repos()), extra))

	// Build main content area with focus badges
	streamBadge := FocusBadgeInactive.Render("Stream")
	if m.focus == focusStream {
		streamBadge = FocusBadgeActive.Render("Stream")
	}
	streamView := lipgloss.JoinVertical(lipgloss.Left, streamBadge, m.viewport.View())

	if m.hasPanel() {
		panelBadge := FocusBadgeInactive.Render("Local")
		if m.focus == focusPanel {
			panelBadge = FocusBadgeActive.Render("Local")
		}
		panel := m.renderStatusPanel()
		panelWithBadge := lipgloss.JoinVertical(lipgloss.Left, panelBadge, panel)

		// Build a full-height vertical divider
		dividerHeight := m.contentHeight() + 1 // viewport + badge
		dividerCol := DividerStyle.Render(strings.Repeat("│\n", dividerHeight))

		streamView = lipgloss.JoinHorizontal(lipgloss.Top, streamView, dividerCol, panelWithBadge)
	}

	// Detail preview bar
	detailBar := m.renderDetailBar()

	// Compose
	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		streamView,
		detailBar,
		help,
	)
}

// sortedRepoStatus returns repo statuses sorted with dirty/unpushed repos first.
func sortedRepoStatus(statuses []gitstatus.RepoStatus) []gitstatus.RepoStatus {
	sorted := make([]gitstatus.RepoStatus, len(statuses))
	copy(sorted, statuses)
	sort.SliceStable(sorted, func(i, j int) bool {
		iWeight := sorted[i].Uncommitted + sorted[i].Unpushed
		jWeight := sorted[j].Uncommitted + sorted[j].Unpushed
		return iWeight > jWeight
	})
	return sorted
}

func (m *Model) rebuildPanelContent() {
	var lines []string
	isFocused := m.focus == focusPanel

	addLine := func(line string) {
		idx := len(lines)
		if isFocused && idx == m.panelCursor {
			line = CursorMarker.Render("▌") + line
		}
		lines = append(lines, line)
	}

	if len(m.repoStatus) == 0 {
		addLine(PanelDimStyle.Render("Scanning..."))
		m.panelLineCount = len(lines)
		m.panelViewport.SetContent(strings.Join(lines, "\n"))
		return
	}

	sorted := sortedRepoStatus(m.repoStatus)

	for _, s := range sorted {
		short := s.Remote
		if i := strings.LastIndex(s.Remote, "/"); i >= 0 {
			short = s.Remote[i+1:]
		}

		if s.Error != nil {
			addLine(PanelRepoStyle.Render(short))
			addLine(PanelDimStyle.Render("  error"))
			addLine("")
			continue
		}

		addLine(PanelRepoStyle.Render(short))

		// Branch
		addLine(PanelDimStyle.Render(fmt.Sprintf("  ᛘ %s", s.Branch)))

		// Status indicators
		if s.Uncommitted == 0 && s.Unpushed == 0 {
			addLine(PanelCleanStyle.Render("  ✓ clean"))
		} else {
			if s.Uncommitted > 0 {
				addLine(PanelDirtyStyle.Render(
					fmt.Sprintf("  ● %d uncommitted", s.Uncommitted)))
			}
			if s.Unpushed > 0 {
				addLine(PanelWarnStyle.Render(
					fmt.Sprintf("  ↑ %d unpushed", s.Unpushed)))
				for _, c := range s.UnpushedCommits {
					msg := c.Message
					maxLen := statusPanelWidth - 8
					if len(msg) > maxLen {
						msg = msg[:maxLen-1] + "…"
					}
					addLine(PanelDimStyle.Render(
						fmt.Sprintf("    %s %s", c.SHA, msg)))
				}
			}
		}
		if !s.HasUpstream {
			addLine(PanelDimStyle.Render("  ⚠ no upstream"))
		}

		// CI status
		if s.CI != nil {
			var ciLine string
			switch s.CI.Conclusion {
			case "success":
				ciLine = PanelCleanStyle.Render("  ✓ CI passed")
			case "failure":
				ciLine = PanelCIFailStyle.Render("  ✗ CI failed")
			case "cancelled":
				ciLine = PanelDimStyle.Render("  ○ CI cancelled")
			default:
				if s.CI.Status == "in_progress" {
					ciLine = PanelWarnStyle.Render("  ◌ CI running")
				} else {
					ciLine = PanelDimStyle.Render(fmt.Sprintf("  ○ CI %s", s.CI.Conclusion))
				}
			}
			addLine(ciLine)
		}

		addLine("")
	}

	m.panelLineCount = len(lines)
	if m.panelCursor >= m.panelLineCount && m.panelLineCount > 0 {
		m.panelCursor = m.panelLineCount - 1
	}
	m.panelViewport.SetContent(strings.Join(lines, "\n"))
}

func (m Model) renderStatusPanel() string {
	return m.panelViewport.View()
}

// renderDetailBar returns a fixed 3-line preview bar (divider + 2 content lines).
func (m Model) renderDetailBar() string {
	divider := DividerStyle.Render(strings.Repeat("─", m.width))

	if m.focus != focusStream {
		return divider + "\n" + "\n"
	}
	de := m.cursorEvent()
	if de == nil {
		return divider + "\n" + "\n"
	}
	ev := de.Event

	// Line 1: repo, type, actor, time
	repo := ev.Repo.Name
	label := ev.Label()
	actor := ev.Actor.Login
	t := ev.CreatedAt.Local().Format("2006-01-02 15:04:05")
	color := EventColor(ev.Type)
	line1 := fmt.Sprintf(" %s  %s  %s  %s",
		lipgloss.NewStyle().Bold(true).Foreground(color).Render(label),
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffffff")).Render(repo),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#d1d5db")).Render(actor),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280")).Render(t),
	)

	// Line 2: full detail + URL hint
	detail := ev.Detail()
	maxDetail := m.width - 20
	if maxDetail < 20 {
		maxDetail = 20
	}
	if len(detail) > maxDetail {
		detail = detail[:maxDetail-1] + "…"
	}
	urlHint := ""
	if url := ev.URL(); url != "" {
		urlHint = lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280")).Render("  ↵ open")
	}
	line2 := " " + DetailStyle.Render(detail) + urlHint

	return divider + "\n" + line1 + "\n" + line2
}

func relativeTime(t time.Time, now time.Time) string {
	d := now.Sub(t)
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// osc8 wraps text in an OSC8 hyperlink escape sequence.
func osc8(url, text string) string {
	if url == "" {
		return text
	}
	return fmt.Sprintf("\x1b]8;;%s\x1b\\%s\x1b]8;;\x1b\\", url, text)
}

func renderEventLine(ev github.Event, now time.Time) string {
	t := ev.CreatedAt.Local().Format("15:04:05")
	rel := relativeTime(ev.CreatedAt, now)
	timeStr := fmt.Sprintf("%s %s", t, rel)
	label := ev.Label()
	detail := ev.Detail()
	actor := ev.Actor.Login
	repo := ev.ShortRepo()
	url := ev.URL()

	// Wrap detail in a clickable hyperlink
	detailRendered := DetailStyle.Render(detail)
	if url != "" {
		detailRendered = osc8(url, detailRendered)
	}

	line := fmt.Sprintf("%s  %s %s %s %s",
		TimeStyle.Render(timeStr),
		RepoStyle.Render(repo),
		LabelStyle(ev.Type).Render(label),
		ActorStyle.Render(actor),
		detailRendered,
	)

	return line
}

// openURL opens a URL in the default browser.
func openURL(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	cmd.Start()
}

// eventCache stores last successful events per repo for fallback.
var (
	eventCacheMu sync.Mutex
	eventCache   = make(map[string][]github.Event)
)

// fetchWithRetries fetches events with up to 3 retries and exponential backoff.
// Returns the FetchResult on success (including 304 Not Modified).
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

		// How many pages to fetch: 2 on initial load for more history, 1 after
		pages := 1
		if initial {
			pages = 2
		}

		// Track latest rate limit seen across all fetches
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

					// Update rate limit from response headers
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

				// If all pages returned 304, serve from cache (data unchanged)
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

				// If fetch failed entirely, fall back to cache
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

				// Deduplicate by event ID across pages
				seen := make(map[string]bool)
				var deduped []github.Event
				for _, ev := range allEvents {
					if !seen[ev.ID] {
						seen[ev.ID] = true
						deduped = append(deduped, ev)
					}
				}

				// Limit to 50 events per repo
				if len(deduped) > 50 {
					deduped = deduped[:50]
				}

				// Update cache with fresh events
				eventCacheMu.Lock()
				eventCache[r] = deduped
				eventCacheMu.Unlock()

				debugLog.RecordFetch(r, true, len(deduped), false)

				// Enrich push events in parallel
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

		// Update rate limit from inline headers (no extra API call needed)
		if latestRL.Limit > 0 {
			debugLog.SetRateLimit(latestRL.Remaining, latestRL.Limit)
		}

		return eventsMsg{events: all, errors: allErrors}
	}
}

func tickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

func uiTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return uiTickMsg{}
	})
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
