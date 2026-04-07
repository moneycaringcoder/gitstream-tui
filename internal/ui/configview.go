package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/moneycaringcoder/gitstream-tui/internal/config"
)

// configField describes an editable config field.
type configField struct {
	group string
	label string
	hint  string
	get   func(*config.Config) string
	set   func(*config.Config, string) error
}

var configFields = []configField{
	{
		group: "Polling",
		label: "Interval (sec)",
		hint:  "How often to poll GitHub for new events",
		get:   func(c *config.Config) string { return strconv.Itoa(c.Interval) },
		set: func(c *config.Config, v string) error {
			n, err := strconv.Atoi(v)
			if err != nil || n < 5 {
				return fmt.Errorf("must be a number >= 5")
			}
			c.Interval = n
			return nil
		},
	},
	{
		group: "Repos",
		label: "Add repo",
		hint:  "Add a new repo to watch (owner/repo format)",
		get:   func(c *config.Config) string { return "" },
		set: func(c *config.Config, v string) error {
			v = strings.TrimSpace(v)
			if v == "" || !strings.Contains(v, "/") {
				return fmt.Errorf("must be owner/repo format")
			}
			for _, r := range c.RepoEntries {
				if r.Name == v {
					return fmt.Errorf("repo already exists")
				}
			}
			c.RepoEntries = append(c.RepoEntries, config.RepoEntry{Name: v})
			return nil
		},
	},
	{
		group: "Repos",
		label: "Remove repo",
		hint:  "Remove a watched repo (owner/repo format)",
		get:   func(c *config.Config) string { return "" },
		set: func(c *config.Config, v string) error {
			v = strings.TrimSpace(v)
			filtered := make([]config.RepoEntry, 0, len(c.RepoEntries))
			found := false
			for _, r := range c.RepoEntries {
				if r.Name == v {
					found = true
					continue
				}
				filtered = append(filtered, r)
			}
			if !found {
				return fmt.Errorf("repo not found")
			}
			c.RepoEntries = filtered
			return nil
		},
	},
}

// configState tracks the config editor state.
type configState struct {
	active      bool
	cursor      int
	editing     bool
	editBuf     string
	editErr     string
	dirty       bool
	savedNotice int
}

// renderConfigView renders the config editor overlay.
func (m Model) renderConfigView() string {
	var b strings.Builder

	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffffff")).
		PaddingLeft(1).Render("─── SETTINGS ───")
	b.WriteString(title + "\n\n")

	// Show current repos
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#3b82f6")).
		PaddingLeft(2).Render("Watched Repos") + "\n")
	for i, r := range m.cfg.RepoEntries {
		prefix := fmt.Sprintf("  %d. ", i+1)
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#9ca3af")).
			Render(prefix+r.Name) + "\n")
	}
	b.WriteString("\n")

	// Render fields
	lastGroup := ""
	for i, f := range configFields {
		if f.group != lastGroup {
			lastGroup = f.group
			groupStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#3b82f6")).PaddingLeft(2)
			b.WriteString(groupStyle.Render(strings.ToUpper(f.group)) + "\n")
		}

		label := fmt.Sprintf("%-18s", f.label)
		val := f.get(m.cfg)

		isCursor := i == m.configUI.cursor

		if isCursor && m.configUI.editing {
			// Show edit buffer with cursor
			valStr := m.configUI.editBuf + "█"
			row := lipgloss.NewStyle().PaddingLeft(2).
				Background(lipgloss.Color("#1e3a5f")).
				Foreground(lipgloss.Color("#ffffff")).
				Render(fmt.Sprintf("  %s  %s", label, valStr))
			b.WriteString(row + "\n")
			if m.configUI.editErr != "" {
				b.WriteString(lipgloss.NewStyle().PaddingLeft(4).
					Foreground(lipgloss.Color("#ef4444")).
					Render(m.configUI.editErr) + "\n")
			}
		} else if isCursor {
			row := lipgloss.NewStyle().PaddingLeft(2).
				Background(lipgloss.Color("#1e3a5f")).
				Foreground(lipgloss.Color("#ffffff")).
				Render(fmt.Sprintf("▸ %s  %s", label, val))
			b.WriteString(row + "\n")
			// Show hint
			b.WriteString(lipgloss.NewStyle().PaddingLeft(6).
				Foreground(ColorDim).Italic(true).
				Render(f.hint) + "\n")
		} else {
			row := lipgloss.NewStyle().PaddingLeft(2).
				Foreground(lipgloss.Color("#d1d5db")).
				Render(fmt.Sprintf("  %s  %s", label, val))
			b.WriteString(row + "\n")
		}
	}

	b.WriteString("\n")

	// Footer
	var footerParts []string
	if m.configUI.editing {
		footerParts = append(footerParts, "enter save", "esc cancel")
	} else {
		footerParts = append(footerParts, "j/k navigate", "enter edit", "ctrl+s save", "esc/c close")
	}
	if m.configUI.dirty {
		footerParts = append(footerParts, "unsaved changes")
	}
	if m.configUI.savedNotice > 0 {
		footerParts = append(footerParts, "saved!")
	}
	footer := HelpStyle.PaddingLeft(2).Render(strings.Join(footerParts, " | "))
	b.WriteString(footer + "\n")

	return b.String()
}
