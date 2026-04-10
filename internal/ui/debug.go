package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	blit "github.com/blitui/blit"
)

// DebugOverlay shows API stats and recent log entries using blit.LogViewer.
// Implements blit.Component and blit.Overlay.
type DebugOverlay struct {
	logViewer *blit.LogViewer
	debugLog  *DebugLog
	active    bool
	width     int
	height    int
	focused   bool
	theme     blit.Theme
}

func NewDebugOverlay(debugLog *DebugLog) *DebugOverlay {
	lv := blit.NewLogViewer()
	debugLog.SetLogViewer(lv)
	return &DebugOverlay{
		logViewer: lv,
		debugLog:  debugLog,
		theme:     blit.DefaultTheme(),
	}
}

func (d *DebugOverlay) Init() tea.Cmd { return nil }

func (d *DebugOverlay) Update(msg tea.Msg, ctx blit.Context) (blit.Component, tea.Cmd) {
	comp, cmd := d.logViewer.Update(msg, ctx)
	if lv, ok := comp.(*blit.LogViewer); ok {
		d.logViewer = lv
	}
	return d, cmd
}

func (d *DebugOverlay) View() string {
	var b strings.Builder
	th := d.theme

	title := lipgloss.NewStyle().Bold(true).Foreground(th.Text).
		PaddingLeft(1).Render(blit.Divider(d.width, th))
	b.WriteString(title + "\n\n")

	stats := d.debugLog.GetStats()

	statsHeader := lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
	dim := lipgloss.NewStyle().Foreground(th.Muted)
	errStyle := lipgloss.NewStyle().Foreground(th.Negative)

	b.WriteString(statsHeader.Render("  API Stats") + "\n")
	b.WriteString(dim.Render(fmt.Sprintf("  Total calls:  %d", stats.TotalCalls)) + "\n")
	b.WriteString(dim.Render(fmt.Sprintf("  Successful:   %d", stats.SuccessCalls)) + "\n")
	if stats.FailedCalls > 0 {
		b.WriteString(errStyle.Render(fmt.Sprintf("  Failed:       %d", stats.FailedCalls)) + "\n")
	} else {
		b.WriteString(dim.Render(fmt.Sprintf("  Failed:       %d", stats.FailedCalls)) + "\n")
	}
	b.WriteString(dim.Render(fmt.Sprintf("  Total events: %d", stats.TotalEvents)) + "\n")
	if !stats.LastFetchAt.IsZero() {
		ago := time.Since(stats.LastFetchAt).Truncate(time.Second)
		b.WriteString(dim.Render(fmt.Sprintf("  Last fetch:   %s ago", ago)) + "\n")
	}

	// Repo health
	if len(stats.RepoHealth) > 0 {
		b.WriteString("\n")
		b.WriteString(statsHeader.Render("  Repo Health") + "\n")
		for repo, h := range stats.RepoHealth {
			var badge string
			if h.LastSuccess {
				badge = blit.Badge("OK", th.Positive, true)
			} else {
				badge = blit.Badge("FAIL", th.Negative, true)
			}
			b.WriteString(fmt.Sprintf("  %s %s", badge, dim.Render(repo)) + "\n")
		}
	}

	// Rate limit
	if stats.RateLimit > 0 {
		b.WriteString(dim.Render(fmt.Sprintf("  Rate limit:   %d/%d", stats.RateRemain, stats.RateLimit)) + "\n")
	}

	b.WriteString("\n")
	b.WriteString(d.logViewer.View())

	return b.String()
}

func (d *DebugOverlay) KeyBindings() []blit.KeyBind {
	return d.logViewer.KeyBindings()
}

func (d *DebugOverlay) SetSize(w, h int) {
	d.width = w
	d.height = h
	// Reserve lines for the stats header; give the rest to the log viewer
	headerLines := 12
	lvHeight := h - headerLines
	if lvHeight < 4 {
		lvHeight = 4
	}
	d.logViewer.SetSize(w, lvHeight)
}

func (d *DebugOverlay) Focused() bool     { return d.focused }
func (d *DebugOverlay) SetFocused(f bool)  { d.focused = f; d.logViewer.SetFocused(f) }
func (d *DebugOverlay) SetTheme(t blit.Theme) { d.theme = t; d.logViewer.SetTheme(t) }
func (d *DebugOverlay) IsActive() bool     { return d.active }
func (d *DebugOverlay) SetActive(v bool)   { d.active = v }
func (d *DebugOverlay) Close()             { d.active = false }
