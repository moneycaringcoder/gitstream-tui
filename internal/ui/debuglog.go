package ui

import (
	"fmt"
	"sync"
	"time"

	blit "github.com/blitui/blit"
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
	mu        sync.Mutex
	entries   []LogEntry
	stats     FetchStats
	logViewer *blit.LogViewer
}

// RepoHealth tracks per-repo fetch health.
type RepoHealth struct {
	LastSuccess bool
	FailStreak  int
	UsingCache  bool // true when serving cached events due to fetch failure
}

// FetchStats tracks API call statistics.
type FetchStats struct {
	TotalCalls   int
	SuccessCalls int
	FailedCalls  int
	TotalEvents  int
	LastFetchAt  time.Time
	RepoHealth   map[string]*RepoHealth
	RateRemain   int // GitHub API rate limit remaining
	RateLimit    int // GitHub API rate limit total
}

func NewDebugLog() *DebugLog {
	return &DebugLog{
		entries: make([]LogEntry, 0, maxLogEntries),
	}
}

// SetLogViewer wires a blit.LogViewer so that new log entries are also appended to it.
func (d *DebugLog) SetLogViewer(lv *blit.LogViewer) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.logViewer = lv
}

// mapLevel converts a ui.LogLevel to a blit.LogLevel.
func mapLevel(l LogLevel) blit.LogLevel {
	switch l {
	case LogWarn:
		return blit.LogWarn
	case LogError:
		return blit.LogError
	default:
		return blit.LogInfo
	}
}

func (d *DebugLog) Log(level LogLevel, format string, args ...interface{}) {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := time.Now()
	msg := fmt.Sprintf(format, args...)
	d.entries = append(d.entries, LogEntry{
		Time:    now,
		Level:   level,
		Message: msg,
	})
	if len(d.entries) > maxLogEntries {
		d.entries = d.entries[len(d.entries)-maxLogEntries:]
	}
	if d.logViewer != nil {
		d.logViewer.Append(blit.LogLine{
			Level:     mapLevel(level),
			Timestamp: now,
			Message:   msg,
		})
	}
}

func (d *DebugLog) Info(format string, args ...interface{})  { d.Log(LogInfo, format, args...) }
func (d *DebugLog) Warn(format string, args ...interface{})  { d.Log(LogWarn, format, args...) }
func (d *DebugLog) Error(format string, args ...interface{}) { d.Log(LogError, format, args...) }

func (d *DebugLog) RecordFetch(repo string, success bool, eventCount int, usingCache bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stats.TotalCalls++
	d.stats.LastFetchAt = time.Now()
	if d.stats.RepoHealth == nil {
		d.stats.RepoHealth = make(map[string]*RepoHealth)
	}
	h, ok := d.stats.RepoHealth[repo]
	if !ok {
		h = &RepoHealth{}
		d.stats.RepoHealth[repo] = h
	}
	if success {
		d.stats.SuccessCalls++
		d.stats.TotalEvents += eventCount
		h.LastSuccess = true
		h.FailStreak = 0
		h.UsingCache = false
	} else {
		d.stats.FailedCalls++
		h.LastSuccess = false
		h.FailStreak++
		h.UsingCache = usingCache
	}
}

func (d *DebugLog) SetRateLimit(remaining, limit int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stats.RateRemain = remaining
	d.stats.RateLimit = limit
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

