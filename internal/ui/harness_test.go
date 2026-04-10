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

// TestHarness_GoldenInitial asserts a golden snapshot of the initial screen.
func TestHarness_GoldenInitial(t *testing.T) {
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

	h := btest.NewHarness(t, app.Model(), 100, 30)
	defer h.Done()

	h.Send(eventsMsg{events: testEvents()})
	h.Snapshot("event_stream_initial")
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
