package ui

import (
	"testing"

	blit "github.com/blitui/blit"
	"github.com/blitui/blit/btest"
)

// TestHarness_EventStreamRender uses the fluent Harness API to verify
// initial rendering of the event stream.
func TestHarness_EventStreamRender(t *testing.T) {
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
			func() string { return " test" },
			func() string { return "test " },
		),
	)

	h := btest.NewHarness(t, app.Model(), 120, 40)
	defer h.Done()

	// Inject events
	h.Send(eventsMsg{events: testEvents()})

	h.Expect("gitstream").
		Expect("alice").
		Expect("bob").
		Expect("charlie").
		Expect("repo-a").
		Expect("repo-b")
}

// TestHarness_CursorNavigation uses chained Keys + Expect calls.
func TestHarness_CursorNavigation(t *testing.T) {
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
			func() string { return " test" },
			func() string { return "test " },
		),
	)

	h := btest.NewHarness(t, app.Model(), 120, 40)
	defer h.Done()

	h.Send(eventsMsg{events: testEvents()})

	h.Keys("down", "down", "up").
		Expect("alice").
		Expect("charlie")
}

// TestHarness_EventTypes verifies that event type badges render correctly
// for each injected event type.
func TestHarness_EventTypes(t *testing.T) {
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
			func() string { return " test" },
			func() string { return "test " },
		),
	)

	h := btest.NewHarness(t, app.Model(), 120, 40)
	defer h.Done()

	h.Send(eventsMsg{events: testEvents()})

	h.Expect("PUSH").
		Expect("PR").
		Expect("ISSUE")
}

// TestHarness_SnapshotEmptyStream captures a golden snapshot of the empty state
// (no events injected), which has no dynamic timestamps.
func TestHarness_SnapshotEmptyStream(t *testing.T) {
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
			func() string { return " test" },
			func() string { return "test " },
		),
	)

	h := btest.NewHarness(t, app.Model(), 80, 20)
	defer h.Done()

	h.Snapshot("empty_stream")
}

// TestHarness_SnapshotAfterResize captures a snapshot at a smaller viewport.
func TestHarness_SnapshotAfterResize(t *testing.T) {
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
			func() string { return " test" },
			func() string { return "test " },
		),
	)

	h := btest.NewHarness(t, app.Model(), 80, 20)
	defer h.Done()

	h.Resize(60, 15)
	h.Snapshot("resized_stream")
}

// testFullApp builds a full app mirroring the real gitstream layout (Tabs + DualPane
// + StatusBar). Reusable across tests that need the production component tree.
func testFullApp() *blit.App {
	cfg := testConfig()
	debugLog := NewDebugLog()
	stream := NewEventStream(cfg, debugLog)
	panel := NewStatusPanel()

	tabs := blit.NewTabs([]blit.TabItem{
		{Title: "All", Glyph: "◉", Content: stream},
		{Title: "Pushes", Glyph: "↑"},
		{Title: "PRs", Glyph: "⎇"},
	}, blit.TabsOpts{})

	return blit.NewApp(
		blit.WithLayout(&blit.DualPane{
			Main:         tabs,
			Side:         panel,
			SideWidth:    32,
			MinMainWidth: 40,
			SideRight:    true,
		}),
		blit.WithStatusBar(
			func() string { return " test" },
			func() string { return "test " },
		),
	)
}

// TestHarness_SnapshotWithTabs captures a golden snapshot of the full app layout
// (Tabs + DualPane + StatusBar) using btest.SnapshotApp. This catches tab bar
// height regressions that the bare-EventStream snapshots would miss.
func TestHarness_SnapshotWithTabs(t *testing.T) {
	btest.SnapshotApp(t, testFullApp(), 80, 20, "tabs_empty")
}

// TestHarness_AppResize uses NewAppHarness for an interactive test of the full
// app layout at multiple viewport sizes.
func TestHarness_AppResize(t *testing.T) {
	h := btest.NewAppHarness(t, testFullApp(), 80, 20)
	defer h.Done()

	h.Expect("◉ All").
		Expect("↑ Pushes").
		Expect("gitstream")

	h.Resize(60, 15).
		Expect("gitstream").
		Snapshot("tabs_resized")
}

// TestHarness_Resize verifies the layout adapts to terminal size changes.
func TestHarness_Resize(t *testing.T) {
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
			func() string { return " test" },
			func() string { return "test " },
		),
	)

	h := btest.NewHarness(t, app.Model(), 120, 40)
	defer h.Done()

	h.Send(eventsMsg{events: testEvents()})
	h.Resize(60, 20).
		Expect("gitstream").
		Expect("alice")
}
