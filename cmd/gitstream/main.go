package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/moneycaringcoder/gitstream-tui/internal/config"
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
			for _, r := range cfg.Repos {
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

	if len(cfg.Repos) == 0 {
		fmt.Fprintln(os.Stderr, "No repos configured. Run 'gitstream add owner/repo' to get started.")
		os.Exit(1)
	}

	p := tea.NewProgram(
		ui.NewModel(cfg),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
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
  1-9           Filter by repo number
  0             Clear filter
  Up/Down       Scroll

Config: ~/.config/gitstream/config.yaml
`)
}
