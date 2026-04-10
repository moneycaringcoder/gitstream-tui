package ui

import (
	"fmt"
	"sync"
	"time"

	blit "github.com/blitui/blit"
)

// DebugLog is a thread-safe log that writes directly to a blit.LogViewer.
type DebugLog struct {
	mu        sync.Mutex
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
	return &DebugLog{}
}

// SetLogViewer wires a blit.LogViewer so that new log entries are appended to it.
func (d *DebugLog) SetLogViewer(lv *blit.LogViewer) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.logViewer = lv
}

func (d *DebugLog) Log(level blit.LogLevel, format string, args ...interface{}) {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := time.Now()
	msg := fmt.Sprintf(format, args...)
	if d.logViewer != nil {
		d.logViewer.Append(blit.LogLine{
			Level:     level,
			Timestamp: now,
			Message:   msg,
		})
	}
}

func (d *DebugLog) Info(format string, args ...interface{})  { d.Log(blit.LogInfo, format, args...) }
func (d *DebugLog) Warn(format string, args ...interface{})  { d.Log(blit.LogWarn, format, args...) }
func (d *DebugLog) Error(format string, args ...interface{}) { d.Log(blit.LogError, format, args...) }

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
