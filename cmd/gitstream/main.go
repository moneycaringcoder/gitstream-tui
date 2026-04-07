package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	tuikit "github.com/moneycaringcoder/tuikit-go"
	"github.com/moneycaringcoder/gitstream-tui/internal/config"
	"github.com/moneycaringcoder/gitstream-tui/internal/github"
	"github.com/moneycaringcoder/gitstream-tui/internal/ui"
)

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

	debugLog := ui.NewDebugLog()
	stream := ui.NewEventStream(cfg, debugLog)
	panel := ui.NewStatusPanel()
	debugOverlay := ui.NewDebugOverlay(debugLog)

	// Config editor using tuikit.ConfigEditor
	configEditor := tuikit.NewConfigEditor([]tuikit.ConfigField{
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

	app := tuikit.NewApp(
		tuikit.WithTheme(tuikit.DefaultTheme()),
		tuikit.WithLayout(&tuikit.DualPane{
			Main:         stream,
			Side:         panel,
			SideWidth:    32,
			MinMainWidth: 40,
			SideRight:    true,
			ToggleKey:    "",
		}),
		tuikit.WithStatusBar(statusLeft, statusRight),
		tuikit.WithHelp(),
		tuikit.WithOverlay("Settings", "c", configEditor),
		tuikit.WithOverlay("Debug", "D", debugOverlay),
		// Global keybindings
		tuikit.WithKeyBind(tuikit.KeyBind{
			Key: "p", Label: "Pause/resume", Group: "CONTROLS",
			Handler: func() { stream.TogglePause() },
		}),
		tuikit.WithKeyBind(tuikit.KeyBind{
			Key: "r", Label: "Refresh now", Group: "CONTROLS",
			Handler: func() { stream.ForceRefresh() },
		}),
		tuikit.WithKeyBind(tuikit.KeyBind{
			Key: "s", Label: "Toggle sort", Group: "CONTROLS",
			Handler: func() { stream.ToggleSort() },
		}),
		tuikit.WithKeyBind(tuikit.KeyBind{
			Key: "t", Label: "Type filter →", Group: "FILTER",
			Handler: func() { stream.CycleTypeFilter(true) },
		}),
		tuikit.WithKeyBind(tuikit.KeyBind{
			Key: "T", Label: "Type filter ←", Group: "FILTER",
			Handler: func() { stream.CycleTypeFilter(false) },
		}),
		tuikit.WithKeyBind(tuikit.KeyBind{
			Key: "0", Label: "Clear filters", Group: "FILTER",
			Handler: func() { stream.ClearFilters() },
		}),
		tuikit.WithMouseSupport(),
		tuikit.WithTickInterval(time.Second),
	)

	// Register repo number filters (1-9)
	for i := 1; i <= 9; i++ {
		idx := i - 1
		app.AddKeyBind(tuikit.KeyBind{
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
  Enter         Open in browser

Config: ~/.config/gitstream/config.yaml
`)
}
