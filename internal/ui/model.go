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
	"github.com/moneycaringcoder/gitstream-tui/internal/github"
)

const flashDuration = 3 * time.Second

// DisplayEvent holds a parsed event for display.
type DisplayEvent struct {
	Event   github.Event
	AddedAt time.Time
}

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
	newestFirst bool   // sort order: true = newest on top
	firstPoll   bool   // true after first poll completes
}

// Messages
type tickMsg struct{}
type uiTickMsg struct{}
type eventsMsg struct {
	events []github.Event
	err    error
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
			if idx < len(m.cfg.Repos) {
				repo := m.cfg.Repos[idx]
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
			m.rebuildViewport()
		case "s":
			m.newestFirst = !m.newestFirst
			m.rebuildViewport()
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		headerHeight := 4
		footerHeight := 2
		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-headerHeight-footerHeight)
			m.viewport.YPosition = headerHeight
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
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

	case uiTickMsg:
		m.rebuildViewport()
		return m, uiTickCmd()

	case tickMsg:
		cmds := []tea.Cmd{tickCmd(time.Duration(m.cfg.Interval) * time.Second)}
		if !m.paused {
			cmds = append(cmds, pollEvents(m.cfg))
		}
		return m, tea.Batch(cmds...)
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m *Model) rebuildViewport() {
	now := time.Now()
	var lines []string
	if m.newestFirst {
		for i := len(m.events) - 1; i >= 0; i-- {
			de := m.events[i]
			if m.filter != "" && de.Event.ShortRepo() != m.filter {
				continue
			}
			flash := !de.AddedAt.IsZero() && now.Before(de.AddedAt.Add(flashDuration))
			lines = append(lines, renderEventLine(de.Event, now, flash, m.width))
		}
	} else {
		for _, de := range m.events {
			if m.filter != "" && de.Event.ShortRepo() != m.filter {
				continue
			}
			flash := !de.AddedAt.IsZero() && now.Before(de.AddedAt.Add(flashDuration))
			lines = append(lines, renderEventLine(de.Event, now, flash, m.width))
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
	repoList := SubtitleStyle.Render(fmt.Sprintf("Watching: %s", strings.Join(m.cfg.Repos, ", ")))

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
		extra += fmt.Sprintf(" | filtered: %s", m.filter)
	}
	sortLabel := "oldest first"
	if m.newestFirst {
		sortLabel = "newest first"
	}
	help := HelpStyle.PaddingLeft(1).Render(
		fmt.Sprintf("q quit | p pause | r refresh | s sort (%s) | 1-%d filter | 0 clear%s",
			sortLabel, len(m.cfg.Repos), extra))

	// Compose
	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		m.viewport.View(),
		"",
		help,
	)
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

func renderEventLine(ev github.Event, now time.Time, flash bool, width int) string {
	t := ev.CreatedAt.Local().Format("15:04:05")
	rel := relativeTime(ev.CreatedAt, now)
	timeStr := fmt.Sprintf("%s %s", t, rel)
	label := ev.Label()
	detail := ev.Detail()
	actor := ev.Actor.Login
	repo := ev.ShortRepo()

	line := fmt.Sprintf("%s  %s %s %s %s",
		TimeStyle.Render(timeStr),
		RepoStyle.Render(repo),
		LabelStyle(ev.Type).Render(label),
		ActorStyle.Render(actor),
		DetailStyle.Render(detail),
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
		results := make([]result, len(cfg.Repos))

		// Fetch all repos in parallel
		for idx, repo := range cfg.Repos {
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
