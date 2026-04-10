# gitstream-tui

A lightweight terminal dashboard for watching GitHub repository activity in real-time. Built for developers running multiple agentic coding sessions who need a single pane of glass for what's happening across their repos.

## Features

- Live multi-repo activity feed (pushes, PRs, comments, reviews, CI, branches)
- Color-coded event types for quick scanning
- Configurable polling interval
- Minimal resource usage - single binary, no runtime dependencies
- Keyboard navigation and filtering

## Install

### Homebrew (macOS / Linux)

```bash
brew install moneycaringcoder/tap/gitstream
```

### Scoop (Windows)

```powershell
scoop bucket add moneycaringcoder https://github.com/moneycaringcoder/scoop-bucket
scoop install gitstream
```

### Go

```bash
go install github.com/moneycaringcoder/gitstream-tui/cmd/gitstream@latest
```

Requires Go 1.24+. The binary is installed to `$GOPATH/bin` (usually `~/go/bin`).

> **Note:** `go install` builds from source and does not embed version info. For automatic update checks to work correctly, use one of the other install methods or download a pre-built binary.

### Pre-built binaries

Download the latest release for your platform from [Releases](https://github.com/moneycaringcoder/gitstream-tui/releases). Extract and place the binary somewhere on your `PATH`.

## Updating

### Homebrew

```bash
brew upgrade gitstream
```

### Scoop

```powershell
scoop update gitstream
```

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

- [blit](https://github.com/blitui/blit) - TUI component toolkit
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) - Styling
- GitHub REST API via `gh` CLI auth

## License

MIT
