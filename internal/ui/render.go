package ui

import (
	"fmt"
	"time"

	"github.com/moneycaringcoder/gitstream-tui/internal/github"
	tuikit "github.com/moneycaringcoder/tuikit-go"
)

const flashDuration = 3 * time.Second

// DisplayEvent holds a parsed event for display.
type DisplayEvent struct {
	Event   github.Event
	AddedAt time.Time
}

func renderEventLine(ev github.Event, now time.Time) string {
	t := ev.CreatedAt.Local().Format("15:04:05")
	rel := tuikit.RelativeTime(ev.CreatedAt, now)
	timeStr := fmt.Sprintf("%s %s", t, rel)

	label := ev.Label()
	detail := ev.Detail()
	actor := ev.Actor.Login
	repo := ev.ShortRepo()
	url := ev.URL()

	detailRendered := DetailStyle.Render(detail)
	if url != "" {
		detailRendered = tuikit.OSC8Link(url, detailRendered)
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
