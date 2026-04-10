package ui

import (
	"testing"
	"time"

	blit "github.com/blitui/blit"
	"github.com/blitui/blit/btest"
	"github.com/moneycaringcoder/gitstream-tui/internal/config"
	"github.com/moneycaringcoder/gitstream-tui/internal/github"
)

// testConfig returns a minimal config for testing.
func testConfig() *config.Config {
	return &config.Config{
		RepoEntries: []config.RepoEntry{
			{Name: "owner/repo-a"},
			{Name: "owner/repo-b"},
		},
		Interval: 30,
	}
}

// testEvents returns a set of fake events for testing.
func testEvents() []github.Event {
	now := time.Now()
	return []github.Event{
		{
			ID:        "1",
			Type:      "PushEvent",
			Actor:     github.Actor{Login: "alice"},
			Repo:      github.Repo{Name: "owner/repo-a"},
			Payload:   github.Payload{Ref: "refs/heads/main", Commits: []github.Commit{{SHA: "abc1234", Message: "feat: add feature"}}},
			CreatedAt: now.Add(-5 * time.Minute),
		},
		{
			ID:        "2",
			Type:      "PullRequestEvent",
			Actor:     github.Actor{Login: "bob"},
			Repo:      github.Repo{Name: "owner/repo-b"},
			Payload:   github.Payload{Action: "opened", PullRequest: &github.PullRequest{Number: 42, Title: "Fix the bug"}},
			CreatedAt: now.Add(-3 * time.Minute),
		},
		{
			ID:        "3",
			Type:      "IssuesEvent",
			Actor:     github.Actor{Login: "charlie"},
			Repo:      github.Repo{Name: "owner/repo-a"},
			Payload:   github.Payload{Action: "opened", Issue: &github.Issue{Number: 10, Title: "Something broken"}},
			CreatedAt: now.Add(-1 * time.Minute),
		},
	}
}

// testApp builds a blit.App wrapping an EventStream with injected events.
func testApp(t testing.TB, events []github.Event) (*btest.TestModel, *EventStream) {
	t.Helper()
	cfg := testConfig()
	debugLog := NewDebugLog()
	stream := NewEventStream(cfg, debugLog)
	panel := NewStatusPanel()

	app := blit.NewApp(
		blit.WithLayout(&blit.DualPane{
			Main:         stream,
			Side:         panel,
			SideWidth:    32,
			MinMainWidth: 40,
			SideRight:    true,
		}),
		blit.WithStatusBar(
			func() string { return " test status left" },
			func() string { return "test status right " },
		),
	)

	tm := btest.NewTestModel(t, app.Model(), 120, 40)

	// Inject events directly
	if len(events) > 0 {
		tm.SendMsg(eventsMsg{events: events})
	}

	return tm, stream
}

func TestStreamRendersEvents(t *testing.T) {
	tm, _ := testApp(t, testEvents())
	s := tm.Screen()

	btest.AssertContains(t, s, "alice")
	btest.AssertContains(t, s, "bob")
	btest.AssertContains(t, s, "charlie")
	btest.AssertContains(t, s, "repo-a")
	btest.AssertContains(t, s, "repo-b")
}

func TestStreamRendersHeader(t *testing.T) {
	tm, _ := testApp(t, testEvents())
	s := tm.Screen()

	btest.AssertContains(t, s, "gitstream")
	btest.AssertContains(t, s, "owner/repo-a")
	btest.AssertContains(t, s, "owner/repo-b")
}

func TestStreamCursorNavigation(t *testing.T) {
	tm, _ := testApp(t, testEvents())

	// Move cursor down
	tm.SendKey("down")
	tm.SendKey("down")

	s := tm.Screen()
	// Should still render all events
	btest.AssertContains(t, s, "alice")
	btest.AssertContains(t, s, "charlie")
}

func TestStreamSortToggle(t *testing.T) {
	tm, stream := testApp(t, testEvents())

	// Default is oldest first
	if stream.IsNewestFirst() {
		t.Error("should default to oldest first")
	}

	// Send sort toggle via the stream method
	stream.ToggleSort()

	s := tm.Screen()
	if !stream.IsNewestFirst() {
		t.Error("should be newest first after toggle")
	}
	// Events should still render
	btest.AssertContains(t, s, "alice")
}

func TestStreamTypeFilter(t *testing.T) {
	tm, stream := testApp(t, testEvents())

	// Cycle to PushEvent filter (second in list, after "all")
	// typeFilters: "", "LocalPushEvent", "PushEvent", ...
	stream.CycleTypeFilter(true) // -> LocalPushEvent
	stream.CycleTypeFilter(true) // -> PushEvent

	s := tm.Screen()
	btest.AssertContains(t, s, "alice") // alice has PushEvent
	// bob's PR event should be filtered out
	if s.Contains("bob") {
		t.Error("bob's PullRequestEvent should be filtered when type=PushEvent")
	}
}

func TestStreamRepoFilter(t *testing.T) {
	tm, stream := testApp(t, testEvents())

	stream.SetRepoFilter("repo-b")

	s := tm.Screen()
	btest.AssertContains(t, s, "bob") // bob is in repo-b
	// alice and charlie are in repo-a, should be hidden
	if s.Contains("alice") {
		t.Error("alice should be filtered out when repo=repo-b")
	}
}

func TestStreamClearFilters(t *testing.T) {
	_, stream := testApp(t, testEvents())

	stream.SetRepoFilter("repo-b")
	stream.CycleTypeFilter(true)
	stream.ClearFilters()

	if stream.RepoFilter() != "" {
		t.Error("repo filter should be cleared")
	}
	if stream.TypeFilter() != "" {
		t.Error("type filter should be cleared")
	}
}

func TestStreamPauseToggle(t *testing.T) {
	_, stream := testApp(t, testEvents())

	if stream.IsPaused() {
		t.Error("should not be paused initially")
	}
	stream.TogglePause()
	if !stream.IsPaused() {
		t.Error("should be paused after toggle")
	}
	stream.TogglePause()
	if stream.IsPaused() {
		t.Error("should be unpaused after second toggle")
	}
}

func TestStreamEmptyState(t *testing.T) {
	tm, _ := testApp(t, nil)
	s := tm.Screen()

	// Should still render header
	btest.AssertContains(t, s, "gitstream")
	btest.AssertNotEmpty(t, s)
}

func TestStreamResize(t *testing.T) {
	tm, _ := testApp(t, testEvents())

	// Resize to small
	tm.SendResize(60, 20)
	s := tm.Screen()
	btest.AssertNotEmpty(t, s)
	btest.AssertContains(t, s, "gitstream")

	// Resize to large
	tm.SendResize(200, 50)
	s = tm.Screen()
	btest.AssertNotEmpty(t, s)
	btest.AssertContains(t, s, "alice")
}
