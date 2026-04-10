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
	sections   map[string]*blit.CollapsibleSection
	headerMap  map[int]string // line index → repo remote
}

func NewStatusPanel() *StatusPanel {
	p := &StatusPanel{
		sections:  make(map[string]*blit.CollapsibleSection),
		headerMap: make(map[int]string),
	}
	p.listView = blit.NewListView(blit.ListViewOpts[panelLine]{
		EmptyText: "No local repos found",
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
		if msg.String() == "enter" || msg.String() == " " {
			idx := p.listView.CursorIndex()
			if remote, ok := p.headerMap[idx]; ok {
				if sec, ok := p.sections[remote]; ok {
					sec.Toggle()
					p.rebuildContent()
					return p, nil
				}
			}
		}
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
		{Key: "enter/space", Label: "Toggle section", Group: "NAVIGATION"},
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

// SetTheme implements blit.Themed so the App's theme propagates to the ListView.
func (p *StatusPanel) SetTheme(t blit.Theme) {
	p.listView.SetTheme(t)
}

func (p *StatusPanel) rebuildContent() {
	var lines []panelLine
	p.headerMap = make(map[int]string)

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

		// Get or create collapsible section for this repo.
		sec, ok := p.sections[s.Remote]
		if !ok {
			sec = blit.NewCollapsibleSection(short)
			// Default: collapse clean repos, expand dirty repos.
			sec.Collapsed = (s.Uncommitted == 0 && s.Unpushed == 0 && s.Error == nil)
			p.sections[s.Remote] = sec
		}

		// Header line with collapse indicator.
		indicator := "▼"
		if sec.Collapsed {
			indicator = "▶"
		}
		p.headerMap[len(lines)] = s.Remote
		lines = append(lines, panelLine{text: PanelRepoStyle.Render(indicator + " " + short)})

		if !sec.Collapsed {
			if s.Error != nil {
				lines = append(lines, panelLine{text: PanelDimStyle.Render("  error")})
			} else {
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
			}
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
