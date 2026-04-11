package ui

import (
	blit "github.com/blitui/blit"
)

// PanelStyles holds status-panel-specific styles that extend blit.Styles.
// These are derived from the theme and rebuilt on theme change.
type PanelStyles struct {
	Repo    blit.Style
	Dim     blit.Style
	Clean   blit.Style
	Dirty   blit.Style
	Warn    blit.Style
	CIFail  blit.Style
}

// NewPanelStyles constructs panel-specific styles from a blit.Theme.
func NewPanelStyles(t blit.Theme) PanelStyles {
	return PanelStyles{
		Repo:   blit.NewStyle().Bold(true).Foreground(t.Accent),
		Dim:    blit.NewStyle().Foreground(t.Muted),
		Clean:  blit.NewStyle().Foreground(t.Positive),
		Dirty:  blit.NewStyle().Foreground(t.Warn),
		Warn:   blit.NewStyle().Foreground(t.SemanticColor("issue", t.Warn)),
		CIFail: blit.NewStyle().Foreground(t.Negative),
	}
}

// EventColor returns the color for a given event type using theme semantic colors.
func EventColor(eventType string, theme blit.Theme) blit.Color {
	switch eventType {
	case "LocalPushEvent":
		return theme.SemanticColor("local", theme.Accent)
	case "PushEvent":
		return theme.SemanticColor("create", theme.Positive)
	case "PullRequestEvent":
		return theme.Accent
	case "PullRequestReviewEvent", "PullRequestReviewCommentEvent":
		return theme.SemanticColor("review", theme.Cursor)
	case "IssueCommentEvent":
		return theme.SemanticColor("comment", theme.Muted)
	case "IssuesEvent":
		return theme.SemanticColor("issue", theme.Warn)
	case "CreateEvent":
		return theme.SemanticColor("create", theme.Positive)
	case "DeleteEvent":
		return theme.SemanticColor("delete", theme.Negative)
	case "ReleaseEvent":
		return theme.SemanticColor("release", theme.Flash)
	case "MemberEvent":
		return theme.SemanticColor("comment", theme.Muted)
	default:
		return theme.Muted
	}
}

// LabelColor maps a display label back to its themed color using semantic colors.
func LabelColor(label string, theme blit.Theme) blit.Color {
	switch label {
	case "LOCAL":
		return theme.SemanticColor("local", theme.Accent)
	case "PUSH":
		return theme.SemanticColor("create", theme.Positive)
	case "PR":
		return theme.Accent
	case "REVIEW":
		return theme.SemanticColor("review", theme.Cursor)
	case "COMMENT":
		return theme.SemanticColor("comment", theme.Muted)
	case "ISSUE":
		return theme.SemanticColor("issue", theme.Warn)
	case "CREATE":
		return theme.SemanticColor("create", theme.Positive)
	case "DELETE":
		return theme.SemanticColor("delete", theme.Negative)
	case "RELEASE":
		return theme.SemanticColor("release", theme.Flash)
	case "STAR", "FORK":
		return theme.SemanticColor("comment", theme.Muted)
	default:
		return theme.Muted
	}
}
