package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	blit "github.com/blitui/blit"
	"github.com/moneycaringcoder/gitstream-tui/internal/config"
	"github.com/moneycaringcoder/gitstream-tui/internal/github"
	"github.com/moneycaringcoder/gitstream-tui/internal/ui"
	"github.com/moneycaringcoder/gitstream-tui/internal/updatewire"
)

var version = "dev"

func main() {
	args := os.Args[1:]

	// Handle subcommands
	if len(args) > 0 {
		switch args[0] {
		case "add":
			if len(args) < 2 {
				fmt.Fprintln(os.Stderr, "Usage: gitstream add owner/repo")
				os.Exit(1)
			}
			if err := config.AddRepo(args[1]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Added %s\n", args[1])
			return

		case "remove", "rm":
			if len(args) < 2 {
				fmt.Fprintln(os.Stderr, "Usage: gitstream remove owner/repo")
				os.Exit(1)
			}
			if err := config.RemoveRepo(args[1]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Removed %s\n", args[1])
			return

		case "list", "ls":
			cfg, err := config.Load()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			for _, r := range cfg.Repos() {
				fmt.Println(r)
			}
			return

		case "help", "--help", "-h":
			printHelp()
			return
		}
	}

	// Default: launch TUI
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintln(os.Stderr, "Run 'gitstream add owner/repo' to add a repo.")
		os.Exit(1)
	}

	if len(cfg.Repos()) == 0 {
		fmt.Fprintln(os.Stderr, "No repos configured. Run 'gitstream add owner/repo' to get started.")
		os.Exit(1)
	}

	blit.CleanupOldBinary()

	debugLog := ui.NewDebugLog()
	stream := ui.NewEventStream(cfg, debugLog)
	panel := ui.NewStatusPanel()
	debugOverlay := ui.NewDebugOverlay(debugLog)

	detailOverlay := blit.NewDetailOverlay(blit.DetailOverlayOpts[ui.DisplayEvent]{
		Title: "Event Detail",
		Render: func(de ui.DisplayEvent, w, h int, theme blit.Theme) string {
			return renderEventDetail(de, w)
		},
		OnKey: func(de ui.DisplayEvent, key tea.KeyMsg) tea.Cmd {
			if key.String() == "o" {
				if url := de.Event.URL(); url != "" {
					blit.OpenURL(url)
				}
				return blit.Consumed()
			}
			return nil
		},
		KeyBindings: []blit.KeyBind{
			{Key: "o", Label: "Open in browser", Group: "DETAIL"},
		},
	})
	stream.DetailOverlay = detailOverlay

	// Config editor using blit.ConfigEditor
	configEditor := blit.NewConfigEditor([]blit.ConfigField{
		{
			Label: "Interval (sec)",
			Group: "Polling",
			Hint:  "How often to poll GitHub for new events (min 5)",
			Get:   func() string { return strconv.Itoa(cfg.Interval) },
			Set: func(v string) error {
				n, err := strconv.Atoi(v)
				if err != nil || n < 5 {
					return fmt.Errorf("must be a number >= 5")
				}
				cfg.Interval = n
				config.Save(cfg)
				return nil
			},
		},
		{
			Label: "Add repo",
			Group: "Repos",
			Hint:  "Add a new repo to watch (owner/repo format)",
			Get:   func() string { return "" },
			Set: func(v string) error {
				v = strings.TrimSpace(v)
				if v == "" || !strings.Contains(v, "/") {
					return fmt.Errorf("must be owner/repo format")
				}
				for _, r := range cfg.RepoEntries {
					if r.Name == v {
						return fmt.Errorf("repo already exists")
					}
				}
				cfg.RepoEntries = append(cfg.RepoEntries, config.RepoEntry{Name: v})
				config.Save(cfg)
				return nil
			},
		},
		{
			Label: "Remove repo",
			Group: "Repos",
			Hint:  "Remove a watched repo (owner/repo format)",
			Get:   func() string { return "" },
			Set: func(v string) error {
				v = strings.TrimSpace(v)
				filtered := make([]config.RepoEntry, 0, len(cfg.RepoEntries))
				found := false
				for _, r := range cfg.RepoEntries {
					if r.Name == v {
						found = true
						continue
					}
					filtered = append(filtered, r)
				}
				if !found {
					return fmt.Errorf("repo not found")
				}
				cfg.RepoEntries = filtered
				config.Save(cfg)
				return nil
			},
		},
	})

	// Status bar: left = help hints, right = active filters
	statusLeft := func() string {
		return fmt.Sprintf(" ? help  s sort  t type  c config  D debug  p pause  r refresh  1-%d repo  0 clear",
			len(cfg.Repos()))
	}

	statusRight := func() string {
		var parts []string
		sortLabel := "oldest"
		if stream.IsNewestFirst() {
			sortLabel = "newest"
		}
		parts = append(parts, sortLabel+" first")
		if stream.RepoFilter() != "" {
			parts = append(parts, "repo:"+stream.RepoFilter())
		}
		if stream.TypeFilter() != "" {
			ev := github.Event{Type: stream.TypeFilter()}
			parts = append(parts, "type:"+ev.Label())
		}
		return strings.Join(parts, "  ") + " "
	}

	app := blit.NewApp(
		blit.WithTheme(blit.DefaultTheme()),
		blit.WithLayout(&blit.DualPane{
			Main:         stream,
			Side:         panel,
			MainName:     "Stream",
			SideName:     "Local",
			SideWidth:    32,
			MinMainWidth: 40,
			SideRight:    true,
			ToggleKey:    "",
		}),
		blit.WithStatusBar(statusLeft, statusRight),
		blit.WithHelp(),
		blit.WithOverlay("Settings", "c", configEditor),
		blit.WithOverlay("Debug", "D", debugOverlay),
		blit.WithOverlay("Detail", "", detailOverlay),
		// Global keybindings
		blit.WithKeyBind(blit.KeyBind{
			Key: "p", Label: "Pause/resume", Group: "CONTROLS",
			Handler: func() { stream.TogglePause() },
		}),
		blit.WithKeyBind(blit.KeyBind{
			Key: "r", Label: "Refresh now", Group: "CONTROLS",
			Handler: func() { stream.ForceRefresh() },
		}),
		blit.WithKeyBind(blit.KeyBind{
			Key: "s", Label: "Toggle sort", Group: "CONTROLS",
			Handler: func() { stream.ToggleSort() },
		}),
		blit.WithKeyBind(blit.KeyBind{
			Key: "t", Label: "Type filter →", Group: "FILTER",
			Handler: func() { stream.CycleTypeFilter(true) },
		}),
		blit.WithKeyBind(blit.KeyBind{
			Key: "T", Label: "Type filter ←", Group: "FILTER",
			Handler: func() { stream.CycleTypeFilter(false) },
		}),
		blit.WithKeyBind(blit.KeyBind{
			Key: "0", Label: "Clear filters", Group: "FILTER",
			Handler: func() { stream.ClearFilters() },
		}),
		blit.WithMouseSupport(),
		blit.WithTickInterval(time.Second),
		blit.WithAutoUpdate(updatewire.New(version)),
	)

	// Register repo number filters (1-9)
	for i := 1; i <= 9; i++ {
		idx := i - 1
		app.AddKeyBind(blit.KeyBind{
			Key: fmt.Sprintf("%d", i), Label: fmt.Sprintf("Filter repo %d", i), Group: "FILTER",
			Handler: func() {
				repos := cfg.Repos()
				if idx < len(repos) {
					repo := repos[idx]
					short := repo
					if j := strings.LastIndex(repo, "/"); j >= 0 {
						short = repo[j+1:]
					}
					stream.SetRepoFilter(short)
				}
			},
		})
	}

	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Print(`gitstream - live GitHub activity feed for your terminal

Usage:
  gitstream              Launch the TUI dashboard
  gitstream add <repo>   Add a repo to watch (owner/repo format)
  gitstream rm <repo>    Remove a repo
  gitstream ls           List watched repos
  gitstream help         Show this help

Keybindings (in TUI):
  q / Ctrl+C    Quit
  p             Pause/resume polling
  r             Refresh now
  s             Toggle sort order
  t / T         Cycle type filter
  1-9           Filter by repo number
  0             Clear filter
  c             Settings
  D             Debug log
  ?             Help
  Up/Down       Scroll
  Tab           Switch focus
  Enter         Event detail
  o             Open in browser

Config: ~/.config/gitstream/config.yaml
`)
}

func renderEventDetail(de ui.DisplayEvent, w int) string {
	ev := de.Event
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	valStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff"))
	color := ui.EventColor(ev.Type)
	typeStyle := lipgloss.NewStyle().Foreground(color).Bold(true)

	lines := []string{
		labelStyle.Render("Type:    ") + typeStyle.Render(ev.Label()),
		labelStyle.Render("Repo:    ") + valStyle.Render(ev.Repo.Name),
		labelStyle.Render("Actor:   ") + valStyle.Render(ev.Actor.Login),
		labelStyle.Render("Time:    ") + valStyle.Render(ev.CreatedAt.Local().Format("2006-01-02 15:04:05")),
	}

	if url := ev.URL(); url != "" {
		lines = append(lines, labelStyle.Render("URL:     ")+valStyle.Render(url))
	}

	lines = append(lines, "")

	detail := ev.Detail()
	if detail != "" {
		lines = append(lines, labelStyle.Render("Detail:"))
		// Word-wrap detail to width
		maxW := w - 2
		if maxW < 20 {
			maxW = 20
		}
		for len(detail) > maxW {
			lines = append(lines, "  "+valStyle.Render(detail[:maxW]))
			detail = detail[maxW:]
		}
		if detail != "" {
			lines = append(lines, "  "+valStyle.Render(detail))
		}
	}

	// Show commits for push events
	if len(ev.Payload.Commits) > 0 {
		lines = append(lines, "")
		lines = append(lines, labelStyle.Render("Commits:"))
		for _, c := range ev.Payload.Commits {
			sha := c.SHA
			if len(sha) > 7 {
				sha = sha[:7]
			}
			msg := c.Message
			if idx := strings.Index(msg, "\n"); idx > 0 {
				msg = msg[:idx]
			}
			maxMsg := w - 12
			if maxMsg > 0 && len(msg) > maxMsg {
				msg = msg[:maxMsg-1] + "…"
			}
			lines = append(lines, "  "+lipgloss.NewStyle().Foreground(lipgloss.Color("#ffaa00")).Render(sha)+" "+valStyle.Render(msg))
		}
	}

	// PR info
	if pr := ev.Payload.PullRequest; pr != nil {
		lines = append(lines, "")
		lines = append(lines, labelStyle.Render("PR:      ")+valStyle.Render(fmt.Sprintf("#%d %s", pr.Number, pr.Title)))
		lines = append(lines, labelStyle.Render("State:   ")+valStyle.Render(pr.State))
	}

	// Issue info
	if issue := ev.Payload.Issue; issue != nil {
		lines = append(lines, "")
		lines = append(lines, labelStyle.Render("Issue:   ")+valStyle.Render(fmt.Sprintf("#%d %s", issue.Number, issue.Title)))
	}

	// Release info
	if rel := ev.Payload.Release; rel != nil {
		lines = append(lines, "")
		lines = append(lines, labelStyle.Render("Release: ")+valStyle.Render(rel.TagName+" — "+rel.Name))
	}

	// Compare data (diff stats)
	if cd := ev.CompareData; cd != nil {
		lines = append(lines, "")
		lines = append(lines, labelStyle.Render(fmt.Sprintf("Files changed: %d, Commits: %d", len(cd.Files), cd.TotalCommits)))
		for _, f := range cd.Files {
			adds := lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e")).Render(fmt.Sprintf("+%d", f.Additions))
			dels := lipgloss.NewStyle().Foreground(lipgloss.Color("#ef4444")).Render(fmt.Sprintf("-%d", f.Deletions))
			lines = append(lines, "  "+adds+" "+dels+" "+valStyle.Render(f.Filename))
		}
	}

	return strings.Join(lines, "\n")
}
