package ui

import (
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"github.com/moneycaringcoder/gitstream-tui/internal/github"
)

const flashDuration = 3 * time.Second

// DisplayEvent holds a parsed event for display.
type DisplayEvent struct {
	Event   github.Event
	AddedAt time.Time
}

func relativeTime(t time.Time, now time.Time) string {
	d := now.Sub(t)
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// osc8 wraps text in an OSC8 hyperlink escape sequence.
func osc8(url, text string) string {
	if url == "" {
		return text
	}
	return fmt.Sprintf("\x1b]8;;%s\x1b\\%s\x1b]8;;\x1b\\", url, text)
}

func renderEventLine(ev github.Event, now time.Time) string {
	t := ev.CreatedAt.Local().Format("15:04:05")
	rel := relativeTime(ev.CreatedAt, now)
	timeStr := fmt.Sprintf("%s %s", t, rel)

	label := ev.Label()
	detail := ev.Detail()
	actor := ev.Actor.Login
	repo := ev.ShortRepo()
	url := ev.URL()

	detailRendered := DetailStyle.Render(detail)
	if url != "" {
		detailRendered = osc8(url, detailRendered)
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

// openURL opens a URL in the default browser.
func openURL(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	cmd.Start()
}
