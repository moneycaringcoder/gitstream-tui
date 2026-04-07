package ui

import (
	"fmt"
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

// Model is the main Bubble Tea model.
type Model struct {
	cfg         *config.Config
	events      []DisplayEvent
	seen        map[string]bool
	viewport    viewport.Model
	width       int
	height      int
	ready       bool
	err         error
	lastPoll    time.Time
	paused      bool
	filter      string // filter by repo name
	typeFilter  string // filter by event type label
	newestFirst bool   // sort order: true = newest on top
	firstPoll   bool   // true after first poll completes
	localRepos  []discovery.LocalRepo
	repoStatus  []gitstatus.RepoStatus
}

// Messages
type tickMsg struct{}
type uiTickMsg struct{}
type eventsMsg struct {
	events []github.Event
	err    error
}
type discoveryMsg struct {
	repos []discovery.LocalRepo
}
type gitStatusMsg struct {
	statuses []gitstatus.RepoStatus
}

func NewModel(cfg *config.Config) Model {
	return Model{
		cfg:    cfg,
		seen:   make(map[string]bool),
		events: make([]DisplayEvent, 0, 256),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		pollEvents(m.cfg),
		tickCmd(time.Duration(m.cfg.Interval)*time.Second),
		uiTickCmd(),
		discoverRepos(m.cfg),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "p":
			m.paused = !m.paused
		case "r":
			return m, pollEvents(m.cfg)
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
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		headerHeight := 4
		footerHeight := 2
		streamWidth := m.streamWidth()
		if !m.ready {
			m.viewport = viewport.New(streamWidth, msg.Height-headerHeight-footerHeight)
			m.viewport.YPosition = headerHeight
			m.ready = true
		} else {
			m.viewport.Width = streamWidth
			m.viewport.Height = msg.Height - headerHeight - footerHeight
		}
		m.rebuildViewport()

	case eventsMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			now := time.Now()
			newCount := 0
			for _, ev := range msg.events {
				if m.seen[ev.ID] {
					continue
				}
				m.seen[ev.ID] = true
				addedAt := now
				if !m.firstPoll {
					addedAt = time.Time{} // zero = no flash for initial load
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
				m.rebuildViewport()
				m.viewport.GotoBottom()
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

	case uiTickMsg:
		m.rebuildViewport()
		return m, uiTickCmd()

	case tickMsg:
		cmds := []tea.Cmd{tickCmd(time.Duration(m.cfg.Interval) * time.Second)}
		if !m.paused {
			cmds = append(cmds, pollEvents(m.cfg))
			if len(m.localRepos) > 0 {
				cmds = append(cmds, pollGitStatus(m.localRepos))
			}
		}
		return m, tea.Batch(cmds...)
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m *Model) streamWidth() int {
	if len(m.localRepos) > 0 {
		w := m.width - statusPanelWidth - 3 // 3 for border + gap
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
	sw := m.streamWidth()
	var lines []string
	if m.newestFirst {
		for i := len(m.events) - 1; i >= 0; i-- {
			de := m.events[i]
			if m.skipEvent(de) {
				continue
			}
			flash := !de.AddedAt.IsZero() && now.Before(de.AddedAt.Add(flashDuration))
			lines = append(lines, renderEventLine(de.Event, now, flash, sw))
		}
	} else {
		for _, de := range m.events {
			if m.skipEvent(de) {
				continue
			}
			flash := !de.AddedAt.IsZero() && now.Before(de.AddedAt.Add(flashDuration))
			lines = append(lines, renderEventLine(de.Event, now, flash, sw))
		}
	}
	m.viewport.SetContent(strings.Join(lines, "\n"))
}

func (m Model) View() string {
	if !m.ready {
		return "Initializing..."
	}

	// Header
	title := TitleStyle.Render("gitstream")
	repoList := SubtitleStyle.Render(fmt.Sprintf("Watching: %s", strings.Join(m.cfg.Repos(), ", ")))

	status := ""
	if m.paused {
		status = SubtitleStyle.Copy().Foreground(lipgloss.Color("#eab308")).Render("[PAUSED]")
	} else if !m.lastPoll.IsZero() {
		ago := time.Since(m.lastPoll).Truncate(time.Second)
		status = SubtitleStyle.Render(fmt.Sprintf("Last poll: %s ago", ago))
	}

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
	help := HelpStyle.PaddingLeft(1).Render(
		fmt.Sprintf("q quit | p pause | r refresh | s sort (%s) | t type | 1-%d repo | 0 clear%s",
			sortLabel, len(m.cfg.Repos()), extra))

	// Build main content area
	streamView := m.viewport.View()

	if m.hasPanel() {
		panel := m.renderStatusPanel()
		streamView = lipgloss.JoinHorizontal(lipgloss.Top, streamView, " ", panel)
	}

	// Compose
	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		streamView,
		"",
		help,
	)
}

func (m Model) renderStatusPanel() string {
	headerHeight := 4
	footerHeight := 2
	panelHeight := m.height - headerHeight - footerHeight

	border := PanelBorderStyle.Width(statusPanelWidth).Height(panelHeight)

	title := PanelTitleStyle.Render("Local Status")
	var lines []string
	lines = append(lines, title)
	lines = append(lines, PanelDividerStyle.Render(strings.Repeat("─", statusPanelWidth-2)))

	if len(m.repoStatus) == 0 {
		lines = append(lines, PanelDimStyle.Render("  Scanning..."))
	}

	for _, s := range m.repoStatus {
		// Repo name
		short := s.Remote
		if i := strings.LastIndex(s.Remote, "/"); i >= 0 {
			short = s.Remote[i+1:]
		}

		if s.Error != nil {
			lines = append(lines, PanelRepoStyle.Render(short))
			lines = append(lines, PanelDimStyle.Render("  error"))
			lines = append(lines, "")
			continue
		}

		lines = append(lines, PanelRepoStyle.Render(short))

		// Branch
		branchLine := fmt.Sprintf("  ᛘ %s", s.Branch)
		lines = append(lines, PanelDimStyle.Render(branchLine))

		// Status indicators
		if s.Uncommitted == 0 && s.Unpushed == 0 {
			lines = append(lines, PanelCleanStyle.Render("  ✓ clean"))
		} else {
			if s.Uncommitted > 0 {
				lines = append(lines, PanelDirtyStyle.Render(
					fmt.Sprintf("  ● %d uncommitted", s.Uncommitted)))
			}
			if s.Unpushed > 0 {
				lines = append(lines, PanelWarnStyle.Render(
					fmt.Sprintf("  ↑ %d unpushed", s.Unpushed)))
			}
		}
		if !s.HasUpstream {
			lines = append(lines, PanelDimStyle.Render("  ⚠ no upstream"))
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
			lines = append(lines, ciLine)
		}

		lines = append(lines, "")
	}

	content := strings.Join(lines, "\n")
	return border.Render(content)
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

func renderEventLine(ev github.Event, now time.Time, flash bool, width int) string {
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

	if flash {
		line = FlashStyle.Width(width).Render(line)
	}

	return line
}

func pollEvents(cfg *config.Config) tea.Cmd {
	return func() tea.Msg {
		type result struct {
			events []github.Event
		}

		var wg sync.WaitGroup
		results := make([]result, len(cfg.Repos()))

		// Fetch all repos in parallel
		for idx, repo := range cfg.Repos() {
			wg.Add(1)
			go func(i int, r string) {
				defer wg.Done()
				events, err := github.FetchEvents(r, 20)
				if err != nil {
					return
				}

				// Enrich push events in parallel
				var ewg sync.WaitGroup
				for j := range events {
					if events[j].Type == "PushEvent" {
						ewg.Add(1)
						go func(e *github.Event) {
							defer ewg.Done()
							github.EnrichPushEvent(e)
						}(&events[j])
					}
				}
				ewg.Wait()

				results[i] = result{events: events}
			}(idx, repo)
		}
		wg.Wait()

		var all []github.Event
		for _, r := range results {
			all = append(all, r.events...)
		}
		return eventsMsg{events: all}
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
