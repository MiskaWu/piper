# piper

透過 tmux 主動管理多個 [Claude Code](https://docs.anthropic.com/en/docs/claude-code) session 的本地工具。提供 Web UI 總覽、互動式終端、狀態即時偵測——單一 binary 即可執行。

## 快速開始

```bash
go install github.com/miskawu/piper@latest
piper
# 開啟瀏覽器
open http://localhost:8622
```

需要 Go 1.26+。前端已預先建置並嵌入 binary。
啟動時自動檢查所有依賴工具——必要工具缺少會阻擋啟動，可選工具缺少僅顯示警告。

### 從原始碼建置

需要 Go 1.26+ 和 Node.js 22+。

```bash
git clone https://github.com/miskawu/piper.git
cd piper
make build    # 先 build UI 再 build Go → 單一 binary
./piper
```

### CLI 參數

| Flag | 預設值 | 說明 |
|------|--------|------|
| `-host` | `127.0.0.1` | 綁定位址，逗號分隔（例：`127.0.0.1,192.168.1.10`）。非 loopback 位址要帶啟動時印出的 token |
| `-allow-origin` | | 額外信任的來源，逗號分隔（例：Vite 開發伺服器的 `http://localhost:5173`） |
| `-port` | `8622` | HTTP server port |
| `-poll` | `2s` | Session 狀態輪詢間隔 |
| `-version` | | 顯示版本並退出 |
| `-project-root` | `.` | 專案根目錄 |

## 安全

Piper 能開 shell，所以每個請求都先經過 [`internal/guard`](../internal/guard/guard.go)：

- 預設只綁 `127.0.0.1`，`make run` 也一樣。
- `Host` 與 `Origin` 必須是 Piper 自己綁定的位址。跨來源的 WebSocket 連線與 API 呼叫都回 403，所以網頁連不到 `ws://127.0.0.1:8622/ws/host-shell`。
- 綁到非 loopback 位址（`make run-lan`，或 `-host` 帶內網 IP）時，啟動會產生 token，只在終端機印一次。打開印出的網址後，瀏覽器把 token 存在 HttpOnly cookie；其他連線一律回 401。API 用戶端帶 `Authorization: Bearer <token>`，DevContainer 會收到 `PIPER_TOKEN` 環境變數。

## 系統需求

| 工具 | 級別 | 用途 |
|------|------|------|
| [tmux](https://github.com/tmux/tmux) | 必要 | 核心 session 管理 |
| [Claude Code](https://docs.anthropic.com/en/docs/claude-code) | 必要 | Claude CLI |
| git | 可選 | Epic Git 同步、Git 線圖 |
| docker | 可選 | DevContainer 功能 |
| [devcontainer CLI](https://github.com/devcontainers/cli) | 可選 | DevContainer 建立/重建 |

## 架構

單一 binary：Go 後端透過 `go:embed` 提供 React UI。

```
┌─────────────────────────────────────────────────────┐
│                    Go 後端                           │
│                                                     │
│  ┌─────────────┐  ┌──────────┐  ┌───────────────┐  │
│  │Session 核心 │  │ Epic     │  │ DevContainer  │  │
│  │tmux watcher │  │ 規劃     │  │ 管理          │  │
│  │ws pty store │  │plan vault│  │ devcontainer  │  │
│  │  lockfile   │  │dispatch  │  │ autostop      │  │
│  │             │  │retro     │  │ preflight     │  │
│  └─────────────┘  └──────────┘  └───────────────┘  │
│                                                     │
│  ┌─────────────┐  ┌──────────────────────────────┐  │
│  │ AI & 用量   │  │ UI 輔助                      │  │
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

### 模組文件

每個功能域都有獨立 README，包含資料流、關鍵介面、相關 API：

| 功能域 | README | 包含的 packages |
|--------|--------|----------------|
| **Session 核心** | [`internal/tmux/`](../internal/tmux/README.md) | tmux, watcher, ws, pty, store, lockfile |
| **Epic 規劃** | [`internal/plan/`](../internal/plan/README.md) | plan, vault, dispatch, retro |
| **DevContainer** | [`internal/devcontainer/`](../internal/devcontainer/README.md) | devcontainer, autostop, preflight |
| **AI & 用量** | [`internal/summarizer/`](../internal/summarizer/README.md) | summarizer, usage, digest, claude |
| **UI 輔助** | [`internal/browse/`](../internal/browse/README.md) | browse, clipboard, git, gitwatcher, mousedex |

### 狀態偵測

透過掃描 tmux pane 底部 50 行內容，將 session 分為三種狀態：

| 狀態 | 觸發範例 |
|------|---------|
| **working** | `esc to interrupt`、`ctrl+c to interrupt`、`⏳`、thinking 動詞 |
| **needs_input** | `[y/n]`、`Do you want`、`? ` 開頭、`>` 結尾、選單提示 |
| **idle** | 無 pattern 匹配時的預設狀態 |

偵測 pattern 可透過 Settings API 自訂（子字串與 regex）。

## 開發

```bash
make dev-ui          # Vite dev server + HMR
make dev-backend     # Go + Air hot-reload
go test ./...        # 執行所有測試
```

## 授權

[Apache License 2.0](../LICENSE)
