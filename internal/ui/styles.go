package ui

import (
	"github.com/charmbracelet/lipgloss"
	blit "github.com/blitui/blit"
)

// Styles holds all UI styles derived from the current theme.
// Rebuild via NewStyles whenever the theme changes.
type Styles struct {
	Title    lipgloss.Style
	Subtitle lipgloss.Style
	Detail   lipgloss.Style

	PanelRepo  lipgloss.Style
	PanelDim   lipgloss.Style
	PanelClean lipgloss.Style
	PanelDirty lipgloss.Style
	PanelWarn  lipgloss.Style
	PanelCIFail lipgloss.Style

	DetailRepo  lipgloss.Style
	DetailActor lipgloss.Style
	DetailTime  lipgloss.Style
}

// NewStyles constructs a full Styles set from a blit.Theme.
func NewStyles(t blit.Theme) Styles {
	return Styles{
		Title:    lipgloss.NewStyle().Bold(true).Foreground(t.Text).PaddingLeft(1),
		Subtitle: lipgloss.NewStyle().Foreground(t.Muted).PaddingLeft(1),
		Detail:   lipgloss.NewStyle().Foreground(t.Muted),

		PanelRepo:  lipgloss.NewStyle().Bold(true).Foreground(t.Accent),
		PanelDim:   lipgloss.NewStyle().Foreground(t.Muted),
		PanelClean: lipgloss.NewStyle().Foreground(t.Positive),
		PanelDirty: lipgloss.NewStyle().Foreground(t.Warn),
		PanelWarn:  lipgloss.NewStyle().Foreground(t.Color("issue", t.Warn)),
		PanelCIFail: lipgloss.NewStyle().Foreground(t.Negative),

		DetailRepo:  lipgloss.NewStyle().Bold(true).Foreground(t.Text),
		DetailActor: lipgloss.NewStyle().Foreground(t.Text),
		DetailTime:  lipgloss.NewStyle().Foreground(t.Muted),
	}
}

// EventColor returns the color for a given event type, derived from theme tokens.
func EventColor(eventType string, theme blit.Theme) lipgloss.Color {
	switch eventType {
	case "LocalPushEvent":
		return theme.Color("local", theme.Accent)
	case "PushEvent":
		return theme.Color("create", theme.Positive)
	case "PullRequestEvent":
		return theme.Accent
	case "PullRequestReviewEvent", "PullRequestReviewCommentEvent":
		return theme.Color("review", theme.Cursor)
	case "IssueCommentEvent":
		return theme.Color("comment", theme.Muted)
	case "IssuesEvent":
		return theme.Color("issue", theme.Warn)
	case "CreateEvent":
		return theme.Color("create", theme.Positive)
	case "DeleteEvent":
		return theme.Color("delete", theme.Negative)
	case "ReleaseEvent":
		return theme.Color("release", theme.Flash)
	case "MemberEvent":
		return theme.Color("comment", theme.Muted)
	default:
		return theme.Muted
	}
}

// LabelColor maps a display label back to its themed color.
func LabelColor(label string, theme blit.Theme) lipgloss.Color {
	switch label {
	case "LOCAL":
		return theme.Color("local", theme.Accent)
	case "PUSH":
		return theme.Color("create", theme.Positive)
	case "PR":
		return theme.Accent
	case "REVIEW":
		return theme.Color("review", theme.Cursor)
	case "COMMENT":
		return theme.Color("comment", theme.Muted)
	case "ISSUE":
		return theme.Color("issue", theme.Warn)
	case "CREATE":
		return theme.Color("create", theme.Positive)
	case "DELETE":
		return theme.Color("delete", theme.Negative)
	case "RELEASE":
		return theme.Color("release", theme.Flash)
	case "STAR", "FORK":
		return theme.Color("comment", theme.Muted)
	default:
		return theme.Muted
	}
}
