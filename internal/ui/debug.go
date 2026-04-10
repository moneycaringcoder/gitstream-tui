package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	blit "github.com/blitui/blit"
)

// DebugOverlay shows API stats and recent log entries.
// Implements blit.Overlay.
type DebugOverlay struct {
	debugLog *DebugLog
	active   bool
	width    int
	height   int
	focused  bool
}

func NewDebugOverlay(debugLog *DebugLog) *DebugOverlay {
	return &DebugOverlay{debugLog: debugLog}
}

func (d *DebugOverlay) Init() tea.Cmd { return nil }

func (d *DebugOverlay) Update(msg tea.Msg, ctx blit.Context) (blit.Component, tea.Cmd) {
	return d, nil
}

func (d *DebugOverlay) View() string {
	var b strings.Builder

	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffffff")).
		PaddingLeft(1).Render("─── DEBUG LOG ───")
	b.WriteString(title + "\n\n")

	stats := d.debugLog.GetStats()
	b.WriteString(logStatsStyle.Render("  API Stats") + "\n")
	b.WriteString(logInfoStyle.Render(fmt.Sprintf("  Total calls:  %d", stats.TotalCalls)) + "\n")
	b.WriteString(logInfoStyle.Render(fmt.Sprintf("  Successful:   %d", stats.SuccessCalls)) + "\n")
	if stats.FailedCalls > 0 {
		b.WriteString(logErrorStyle.Render(fmt.Sprintf("  Failed:       %d", stats.FailedCalls)) + "\n")
	} else {
		b.WriteString(logInfoStyle.Render(fmt.Sprintf("  Failed:       %d", stats.FailedCalls)) + "\n")
	}
	b.WriteString(logInfoStyle.Render(fmt.Sprintf("  Total events: %d", stats.TotalEvents)) + "\n")
	if !stats.LastFetchAt.IsZero() {
		ago := time.Since(stats.LastFetchAt).Truncate(time.Second)
		b.WriteString(logInfoStyle.Render(fmt.Sprintf("  Last fetch:   %s ago", ago)) + "\n")
	}
	b.WriteString("\n")

	b.WriteString(logStatsStyle.Render("  Recent Log") + "\n")

	entries := d.debugLog.GetEntries()
	maxShow := d.height - 14
	if maxShow < 5 {
		maxShow = 5
	}
	start := 0
	if len(entries) > maxShow {
		start = len(entries) - maxShow
	}
	for i := len(entries) - 1; i >= start; i-- {
		e := entries[i]
		ts := logTimeStyle.Render(e.Time.Format("15:04:05"))
		var levelStyle lipgloss.Style
		var prefix string
		switch e.Level {
		case LogInfo:
			levelStyle = logInfoStyle
			prefix = "INFO"
		case LogWarn:
			levelStyle = logWarnStyle
			prefix = "WARN"
		case LogError:
			levelStyle = logErrorStyle
			prefix = "ERR "
		}
		line := fmt.Sprintf("  %s %s %s", ts, levelStyle.Render(prefix), levelStyle.Render(e.Message))
		b.WriteString(line + "\n")
	}

	if len(entries) == 0 {
		b.WriteString(logInfoStyle.Render("  No log entries yet") + "\n")
	}

	b.WriteString("\n")
	b.WriteString(HelpStyle.PaddingLeft(2).Render("esc close | q quit") + "\n")

	return b.String()
}

func (d *DebugOverlay) KeyBindings() []blit.KeyBind {
	return []blit.KeyBind{
		{Key: "D", Label: "Debug log", Group: "OTHER"},
	}
}

func (d *DebugOverlay) SetSize(w, h int)    { d.width = w; d.height = h }
func (d *DebugOverlay) Focused() bool       { return d.focused }
func (d *DebugOverlay) SetFocused(f bool)   { d.focused = f }
func (d *DebugOverlay) IsActive() bool      { return d.active }
func (d *DebugOverlay) SetActive(v bool)    { d.active = v }
func (d *DebugOverlay) Close()              { d.active = false }
