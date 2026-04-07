package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tuikit "github.com/moneycaringcoder/tuikit-go"
	"github.com/moneycaringcoder/gitstream-tui/internal/gitstatus"
)

// StatusPanel displays local repo git status.
// Implements tuikit.Component.
type StatusPanel struct {
	repoStatus []gitstatus.RepoStatus
	viewport   viewport.Model
	cursor     int
	lineCount  int
	focused    bool
	width      int
	ready      bool
}

func NewStatusPanel() *StatusPanel {
	return &StatusPanel{}
}

func (p *StatusPanel) Init() tea.Cmd { return nil }

func (p *StatusPanel) Update(msg tea.Msg) (tuikit.Component, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return p.handleKey(msg)
	case gitStatusMsg:
		p.repoStatus = msg.statuses
		p.rebuildContent()
		return p, nil
	}
	return p, nil
}

func (p *StatusPanel) handleKey(msg tea.KeyMsg) (tuikit.Component, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if p.cursor > 0 {
			p.cursor--
		}
		p.rebuildContent()
		p.ensureCursorVisible()
		return p, tuikit.Consumed()
	case "down", "j":
		if p.cursor < p.lineCount-1 {
			p.cursor++
		}
		p.rebuildContent()
		p.ensureCursorVisible()
		return p, tuikit.Consumed()
	case "home", "g":
		p.cursor = 0
		p.rebuildContent()
		p.ensureCursorVisible()
		return p, tuikit.Consumed()
	case "end", "G":
		p.cursor = max(0, p.lineCount-1)
		p.rebuildContent()
		p.ensureCursorVisible()
		return p, tuikit.Consumed()
	}
	return p, nil
}

func (p *StatusPanel) View() string {
	if !p.ready {
		return "PANEL NOT READY"
	}
	badge := FocusBadgeInactive.Render("Local")
	if p.focused {
		badge = FocusBadgeActive.Render("Local")
	}
	vpView := strings.TrimRight(p.viewport.View(), "\n")
	view := lipgloss.JoinVertical(lipgloss.Left, badge, vpView)
	return lipgloss.NewStyle().MaxWidth(p.width).Render(view)
}

func (p *StatusPanel) KeyBindings() []tuikit.KeyBind {
	return []tuikit.KeyBind{
		{Key: "up/k", Label: "Scroll up", Group: "NAVIGATION"},
		{Key: "down/j", Label: "Scroll down", Group: "NAVIGATION"},
	}
}

func (p *StatusPanel) SetSize(w, h int) {
	p.width = w
	vpHeight := h - 1 // badge
	if vpHeight < 3 {
		vpHeight = 3
	}
	if !p.ready {
		p.viewport = viewport.New(w, vpHeight)
		p.ready = true
	} else {
		p.viewport.Width = w
		p.viewport.Height = vpHeight
	}
	p.rebuildContent()
}

func (p *StatusPanel) Focused() bool    { return p.focused }
func (p *StatusPanel) SetFocused(f bool) { p.focused = f; p.rebuildContent() }

func (p *StatusPanel) rebuildContent() {
	var lines []string

	addLine := func(line string) {
		idx := len(lines)
		if p.focused && idx == p.cursor {
			line = CursorMarker.Render("▌") + line
		}
		lines = append(lines, line)
	}

	if len(p.repoStatus) == 0 {
		addLine("")
		addLine(PanelDimStyle.Render("  Scanning for repos..."))
		p.lineCount = len(lines)
		p.viewport.SetContent(strings.Join(lines, "\n"))
		return
	}

	sorted := sortedRepoStatus(p.repoStatus)

	for _, s := range sorted {
		short := s.Remote
		if i := strings.LastIndex(s.Remote, "/"); i >= 0 {
			short = s.Remote[i+1:]
		}

		if s.Error != nil {
			addLine(PanelRepoStyle.Render("  ◆ " + short))
			addLine(PanelDimStyle.Render("    error"))
			addLine("")
			continue
		}

		addLine(PanelRepoStyle.Render("  ◆ " + short))
		addLine(PanelDimStyle.Render(fmt.Sprintf("    ᛘ %s", s.Branch)))

		if s.Uncommitted == 0 && s.Unpushed == 0 {
			addLine(PanelCleanStyle.Render("    ✓ clean"))
		} else {
			if s.Uncommitted > 0 {
				addLine(PanelDirtyStyle.Render(
					fmt.Sprintf("    ● %d uncommitted", s.Uncommitted)))
			}
			if s.Unpushed > 0 {
				addLine(PanelWarnStyle.Render(
					fmt.Sprintf("    ↑ %d unpushed", s.Unpushed)))
				for _, c := range s.UnpushedCommits {
					msg := c.Message
					maxLen := p.width - 10
					if maxLen < 10 {
						maxLen = 10
					}
					if len(msg) > maxLen {
						msg = msg[:maxLen-1] + "…"
					}
					addLine(PanelDimStyle.Render(
						fmt.Sprintf("      %s %s", c.SHA, msg)))
				}
			}
		}
		if !s.HasUpstream {
			addLine(PanelDimStyle.Render("    ⚠ no upstream"))
		}

		if s.CI != nil {
			var ciLine string
			switch s.CI.Conclusion {
			case "success":
				ciLine = PanelCleanStyle.Render("    ✓ CI passed")
			case "failure":
				ciLine = PanelCIFailStyle.Render("    ✗ CI failed")
			case "cancelled":
				ciLine = PanelDimStyle.Render("    ○ CI cancelled")
			default:
				if s.CI.Status == "in_progress" {
					ciLine = PanelWarnStyle.Render("    ◌ CI running")
				} else {
					ciLine = PanelDimStyle.Render(fmt.Sprintf("    ○ CI %s", s.CI.Conclusion))
				}
			}
			addLine(ciLine)
		}

		addLine("")
	}

	p.lineCount = len(lines)
	if p.cursor >= p.lineCount && p.lineCount > 0 {
		p.cursor = p.lineCount - 1
	}
	p.viewport.SetContent(strings.Join(lines, "\n"))
}

func (p *StatusPanel) ensureCursorVisible() {
	vpHeight := p.viewport.Height
	yOffset := p.viewport.YOffset
	if p.cursor < yOffset {
		p.viewport.SetYOffset(p.cursor)
	} else if p.cursor >= yOffset+vpHeight {
		p.viewport.SetYOffset(p.cursor - vpHeight + 1)
	}
}

func sortedRepoStatus(statuses []gitstatus.RepoStatus) []gitstatus.RepoStatus {
	sorted := make([]gitstatus.RepoStatus, len(statuses))
	copy(sorted, statuses)
	sort.SliceStable(sorted, func(i, j int) bool {
		iWeight := sorted[i].Uncommitted + sorted[i].Unpushed
		jWeight := sorted[j].Uncommitted + sorted[j].Unpushed
		return iWeight > jWeight
	})
	return sorted
}
