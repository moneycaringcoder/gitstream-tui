# gitstream-tui

A lightweight terminal dashboard for watching GitHub repository activity in real-time. Built for developers running multiple agentic coding sessions who need a single pane of glass for what's happening across their repos.

## Features

- Live multi-repo activity feed (pushes, PRs, comments, reviews, CI, branches)
- Color-coded event types for quick scanning
- Configurable polling interval
- Minimal resource usage - single binary, no runtime dependencies
- Keyboard navigation and filtering

## Install

```bash
go install github.com/moneycaringcoder/gitstream-tui/cmd/gitstream@latest
```

Or download a binary from [Releases](https://github.com/moneycaringcoder/gitstream-tui/releases).

## Usage

```bash
# First run - creates config at ~/.config/gitstream/config.yaml
gitstream

# Add repos to watch
gitstream add owner/repo

# Start the feed
gitstream
```

## Configuration

Config lives at `~/.config/gitstream/config.yaml`:

```yaml
repos:
  - owner/repo
  - org/another-repo
interval: 30  # polling interval in seconds
```

## Requirements

- GitHub CLI (`gh`) installed and authenticated

## Built With

- [tuikit-go](https://github.com/moneycaringcoder/tuikit-go) - TUI component toolkit
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) - Styling
- GitHub REST API via `gh` CLI auth

## License

MIT
