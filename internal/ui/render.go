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

