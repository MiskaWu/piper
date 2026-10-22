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
| `-host` | `127.0.0.1` | Bind addresses, comma-separated (e.g. `127.0.0.1,192.168.1.10`). A non-loopback address requires the token printed at startup |
| `-allow-origin` | | Extra trusted origins, comma-separated (e.g. `http://localhost:5173` for the Vite dev server) |
| `-port` | `8622` | HTTP server port |
| `-poll` | `2s` | Session poll interval |
| `-version` | | Print version and exit |
| `-project-root` | `.` | Project root directory |

## Security

Piper can open a shell, so every request passes through [`internal/guard`](internal/guard/guard.go):

- It binds to `127.0.0.1` by default, and `make run` keeps it there.
- `Host` and `Origin` must be an address Piper itself is bound to. Cross-origin WebSocket handshakes and API calls get a 403, so a web page cannot reach `ws://127.0.0.1:8622/ws/host-shell`.
- Binding to a non-loopback address (`make run-lan`, or `-host` with a LAN IP) generates a token at startup, printed once in the terminal. Open the printed URL and the browser keeps the token in an HttpOnly cookie; anything else gets a 401. API clients send `Authorization: Bearer <token>`, and DevContainers receive it as `PIPER_TOKEN`.

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
│  │             │  │retro     │  │ autostop      │  │
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
| **Epic Planning** | [`internal/plan/`](internal/plan/README.md) | plan, vault, dispatch, retro, epiccontainer |
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
