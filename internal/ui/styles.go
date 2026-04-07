package ui

import "github.com/charmbracelet/lipgloss"

var (
	// Event type colors
	ColorPush    = lipgloss.Color("#22c55e") // green
	ColorPR      = lipgloss.Color("#3b82f6") // blue
	ColorReview  = lipgloss.Color("#a855f7") // purple
	ColorComment = lipgloss.Color("#06b6d4") // cyan
	ColorIssue   = lipgloss.Color("#eab308") // yellow
	ColorCreate  = lipgloss.Color("#22c55e") // green
	ColorDelete  = lipgloss.Color("#ef4444") // red
	ColorRelease = lipgloss.Color("#f97316") // orange
	ColorDim     = lipgloss.Color("#6b7280") // gray

	// Layout styles
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#ffffff")).
			PaddingLeft(1)

	SubtitleStyle = lipgloss.NewStyle().
			Foreground(ColorDim).
			PaddingLeft(1)

	TimeStyle = lipgloss.NewStyle().
			Foreground(ColorDim).
			Width(20)

	RepoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ffffff")).
			Bold(true).
			Width(18)

	ActorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#d1d5db")).
			Width(22)

	DetailStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9ca3af"))

	StatusBarStyle = lipgloss.NewStyle().
			Foreground(ColorDim).
			PaddingLeft(1)

	HelpStyle = lipgloss.NewStyle().
			Foreground(ColorDim)

	FlashStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#1e3a5f"))
)

// EventColor returns the color for a given event type.
func EventColor(eventType string) lipgloss.Color {
	switch eventType {
	case "PushEvent":
		return ColorPush
	case "PullRequestEvent":
		return ColorPR
	case "PullRequestReviewEvent", "PullRequestReviewCommentEvent":
		return ColorReview
	case "IssueCommentEvent":
		return ColorComment
	case "IssuesEvent":
		return ColorIssue
	case "CreateEvent":
		return ColorCreate
	case "DeleteEvent":
		return ColorDelete
	case "ReleaseEvent":
		return ColorRelease
	case "MemberEvent":
		return ColorComment
	default:
		return ColorDim
	}
}

// LabelStyle returns a styled label for a given event type.
func LabelStyle(eventType string) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(EventColor(eventType)).
		Width(9).
		Bold(true)
}
