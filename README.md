# piper

Web dashboard for managing multiple [Claude Code](https://docs.anthropic.com/en/docs/claude-code) sessions via tmux. Live status detection, interactive terminals, usage tracking — all in a single binary.

[繁體中文說明](docs/README.zh-TW.md)

## Quick Start

```bash
go install github.com/miskawu/piper@latest
piper
# open http://localhost:8622
```

Requires Go 1.26+. The UI is pre-built and embedded in the binary.
On startup, piper checks dependencies — missing required tools block startup; missing optional tools show warnings.

### Build from source

Requires Go 1.26+ and Node.js 22+.

```bash
git clone https://github.com/miskawu/piper.git
cd piper
make build    # build UI + Go → single binary
./piper
```

### CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-host` | `127.0.0.1` | Bind addresses, comma-separated (e.g. `127.0.0.1,192.168.1.10`) |
| `-port` | `8622` | HTTP server port |
| `-poll` | `2s` | Session poll interval |
| `-version` | | Print version and exit |
| `-project-root` | `.` | Project root directory |

## Prerequisites

| Tool | Required | Purpose |
|------|----------|---------|
| [tmux](https://github.com/tmux/tmux) | Yes | Core session management |
| [Claude Code](https://docs.anthropic.com/en/docs/claude-code) | Yes | Claude CLI |
| git | Optional | Epic Git sync, git graph |
| docker | Optional | DevContainer features, epic container sessions |
| [devcontainer CLI](https://github.com/devcontainers/cli) | Optional | DevContainer create/rebuild |

## Architecture

Single binary: Go backend serves a React UI via `go:embed`.

```
┌─────────────────────────────────────────────────────┐
│                    Go Backend                        │
│                                                     │
│  ┌─────────────┐  ┌──────────┐  ┌───────────────┐  │
│  │Session Core │  │  Epic    │  │ DevContainer  │  │
│  │tmux watcher │  │  Planning│  │ Management    │  │
│  │ws pty store │  │plan vault│  │ devcontainer  │  │
│  │  lockfile   │  │dispatch  │  │ epiccontainer │  │
│  │             │  │harvest   │  │ autostop      │  │
│  │             │  │          │  │ preflight     │  │
│  └─────────────┘  └──────────┘  └───────────────┘  │
│                                                     │
│  ┌─────────────┐  ┌──────────────────────────────┐  │
│  │ AI & Usage  │  │ UI Utilities                 │  │
│  │ summarizer  │  │ browse clipboard git         │  │
│  │ usage digest│  │ gitwatcher mousedex          │  │
│  │ claude      │  │                              │  │
│  └─────────────┘  └──────────────────────────────┘  │
└─────────────────────────────────────────────────────┘
         │
         │ go:embed
         ▼
┌─────────────────────┐
│   React UI          │
│ xterm.js React Query│
│ i18next  WebSocket  │
└─────────────────────┘
```

### Module Documentation

Each domain has its own README with data flows, key interfaces, and related APIs:

| Domain | README | Packages |
|--------|--------|----------|
| **Session Core** | [`internal/tmux/`](internal/tmux/README.md) | tmux, watcher, ws, pty, store, lockfile |
| **Epic Planning** | [`internal/plan/`](internal/plan/README.md) | plan, vault, dispatch, harvest, epiccontainer |
| **DevContainer** | [`internal/devcontainer/`](internal/devcontainer/README.md) | devcontainer, autostop, preflight |
| **AI & Usage** | [`internal/summarizer/`](internal/summarizer/README.md) | summarizer, usage, digest, claude |
| **UI Utilities** | [`internal/browse/`](internal/browse/README.md) | browse, clipboard, git, gitwatcher, mousedex |

### Status Detection

Sessions are classified into three states by scanning the bottom 50 lines of the tmux pane:

| State | Trigger examples |
|-------|-----------------|
| **working** | `esc to interrupt`, `ctrl+c to interrupt`, `⏳`, thinking verbs |
| **needs_input** | `[y/n]`, `Do you want`, `? ` prefix, `>` suffix, menu prompts |
| **idle** | Default when no pattern matches |

Detection patterns are customizable via the Settings API (substring and regex).

## Development

```bash
make dev-ui          # Vite dev server with HMR
make dev-backend     # Go with hot reload (air)
go test ./...        # Run all tests
```

## License

[Apache License 2.0](LICENSE)
