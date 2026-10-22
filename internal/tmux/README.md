# Session Core

Session 生命週期的核心模組群——從 tmux 操作、狀態偵測、WebSocket 推播到終端互動。

## 包含的 packages

| Package | 路徑 | 職責 |
|---------|------|------|
| **tmux** | `internal/tmux/` | tmux CLI 封裝：session CRUD、`capture-pane`、`send-keys` |
| **watcher** | `internal/watcher/` | 定期掃描 tmux → 狀態偵測 → 透過 WebSocket 廣播 |
| **ws** | `internal/ws/` | WebSocket Hub：管理連線、廣播事件給前端 |
| **pty** | `internal/pty/` | PTY relay：`tmux attach` via `creack/pty` ↔ WebSocket，供 xterm.js 互動 |
| **store** | `internal/store/` | JSON 檔案持久化：session metadata、projects、settings、ideas |
| **lockfile** | `internal/lockfile/` | `flock(2)` 程序鎖，防止多個 piper 實例同時寫入 |

## 資料流

```
tmux (capture-pane)
  │
  ▼
watcher (每 2s 掃描 → 狀態偵測: working / needs_input / idle)
  │
  ▼
ws Hub (廣播 JSON 給所有 WebSocket 連線)
  │
  ▼
前端 (React → SessionCard 狀態更新)
```

終端互動走獨立路徑：
```
前端 xterm.js ←──WebSocket──→ pty (tmux attach via PTY) ←→ tmux session
```

## 關鍵介面

### `tmux.Commander`

抽象 `exec.Command`，測試時可注入 mock：

```go
type Commander interface {
    Output(name string, args ...string) ([]byte, error)
    Run(name string, args ...string) error
}
```

### 狀態偵測

`watcher` 掃描 `capture-pane` 底部 50 行，依 pattern 判斷狀態：

| 狀態 | 觸發範例 |
|------|---------|
| **working** | `esc to interrupt`、`⏳`、thinking 動詞 |
| **needs_input** | `[y/n]`、`? ` 開頭、`>` 結尾 |
| **idle** | 無匹配（預設） |

Pattern 可透過 Settings API 自訂（子字串 + regex）。

## 相關 API

```
GET    /api/sessions                  → session 列表 + 狀態
POST   /api/sessions                  → 建立 session
DELETE /api/sessions/{name}           → Kill session
POST   /api/sessions/{name}/send-keys → 送按鍵
WS     /ws/status                     → 狀態推播
WS     /ws/terminal/{name}            → PTY relay
```
