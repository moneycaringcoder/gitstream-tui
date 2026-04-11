package main

import (
	"fmt"
	"os"
	"strings"
	"time"

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

	// Build theme picker for runtime switching
	presets := blit.Presets()
	var pickerItems []blit.PickerItem
	for name := range presets {
		n := name
		pickerItems = append(pickerItems, blit.PickerItem{
			Title: n,
			Value: n,
		})
	}
	themePicker := blit.NewPicker(pickerItems, blit.PickerOpts{
		Placeholder: "Search themes...",
		OnConfirm: func(item blit.PickerItem) {
			name := item.Value.(string)
			if _, ok := presets[name]; ok {
				cfg.Theme = name
				config.Save(cfg)
			}
		},
	})

	debugLog := ui.NewDebugLog()
	stream := ui.NewEventStream(cfg, debugLog)
	panel := ui.NewStatusPanel()
	debugOverlay := ui.NewDebugOverlay(debugLog)

	detailOverlay := blit.NewDetailOverlay(blit.DetailOverlayOpts[ui.DisplayEvent]{
		Title: "Event Detail",
		Render: func(de ui.DisplayEvent, w, h int, theme blit.Theme) string {
			return renderEventDetail(de, w, theme)
		},
		OnKey: func(de ui.DisplayEvent, key blit.KeyMsg) blit.Cmd {
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

	// Config editor auto-generated from blit struct tags
	configEditor := config.Editor()

	// Signal-driven status bar. Set() is called via goroutine to avoid
	// deadlocking — bubbletea's p.msgs is unbuffered, and Signal.Set triggers
	// bus.schedule → p.Send from the UI goroutine which would block forever.
	leftSig := blit.NewSignal(
		" ? help  s sort  t type  c config  D debug  p pause  r refresh  y copy  1-5 tab  0 clear")
	rightSig := blit.NewSignal[string]("")
	updateStatusRight := func() {
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
		v := strings.Join(parts, "  ") + " "
		go rightSig.Set(v)
	}
	updateStatusRight()

	// Vim-style command bar
	cmdBar := blit.NewCommandBar([]blit.Command{
		{
			Name: "add", Args: true, Hint: "Add a repo (owner/repo)",
			Run: func(args string) blit.Cmd {
				args = strings.TrimSpace(args)
				if args == "" || !strings.Contains(args, "/") {
					return nil
				}
				cfg.RepoEntries = append(cfg.RepoEntries, config.RepoEntry{Name: args})
				config.Save(cfg)
				return nil
			},
		},
		{
			Name: "remove", Aliases: []string{"rm"}, Args: true, Hint: "Remove a repo",
			Run: func(args string) blit.Cmd {
				args = strings.TrimSpace(args)
				filtered := make([]config.RepoEntry, 0, len(cfg.RepoEntries))
				for _, r := range cfg.RepoEntries {
					if r.Name != args {
						filtered = append(filtered, r)
					}
				}
				cfg.RepoEntries = filtered
				config.Save(cfg)
				return nil
			},
		},
		{
			Name: "sort", Args: true, Hint: "Sort newest|oldest",
			Run: func(args string) blit.Cmd {
				args = strings.TrimSpace(args)
				if args == "newest" && !stream.IsNewestFirst() {
					stream.ToggleSort()
					updateStatusRight()
				} else if args == "oldest" && stream.IsNewestFirst() {
					stream.ToggleSort()
					updateStatusRight()
				}
				return nil
			},
		},
		{
			Name: "filter", Args: true, Hint: "filter repo:<name> or type:<type>",
			Run: func(args string) blit.Cmd {
				args = strings.TrimSpace(args)
				if strings.HasPrefix(args, "repo:") {
					stream.SetRepoFilter(strings.TrimPrefix(args, "repo:"))
					updateStatusRight()
				} else if strings.HasPrefix(args, "type:") {
					stream.SetTypeFilter(strings.TrimPrefix(args, "type:"))
					updateStatusRight()
				}
				return nil
			},
		},
		{
			Name: "clear", Hint: "Clear all filters",
			Run: func(_ string) blit.Cmd {
				stream.ClearFilters()
				updateStatusRight()
				return nil
			},
		},
		{
			Name: "theme", Args: true, Hint: "Set theme by name",
			Run: func(args string) blit.Cmd {
				args = strings.TrimSpace(args)
				if t, ok := presets[args]; ok {
					cfg.Theme = args
					config.Save(cfg)
					return blit.SetThemeCmd(t)
				}
				return nil
			},
		},
		{
			Name: "quit", Aliases: []string{"q"}, Hint: "Quit",
			Run: func(_ string) blit.Cmd { return blit.Quit },
		},
	})

	// Tab bar for event type filtering.
	// Stream is assigned as Content only to the initial tab; OnChange moves
	// it into the newly active slot so Tabs.SetFocused doesn't clobber
	// the shared stream's focus state (last-writer-wins across 5 items).
	tabItems := []blit.TabItem{
		{Title: "All", Glyph: "◉", Content: stream},
		{Title: "Pushes", Glyph: "↑"},
		{Title: "PRs", Glyph: "⎇"},
		{Title: "Issues", Glyph: "!"},
		{Title: "Local", Glyph: "⌂"},
	}
	tabs := blit.NewTabs(tabItems, blit.TabsOpts{
		OnChange: func(idx int) {
			filters := []string{"", "PushEvent", "PullRequestEvent", "IssuesEvent", "LocalPushEvent"}
			if idx < len(filters) {
				stream.SetTypeFilter(filters[idx])
				updateStatusRight()
			}
			// Move stream into the active tab, clear the rest.
			for i := range tabItems {
				if i == idx {
					tabItems[i].Content = stream
				} else {
					tabItems[i].Content = nil
				}
			}
		},
	})

	var app *blit.App
	app = blit.NewApp(
		blit.WithTheme(resolveTheme(cfg.Theme)),
		blit.WithLayout(&blit.DualPane{
			Main:         tabs,
			Side:         panel,
			MainName:     "Stream",
			SideName:     "Local",
			SideWidth:    32,
			MinMainWidth: 40,
			SideRight:    true,
			ToggleKey:    "",
		}),
		blit.WithStatusBarSignal(leftSig, rightSig),
		blit.WithHelp(),
		blit.WithSlotOverlay("Settings", "c", configEditor),
		blit.WithSlotOverlay("Debug", "D", debugOverlay),
		blit.WithSlotOverlay("Detail", "", detailOverlay),
		blit.WithSlotOverlay("Theme", "ctrl+t", themePicker),
		blit.WithSlotOverlay("Command", ":", cmdBar),
		// Global keybindings
		blit.WithKeyBind(blit.KeyBind{
			Key: "p", Label: "Pause/resume", Group: "CONTROLS",
			Handler: func() {
				stream.TogglePause()
				updateStatusRight()
				if stream.IsPaused() {
					app.Send(blit.ToastMsg{Severity: blit.SeverityInfo, Title: "Paused", Duration: 3 * time.Second})
				} else {
					app.Send(blit.ToastMsg{Severity: blit.SeverityInfo, Title: "Resumed", Duration: 3 * time.Second})
				}
			},
		}),
		blit.WithKeyBind(blit.KeyBind{
			Key: "r", Label: "Refresh now", Group: "CONTROLS",
			Handler: func() {
				stream.ForceRefresh()
				app.Send(blit.ToastMsg{Severity: blit.SeverityInfo, Title: "Refreshing…", Duration: 3 * time.Second})
			},
		}),
		blit.WithKeyBind(blit.KeyBind{
			Key: "s", Label: "Toggle sort", Group: "CONTROLS",
			Handler: func() {
				stream.ToggleSort()
				updateStatusRight()
				if stream.IsNewestFirst() {
					app.Send(blit.ToastMsg{Severity: blit.SeverityInfo, Title: "Newest first", Duration: 3 * time.Second})
				} else {
					app.Send(blit.ToastMsg{Severity: blit.SeverityInfo, Title: "Oldest first", Duration: 3 * time.Second})
				}
			},
		}),
		blit.WithKeyBind(blit.KeyBind{
			Key: "t", Label: "Type filter →", Group: "FILTER",
			Handler: func() {
				stream.CycleTypeFilter(true)
				updateStatusRight()
				if f := stream.TypeFilter(); f != "" {
					ev := github.Event{Type: f}
					app.Send(blit.ToastMsg{Severity: blit.SeverityInfo, Title: "Filter: " + ev.Label(), Duration: 3 * time.Second})
				} else {
					app.Send(blit.ToastMsg{Severity: blit.SeverityInfo, Title: "Filter: All", Duration: 3 * time.Second})
				}
			},
		}),
		blit.WithKeyBind(blit.KeyBind{
			Key: "T", Label: "Type filter ←", Group: "FILTER",
			Handler: func() {
				stream.CycleTypeFilter(false)
				updateStatusRight()
				if f := stream.TypeFilter(); f != "" {
					ev := github.Event{Type: f}
					app.Send(blit.ToastMsg{Severity: blit.SeverityInfo, Title: "Filter: " + ev.Label(), Duration: 3 * time.Second})
				} else {
					app.Send(blit.ToastMsg{Severity: blit.SeverityInfo, Title: "Filter: All", Duration: 3 * time.Second})
				}
			},
		}),
		blit.WithKeyBind(blit.KeyBind{
			Key: "0", Label: "Clear filters", Group: "FILTER",
			Handler: func() {
				stream.ClearFilters()
				updateStatusRight()
				app.Send(blit.ToastMsg{Severity: blit.SeverityInfo, Title: "Filters cleared", Duration: 3 * time.Second})
			},
		}),
		blit.WithMouseSupport(),
		blit.WithTickInterval(200*time.Millisecond),
		blit.WithAutoUpdate(updatewire.New(version)),
		blit.WithDevConsole(),
		blit.WithAnimations(true),
	)

	action, err := app.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if action == blit.UpdateActionRestart {
		fmt.Fprintln(os.Stderr, "Update installed. Please restart gitstream.")
		os.Exit(0)
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
  y             Copy URL to clipboard

Config: ~/.config/gitstream/config.yaml
`)
}

func renderEventDetail(de ui.DisplayEvent, w int, theme blit.Theme) string {
	ev := de.Event

	// Breadcrumb trail: gitstream > repo > event type
	bc := blit.NewBreadcrumbs([]string{"gitstream", ev.Repo.Name, ev.Label()})
	bc.SetSize(w, 1)
	bc.SetTheme(theme)

	labelStyle := blit.NewStyle().Foreground(theme.Muted)
	valStyle := blit.NewStyle().Foreground(theme.Text)
	color := ui.EventColor(ev.Type, theme)
	typeStyle := blit.NewStyle().Foreground(color).Bold(true)

	lines := []string{
		bc.View(),
		"",
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
		shaStyle := blit.NewStyle().Foreground(theme.Warn)
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
			lines = append(lines, "  "+shaStyle.Render(sha)+" "+valStyle.Render(msg))
		}
	}

	// PR info
	if pr := ev.Payload.PullRequest; pr != nil {
		lines = append(lines, "")
		lines = append(lines, labelStyle.Render("PR:      ")+valStyle.Render(fmt.Sprintf("#%d %s", pr.Number, pr.Title)))
		lines = append(lines, labelStyle.Render("State:   ")+valStyle.Render(pr.State))
		if pr.Body != "" {
			lines = append(lines, "")
			lines = append(lines, labelStyle.Render("Description:"))
			lines = append(lines, blit.Markdown(pr.Body, theme))
		}
	}

	// Issue info
	if issue := ev.Payload.Issue; issue != nil {
		lines = append(lines, "")
		lines = append(lines, labelStyle.Render("Issue:   ")+valStyle.Render(fmt.Sprintf("#%d %s", issue.Number, issue.Title)))
		if issue.Body != "" {
			lines = append(lines, "")
			lines = append(lines, labelStyle.Render("Description:"))
			lines = append(lines, blit.Markdown(issue.Body, theme))
		}
	}

	// Release info
	if rel := ev.Payload.Release; rel != nil {
		lines = append(lines, "")
		lines = append(lines, labelStyle.Render("Release: ")+valStyle.Render(rel.TagName+" — "+rel.Name))
		if rel.Body != "" {
			lines = append(lines, "")
			lines = append(lines, labelStyle.Render("Release Notes:"))
			lines = append(lines, blit.Markdown(rel.Body, theme))
		}
	}

	// Compare data (diff stats)
	if cd := ev.CompareData; cd != nil {
		lines = append(lines, "")
		lines = append(lines, labelStyle.Render(fmt.Sprintf("Files changed: %d, Commits: %d", len(cd.Files), cd.TotalCommits)))
		addStyle := blit.NewStyle().Foreground(theme.Positive)
		delStyle := blit.NewStyle().Foreground(theme.Negative)
		for _, f := range cd.Files {
			adds := addStyle.Render(fmt.Sprintf("+%d", f.Additions))
			dels := delStyle.Render(fmt.Sprintf("-%d", f.Deletions))
			lines = append(lines, "  "+adds+" "+dels+" "+valStyle.Render(f.Filename))
		}
	}

	return strings.Join(lines, "\n")
}

func resolveTheme(name string) blit.Theme {
	if name != "" {
		presets := blit.Presets()
		if t, ok := presets[name]; ok {
			return t
		}
	}
	t := blit.DefaultTheme()
	t.Extra = map[string]blit.Color{
		"info":    "#06b6d4",
		"create":  "#22c55e",
		"delete":  "#ef4444",
		"review":  "#a855f7",
		"comment": "#6b7280",
		"issue":   "#eab308",
		"release": "#f97316",
		"local":   "#a78bfa",
	}
	return t
}
