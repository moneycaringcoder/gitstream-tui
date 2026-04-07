package ui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
)

const maxLogEntries = 200

// LogLevel indicates severity.
type LogLevel int

const (
	LogInfo LogLevel = iota
	LogWarn
	LogError
)

// LogEntry is a single debug log entry.
type LogEntry struct {
	Time    time.Time
	Level   LogLevel
	Message string
}

// DebugLog is a thread-safe circular log buffer.
type DebugLog struct {
	mu      sync.Mutex
	entries []LogEntry
	stats   FetchStats
}

// FetchStats tracks API call statistics.
type FetchStats struct {
	TotalCalls   int
	SuccessCalls int
	FailedCalls  int
	TotalEvents  int
	LastFetchAt  time.Time
}

func NewDebugLog() *DebugLog {
	return &DebugLog{
		entries: make([]LogEntry, 0, maxLogEntries),
	}
}

func (d *DebugLog) Log(level LogLevel, format string, args ...interface{}) {
	d.mu.Lock()
	defer d.mu.Unlock()
	msg := fmt.Sprintf(format, args...)
	d.entries = append(d.entries, LogEntry{
		Time:    time.Now(),
		Level:   level,
		Message: msg,
	})
	if len(d.entries) > maxLogEntries {
		d.entries = d.entries[len(d.entries)-maxLogEntries:]
	}
}

func (d *DebugLog) Info(format string, args ...interface{})  { d.Log(LogInfo, format, args...) }
func (d *DebugLog) Warn(format string, args ...interface{})  { d.Log(LogWarn, format, args...) }
func (d *DebugLog) Error(format string, args ...interface{}) { d.Log(LogError, format, args...) }

func (d *DebugLog) RecordFetch(repo string, success bool, eventCount int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stats.TotalCalls++
	d.stats.LastFetchAt = time.Now()
	if success {
		d.stats.SuccessCalls++
		d.stats.TotalEvents += eventCount
	} else {
		d.stats.FailedCalls++
	}
}

func (d *DebugLog) GetStats() FetchStats {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.stats
}

func (d *DebugLog) GetEntries() []LogEntry {
	d.mu.Lock()
	defer d.mu.Unlock()
	cp := make([]LogEntry, len(d.entries))
	copy(cp, d.entries)
	return cp
}

// debugState tracks the debug overlay.
type debugState struct {
	active bool
}

var (
	logInfoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6b7280"))
	logWarnStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#eab308"))
	logErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ef4444"))
	logTimeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#4b5563"))
	logStatsStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#3b82f6")).
			Bold(true)
)

func (m Model) renderDebugView() string {
	var b strings.Builder

	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffffff")).
		PaddingLeft(1).Render("─── DEBUG LOG ───")
	b.WriteString(title + "\n\n")

	// Stats
	stats := m.debugLog.GetStats()
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

	entries := m.debugLog.GetEntries()
	// Show most recent first, limit to what fits
	maxShow := m.height - 14
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
	b.WriteString(HelpStyle.PaddingLeft(2).Render("D close | q quit") + "\n")

	return b.String()
}
