package ui

import (
	"fmt"
	"time"

	blit "github.com/blitui/blit"
	"github.com/moneycaringcoder/gitstream-tui/internal/github"
)

const flashDuration = 3 * time.Second

// DisplayEvent holds a parsed event for display.
type DisplayEvent struct {
	Event   github.Event
	AddedAt time.Time
}

// eventToRow converts a DisplayEvent into a blit.Row for the table.
func eventToRow(de DisplayEvent) blit.Row {
	ev := de.Event
	t := ev.CreatedAt.Local().Format("15:04:05")
	rel := blit.RelativeTime(ev.CreatedAt, time.Now())
	return blit.Row{
		fmt.Sprintf("%s %s", t, rel),
		ev.ShortRepo(),
		ev.Label(),
		ev.Actor.Login,
		ev.Detail(),
	}
}

// renderEventLine renders a single event as a styled string (legacy).
func renderEventLine(ev github.Event, now time.Time) string {
	t := ev.CreatedAt.Local().Format("15:04:05")
	rel := blit.RelativeTime(ev.CreatedAt, now)
	timeStr := fmt.Sprintf("%s %s", t, rel)

	label := ev.Label()
	detail := ev.Detail()
	actor := ev.Actor.Login
	repo := ev.ShortRepo()
	url := ev.URL()

	detailRendered := DetailStyle.Render(detail)
	if url != "" {
		detailRendered = blit.OSC8Link(url, detailRendered)
	}

	line := fmt.Sprintf("%s  %s %s %s %s",
		TimeStyle.Render(timeStr),
		RepoStyle.Render(repo),
		LabelStyle(ev.Type).Render(label),
		ActorStyle.Render(actor),
		detailRendered,
	)

	return line
}
