package ui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	blit "github.com/blitui/blit"
	"github.com/moneycaringcoder/gitstream-tui/internal/gitstatus"
)

// panelLine is a single rendered line in the status panel.
type panelLine struct {
	text string
}

// StatusPanel displays local repo git status.
// Implements blit.Component.
type StatusPanel struct {
	repoStatus []gitstatus.RepoStatus
	listView   *blit.ListView[panelLine]
	focused    bool
	width      int
}

func NewStatusPanel() *StatusPanel {
	p := &StatusPanel{}
	p.listView = blit.NewListView(blit.ListViewOpts[panelLine]{
		RenderItem: func(item panelLine, idx int, isCursor bool, theme blit.Theme) string {
			return item.text
		},
	})
	return p
}

func (p *StatusPanel) Init() tea.Cmd { return nil }

func (p *StatusPanel) Update(msg tea.Msg, ctx blit.Context) (blit.Component, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		cmd := p.listView.HandleKey(msg)
		return p, cmd
	case gitStatusMsg:
		p.repoStatus = msg.statuses
		p.rebuildContent()
		return p, nil
	}
	return p, nil
}

func (p *StatusPanel) View() string {
	return p.listView.View()
}

func (p *StatusPanel) KeyBindings() []blit.KeyBind {
	return []blit.KeyBind{
		{Key: "up/k", Label: "Scroll up", Group: "NAVIGATION"},
		{Key: "down/j", Label: "Scroll down", Group: "NAVIGATION"},
	}
}

func (p *StatusPanel) SetSize(w, h int) {
	p.width = w
	p.listView.SetSize(w, h)
}

func (p *StatusPanel) Focused() bool    { return p.focused }
func (p *StatusPanel) SetFocused(f bool) {
	p.focused = f
	p.listView.SetFocused(f)
}

func (p *StatusPanel) rebuildContent() {
	var lines []panelLine

	if len(p.repoStatus) == 0 {
		lines = append(lines, panelLine{text: ""})
		lines = append(lines, panelLine{text: PanelDimStyle.Render("Scanning for repos...")})
		p.listView.SetItems(lines)
		return
	}

	sorted := sortedRepoStatus(p.repoStatus)

	for _, s := range sorted {
		short := s.Remote
		if i := strings.LastIndex(s.Remote, "/"); i >= 0 {
			short = s.Remote[i+1:]
		}

		if s.Error != nil {
			lines = append(lines, panelLine{text: PanelRepoStyle.Render("◆ " + short)})
			lines = append(lines, panelLine{text: PanelDimStyle.Render("  error")})
			lines = append(lines, panelLine{text: ""})
			continue
		}

		lines = append(lines, panelLine{text: PanelRepoStyle.Render("◆ " + short)})
		lines = append(lines, panelLine{text: PanelDimStyle.Render(fmt.Sprintf("  ᛘ %s", s.Branch))})

		if s.Uncommitted == 0 && s.Unpushed == 0 {
			lines = append(lines, panelLine{text: PanelCleanStyle.Render("  ✓ clean")})
		} else {
			if s.Uncommitted > 0 {
				lines = append(lines, panelLine{text: PanelDirtyStyle.Render(
					fmt.Sprintf("  ● %d uncommitted", s.Uncommitted))})
			}
			if s.Unpushed > 0 {
				lines = append(lines, panelLine{text: PanelWarnStyle.Render(
					fmt.Sprintf("  ↑ %d unpushed", s.Unpushed))})
				for _, c := range s.UnpushedCommits {
					msg := c.Message
					maxLen := p.width - 10
					if maxLen < 10 {
						maxLen = 10
					}
					if len(msg) > maxLen {
						msg = msg[:maxLen-1] + "…"
					}
					lines = append(lines, panelLine{text: PanelDimStyle.Render(
						fmt.Sprintf("    %s %s", c.SHA, msg))})
				}
			}
		}
		if !s.HasUpstream {
			lines = append(lines, panelLine{text: PanelDimStyle.Render("  ⚠ no upstream")})
		}

		if s.CI != nil {
			var ciLine string
			switch s.CI.Conclusion {
			case "success":
				ciLine = PanelCleanStyle.Render("  ✓ CI passed")
			case "failure":
				ciLine = PanelCIFailStyle.Render("  ✗ CI failed")
			case "cancelled":
				ciLine = PanelDimStyle.Render("  ○ CI cancelled")
			default:
				if s.CI.Status == "in_progress" {
					ciLine = PanelWarnStyle.Render("  ◌ CI running")
				} else {
					ciLine = PanelDimStyle.Render(fmt.Sprintf("  ○ CI %s", s.CI.Conclusion))
				}
			}
			lines = append(lines, panelLine{text: ciLine})
		}

		lines = append(lines, panelLine{text: ""})
	}

	p.listView.SetItems(lines)
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
