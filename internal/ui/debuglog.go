package ui

import (
	"fmt"
	"sync"
	"time"

	blit "github.com/blitui/blit"
)

// DebugLog composes blit.StatsCollector for API metrics with a blit.LogViewer
// for structured log output. It replaces the former hand-rolled stats tracking.
type DebugLog struct {
	mu        sync.Mutex
	stats     *blit.StatsCollector
	logViewer *blit.LogViewer
}

func NewDebugLog() *DebugLog {
	return &DebugLog{
		stats: blit.NewStatsCollector(),
	}
}

// SetLogViewer wires a blit.LogViewer so that new log entries are appended to it.
func (d *DebugLog) SetLogViewer(lv *blit.LogViewer) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.logViewer = lv
}

// Stats returns the underlying StatsCollector for direct access.
func (d *DebugLog) Stats() *blit.StatsCollector {
	return d.stats
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

// RecordFetch records a fetch result into the StatsCollector.
func (d *DebugLog) RecordFetch(repo string, success bool, eventCount int, usingCache bool) {
	if success {
		d.stats.RecordSuccess(repo, eventCount)
	} else {
		d.stats.RecordFailure(repo, fmt.Errorf("fetch failed"))
		if usingCache {
			d.stats.RecordCached(repo, eventCount)
		}
	}
}

// SetRateLimit updates the rate limit info in the StatsCollector.
func (d *DebugLog) SetRateLimit(remaining, limit int) {
	d.stats.SetRateLimit(remaining, limit)
}

// GetStats returns a snapshot from the StatsCollector for backward-compatible
// callers that still read field-by-field.
func (d *DebugLog) GetStats() blit.StatsSnapshot {
	return d.stats.Snapshot()
}
