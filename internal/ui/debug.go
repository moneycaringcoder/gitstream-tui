package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	blit "github.com/blitui/blit"
	"github.com/blitui/blit/charts"
)

// DebugOverlay shows API stats and recent log entries using blit.LogViewer.
// Implements blit.Component and blit.Overlay. Renders as a full-screen modal.
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

// View renders the debug overlay as a full-screen bordered modal.
func (d *DebugOverlay) View() string {
	th := d.theme

	// Available text area inside border(2) + padding(2)
	textW := d.width - 4
	if textW < 20 {
		textW = 20
	}
	// Total content lines available inside border(2)
	maxLines := d.height - 2
	if maxLines < 6 {
		maxLines = 6
	}

	// Render the stats section first so we know how tall it is
	statsStr := d.renderStats(textW)
	statsLines := strings.Count(statsStr, "\n") + 1

	// Give the remaining lines to the log viewer (minimum 4)
	// Account for divider (2 lines: divider + blank)
	lvLines := maxLines - statsLines - 2
	if lvLines < 4 {
		lvLines = 4
	}
	d.logViewer.SetSize(textW, lvLines)

	// Assemble: stats + divider + log viewer
	var b strings.Builder
	b.WriteString(statsStr)
	b.WriteString("\n")
	b.WriteString(blit.Divider(textW, th))
	b.WriteString("\n")
	b.WriteString(d.logViewer.View())
	content := b.String()

	// Truncate content to maxLines so it can't overflow the box
	cLines := strings.Split(content, "\n")
	if len(cLines) > maxLines {
		cLines = cLines[:maxLines]
	}
	content = strings.Join(cLines, "\n")

	// Render the bordered box
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(th.Accent).
		Render(" Debug Console ")

	box := lipgloss.NewStyle().
		Width(textW + 2). // +2 for padding(0,1)
		Border(lipgloss.RoundedBorder()).
		BorderForeground(th.Border).
		Foreground(th.Text).
		Padding(0, 1)

	rendered := box.Render(content)

	// Inject title into the top border
	lines := strings.Split(rendered, "\n")
	if len(lines) > 0 {
		borderWidth := lipgloss.Width(lines[0])
		titleWidth := lipgloss.Width(title)
		if titleWidth+4 < borderWidth {
			pos := (borderWidth - titleWidth) / 2
			runes := []rune(lines[0])
			if pos+titleWidth <= len(runes) {
				lines[0] = string(runes[:pos]) + title + string(runes[pos+titleWidth:])
			}
		}
	}

	// Hard-clamp to terminal dimensions and pad to fill exactly
	if len(lines) > d.height {
		lines = lines[:d.height]
	}
	for len(lines) < d.height {
		lines = append(lines, "")
	}
	for i, line := range lines {
		if ansi.StringWidth(line) > d.width {
			lines[i] = ansi.Truncate(line, d.width, "")
		}
	}

	return strings.Join(lines, "\n")
}

// renderStats builds the stats section (API stats, repo health, rate limit, bar chart).
func (d *DebugOverlay) renderStats(textW int) string {
	var b strings.Builder
	th := d.theme
	stats := d.debugLog.GetStats()

	statsHeader := lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
	dim := lipgloss.NewStyle().Foreground(th.Muted)
	errStyle := lipgloss.NewStyle().Foreground(th.Negative)

	b.WriteString(statsHeader.Render("API Stats") + "\n")
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

	// Sorted repo keys for stable render order
	repoKeys := make([]string, 0, len(stats.RepoHealth))
	for repo := range stats.RepoHealth {
		repoKeys = append(repoKeys, repo)
	}
	sort.Strings(repoKeys)

	if len(repoKeys) > 0 {
		b.WriteString("\n")
		b.WriteString(statsHeader.Render("Repo Health") + "\n")
		for _, repo := range repoKeys {
			h := stats.RepoHealth[repo]
			var badge string
			if h.LastSuccess {
				badge = blit.Badge("OK", th.Positive, true)
			} else {
				badge = blit.Badge("FAIL", th.Negative, true)
			}
			b.WriteString(fmt.Sprintf("  %s %s", badge, dim.Render(repo)) + "\n")
		}
	}

	// Cap chart width: at most 60 cols or half the text area, whichever is larger
	chartW := textW / 2
	if chartW < 30 {
		chartW = 30
	}
	if chartW > 60 {
		chartW = 60
	}
	if chartW > textW-2 {
		chartW = textW - 2
	}

	// Rate limit gauge
	if stats.RateLimit > 0 {
		b.WriteString("\n")
		b.WriteString(statsHeader.Render("Rate Limit") + "\n")
		gauge := charts.NewGauge(
			float64(stats.RateRemain),
			float64(stats.RateLimit),
			[]float64{float64(stats.RateLimit) * 0.2, float64(stats.RateLimit) * 0.5},
			fmt.Sprintf("%d/%d", stats.RateRemain, stats.RateLimit),
		)
		gauge.SetTheme(th)
		gauge.SetSize(chartW, 1)
		b.WriteString("  " + gauge.View() + "\n")
	}

	// Per-repo bar chart
	if len(repoKeys) > 0 {
		b.WriteString("\n")
		b.WriteString(statsHeader.Render("Events by Repo") + "\n")
		var data []float64
		var labels []string
		for _, repo := range repoKeys {
			h := stats.RepoHealth[repo]
			short := repo
			if i := strings.LastIndex(repo, "/"); i >= 0 {
				short = repo[i+1:]
			}
			streak := float64(h.FailStreak)
			if h.LastSuccess {
				streak = 1
			}
			data = append(data, streak)
			labels = append(labels, short)
		}
		bar := charts.NewBar(data, labels, true)
		bar.SetTheme(th)
		bar.SetSize(chartW, len(labels)+1)
		b.WriteString("  " + bar.View())
	}

	return b.String()
}

func (d *DebugOverlay) KeyBindings() []blit.KeyBind {
	return d.logViewer.KeyBindings()
}

func (d *DebugOverlay) SetSize(w, h int) {
	d.width = w
	d.height = h
	// LogViewer size is computed dynamically in View() based on stats height,
	// but set a reasonable default here for initial sizing.
	contentW := w - 4
	if contentW < 20 {
		contentW = 20
	}
	lvHeight := h - 22
	if lvHeight < 4 {
		lvHeight = 4
	}
	d.logViewer.SetSize(contentW, lvHeight)
}

func (d *DebugOverlay) Focused() bool        { return d.focused }
func (d *DebugOverlay) SetFocused(f bool)     { d.focused = f; d.logViewer.SetFocused(f) }
func (d *DebugOverlay) SetTheme(t blit.Theme) { d.theme = t; d.logViewer.SetTheme(t) }
func (d *DebugOverlay) IsActive() bool        { return d.active }
func (d *DebugOverlay) SetActive(v bool)      { d.active = v }
func (d *DebugOverlay) Close()                { d.active = false }
