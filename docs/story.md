# Claude Session Monitor — 完整規格文件

> 用 Go 從零建置的本地工具，監控所有 Claude Code session 狀態，
> 提供 Web 總覽 + Windows 桌面通知。單一 binary，零外部依賴。

---

## 1. 目標與範圍

### 核心需求

1. **偵測每個 session 的狀態**：Working / Needs Input / Idle
2. **狀態變化時彈 Windows 桌面通知**（透過 `wsl-notify-send`）
3. **簡易 Web 頁面**當狀態總覽，支援 priority 編輯
4. **排序邏輯**：Needs Input 排最前 → 依 priority 排序（Web UI 可編輯）

### 非目標

- 不做 token/cost 統計（已有 ccusage）
- 不做完整對話瀏覽（已有 claude-code-log、claude-session-viewer）
- 不做 API quota 監控

---

## 2. 資料來源：JSONL Session Logs

### 2.1 檔案位置

```
~/.claude/projects/
├── <encoded-cwd>/              # 路徑編碼：/ → -（如 -home-user-acme）
│   ├── <session-uuid>.jsonl    # 每個 session 一個檔案
│   ├── sessions-index.json     # session 索引（含 summary、message count）
│   └── ...
```

- `<encoded-cwd>`：工作目錄的絕對路徑，非字母數字字元替換為 `-`
  - 例：`/home/user/acme` → `-home-user-acme`
  - Windows WSL 例：`C:\Users\user\proj` → `-Users-user-proj`（取決於 WSL 映射）
- 每次 `claude`（不帶 `--continue`）建立新 session，產生新 `.jsonl`

### 2.2 JSONL 行結構

每行是一個 JSON 物件，代表對話中的一個事件。關鍵欄位：

```jsonc
// User message
{
  "type": "user",
  "uuid": "1ac9f388-...",
  "parentUuid": null,           // or parent message uuid
  "sessionId": "13d805c4-...",
  "cwd": "/home/user/acme",
  "version": "1.0.85",          // Claude Code 版本
  "gitBranch": "main",
  "isSidechain": false,
  "userType": "external",
  "timestamp": "2025-08-20T19:42:28.353Z",
  "message": {
    "role": "user",
    "content": "Hello there"     // string 或 content array
  }
}

// Assistant message
{
  "type": "assistant",
  "uuid": "f2fb607f-...",
  "parentUuid": "1ac9f388-...",
  "sessionId": "13d805c4-...",
  "cwd": "/home/user/acme",
  "version": "1.0.85",
  "gitBranch": "main",
  "timestamp": "2025-08-20T19:42:30.851Z",
  "message": {
    "id": "msg_01KvmX...",
    "type": "message",
    "role": "assistant",
    "model": "claude-sonnet-4-20250514",
    "content": [
      {
        "type": "text",
        "text": "I'm Claude Code, ready to help..."
      }
      // 或 { "type": "tool_use", "id": "toolu_...", "name": "Read", "input": {...} }
    ],
    "stop_reason": null,           // null | "end_turn" | "tool_use" | "max_tokens"
    "stop_sequence": null,
    "usage": {
      "input_tokens": 3,
      "output_tokens": 28,
      "cache_creation_input_tokens": 3296,
      "cache_read_input_tokens": 11331
    }
  }
}

// Summary (自動產生)
{
  "type": "summary",
  "sessionId": "...",
  "summary": "Session about implementing auth middleware",
  "timestamp": "..."
}
```

### 2.3 content 陣列中的型別

```jsonc
// 純文字回應
{ "type": "text", "text": "Here's my analysis..." }

// 工具呼叫（assistant 發起）
{ "type": "tool_use", "id": "toolu_xyz", "name": "Read", "input": {"file_path": "/path/to/file.py"} }

// 工具結果（user message 中包含）
{ "type": "tool_result", "tool_use_id": "toolu_xyz", "content": "file contents..." }

// 思考區塊
{ "type": "thinking", "thinking": "Let me analyze this..." }
```

---

## 3. 狀態偵測邏輯

### 3.1 狀態定義

| 狀態 | 符號 | 意義 | 通知 |
|------|------|------|------|
| **Working** | ● | Session 正在處理中（Claude 在思考/執行工具） | 不通知 |
| **Needs Input** | ▲ | 等待使用者輸入或批准工具 | **通知** |
| **Idle** | ◉ | Turn 結束，等待下一個 prompt | 可選通知 |
| **Inactive** | ◌ | 沒有對應的 Claude process 在跑 | 不通知 |

### 3.2 從 JSONL 最後幾行判斷狀態

核心邏輯：**讀取 JSONL 的最後一行**（最新事件），根據 `type`、`stop_reason`、`content[0].type` 三個欄位判斷。

```
讀取最後一行 →

if type == "user":
    if content[0].type == "tool_result":
        → Working（Claude 收到工具結果，正在處理）
    else:
        → Working（使用者剛送出 prompt，Claude 正在回應）

if type == "assistant":
    message = line.message
    
    if message.stop_reason == null:
        // 串流中或未完成
        if content 中有 type=="tool_use":
            → Working（正在呼叫工具，串流中）
        if content 中有 type=="text":
            → Working（正在思考/生成回應）
    
    if message.stop_reason == "tool_use":
        → Needs Input（等待工具批准或結果回傳）
        // 搭配 timeout：如果超過 tool-specific timeout 仍無新行
        // → 可能在等使用者按 approve
    
    if message.stop_reason == "end_turn":
        → Idle（回合結束，等待使用者下一個 prompt）
    
    if message.stop_reason == "max_tokens":
        → Working（可能會自動繼續）

if type == "summary":
    → 忽略，不影響狀態判斷

if type == "result":
    → Idle（session 完成）
```

### 3.3 重要注意事項

1. **`stop_reason: "end_turn"` 不一定會被寫入**
   - Claude Code 在串流完成後，JSONL 不一定可靠地記錄 `end_turn`
   - 需要搭配 **timeout** 機制：如果最後一行是 `assistant` + `stop_reason: null` + content 只有 text，且超過 N 秒沒有新行 → 推斷為 Idle（標記為 estimated）

2. **`stop_reason: "tool_use"` 的兩種情境**
   - Claude 呼叫了工具 → Claude Code 自動執行 → 下一行會是 `user` + `tool_result` → 狀態會快速轉為 Working
   - Claude 呼叫了需要批准的工具 → 停在 permission dialog → **Needs Input**
   - 區分方式：如果 `stop_reason: "tool_use"` 後超過 timeout 沒有 `tool_result` 行，判定為 Needs Input

3. **Tool-specific timeout**（參考 claude-watch-status）

   | 工具類別 | Timeout | 工具範例 |
   |---------|---------|---------|
   | Quick | 5 秒 | TodoWrite, ExitPlanMode |
   | File I/O | 10 秒 | Read, Write, Edit, Glob, Grep |
   | System | 10 秒 | Bash, BashOutput |
   | Network/MCP | 30 秒 | mcp_* 工具 |
   | 預設 | 15 秒 | 其他未分類 |

4. **Process 偵測**（輔助判斷）
   - 用 `ps aux | grep claude` 或 `/proc` 掃描偵測是否有 Claude Code process 在跑
   - 如果 JSONL 判斷為 Working 但沒有對應 process → Inactive
   - WSL 環境下 process 偵測方式：`pgrep -f "claude"` 或掃描 `/proc/*/cmdline`

### 3.4 完整狀態機

```
                    ┌─────────────┐
       新 user msg  │             │  stop_reason: end_turn
    ┌──────────────▶│   Working   │──────────────────────┐
    │               │             │                      ▼
    │               └──────┬──────┘               ┌───────────┐
    │                      │                      │   Idle    │
    │    stop_reason:      │                      │  (等待    │
    │    tool_use          │                      │  下一個   │
    │    + timeout         │                      │  prompt)  │
    │                      ▼                      └───────────┘
    │           ┌──────────────────┐                    │
    │           │  Needs Input     │                    │
    │           │  (等待批准/輸入)  │◀───────────────────┘
    │           └──────────────────┘   使用者不回應
    │                      │
    └──────────────────────┘
          使用者送出輸入
    
    任何狀態 + 無 process → Inactive
```

---

## 4. 架構設計

### 4.1 專案結構

```
claude-session-monitor/
├── main.go                     # 入口
├── go.mod
├── go.sum
├── internal/
│   ├── config/
│   │   └── config.go           # 設定管理（priority、alias、port）
│   ├── parser/
│   │   └── parser.go           # JSONL 解析 + 狀態判定
│   ├── watcher/
│   │   └── watcher.go          # fsnotify 監聽檔案變化
│   ├── notifier/
│   │   └── notifier.go         # wsl-notify-send 通知
│   ├── process/
│   │   └── detector.go         # process 偵測（是否有 claude 在跑）
│   └── server/
│       └── server.go           # HTTP API + 內嵌 Web UI
└── web/
    └── index.html              # 單檔 HTML（go:embed 嵌入）
```

### 4.2 模組職責

#### `config` — 設定管理

```go
// ~/.config/claude-session-monitor/config.json
type Config struct {
    Port     int                        `json:"port"`      // Web server port，預設 9111
    Projects map[string]*ProjectConfig  `json:"projects"`  // key = encoded project path
    Notify   NotifyConfig               `json:"notify"`
}

type ProjectConfig struct {
    Priority int    `json:"priority"`  // 0=預設, 數字越小越優先
    Alias    string `json:"alias"`     // 顯示名稱覆蓋
}

type NotifyConfig struct {
    Enabled       bool   `json:"enabled"`        // 是否啟用通知
    Command       string `json:"command"`         // 通知指令，預設偵測環境
    OnNeedsInput  bool   `json:"on_needs_input"`  // Needs Input 時通知
    OnIdle        bool   `json:"on_idle"`          // Idle 時通知
    CooldownSecs  int    `json:"cooldown_secs"`    // 同一 session 通知冷卻秒數
}
```

#### `parser` — JSONL 解析

```go
type SessionState string

const (
    StateWorking    SessionState = "working"
    StateNeedsInput SessionState = "needs_input"
    StateIdle       SessionState = "idle"
    StateInactive   SessionState = "inactive"
)

type SessionInfo struct {
    SessionID   string       `json:"session_id"`
    ProjectPath string       `json:"project_path"`   // encoded path
    ProjectName string       `json:"project_name"`    // decoded display name
    Alias       string       `json:"alias"`           // from config
    State       SessionState `json:"state"`
    Estimated   bool         `json:"estimated"`       // 狀態是否基於 timeout 推斷
    GitBranch   string       `json:"git_branch"`
    LastMessage string       `json:"last_message"`    // 最後一則 assistant text 摘要
    LastTool    string       `json:"last_tool"`       // 最後呼叫的工具名
    Model         string       `json:"model"`
    ContextTokens int          `json:"context_tokens"`  // 最後一次 input_tokens（當前 context 大小）
    ContextLimit  int          `json:"context_limit"`   // 模型 context 上限（如 200000）
    UpdatedAt     time.Time    `json:"updated_at"`      // JSONL 最後修改時間
    Priority    int          `json:"priority"`
    FilePath    string       `json:"-"`               // JSONL 檔案路徑
}
```

**解析策略（效能導向）：**
- 不完整讀取 JSONL（可能幾十 MB）
- **反向讀取最後 N 行**（從檔案尾端往回讀 4KB buffer，取最後 3-5 行）
- 只解析需要的欄位（`type`、`message.stop_reason`、`message.content[0].type`）
- 首次掃描時讀取第一行取得 `sessionId`、`cwd`、`gitBranch`

#### `watcher` — 檔案監聽

```go
// 使用 fsnotify 監聽 ~/.claude/projects/ 下所有 .jsonl 檔案
// 
// 流程：
// 1. 啟動時掃描所有現有 .jsonl → 建立 SessionInfo map
// 2. 監聽 Write/Create 事件 → 重新解析對應檔案最後幾行
// 3. 狀態變化時 → 通知 notifier + 廣播給 Web UI (SSE)
//
// 注意：
// - 需要 recursive watch（fsnotify 不支援，需手動 walk + 加目錄）
// - 新 project 目錄出現時要動態加入 watch
// - debounce：同一檔案短時間內多次 Write → 合併處理（100ms）
```

#### `notifier` — 桌面通知

```go
// 平台偵測邏輯：
// 1. 檢查是否在 WSL：/proc/version 含 "microsoft" 或 "WSL"
// 2. WSL → 用 wsl-notify-send.exe（需使用者自行安裝到 PATH）
// 3. macOS → osascript -e 'display notification ...'
// 4. Linux → notify-send
//
// 通知時機：
// - 狀態從 Working → Needs Input（最重要）
// - 狀態從 Working → Idle（可選）
// - 冷卻機制：同一 session N 秒內不重複通知
//
// 通知格式：
//   Title: "Claude Code — <project_name>"
//   Body: "⏳ 等待輸入: <last_tool or last_message 摘要>"
```

WSL 環境下 `wsl-notify-send` 使用方式：
```bash
# 安裝：下載 wsl-notify-send.exe 放到 Windows PATH 中
# 從 WSL 呼叫：
wsl-notify-send.exe --category "Claude Monitor" "需要你的輸入" 

# 或用 Go exec:
exec.Command("wsl-notify-send.exe", "--category", "Claude Monitor", message).Run()
```

#### `process/detector` — Process 偵測

```go
// 掃描 /proc/*/cmdline 找 claude 相關 process
// 回傳 map[sessionID]bool 或 map[cwd]bool
// 用途：輔助判斷 Inactive 狀態
//
// 偵測目標字串：
// - "claude" in cmdline
// - "@anthropic-ai/claude-code" in cmdline  
// - node process 含 claude 相關路徑
//
// 配對方式：
// - 讀 process 的 cwd（/proc/<pid>/cwd symlink）
// - 與 JSONL 中的 cwd 比對
```

#### `server` — HTTP Server + Web UI

```go
// 路由：
// GET  /                → 內嵌 HTML 頁面（go:embed web/index.html）
// GET  /api/sessions    → JSON: 所有 session 狀態（已排序）
// GET  /api/events      → SSE: 狀態變化即時推送
// GET  /api/config      → JSON: 目前設定
// PUT  /api/config/project/:projectPath → 更新單一 project 設定（priority, alias）
// PUT  /api/config/notify              → 更新通知設定
```

### 4.3 排序邏輯

```go
// 排序優先順序：
// 1. 狀態權重：Needs Input (0) > Working (1) > Idle (2) > Inactive (3)
// 2. 同狀態內按 priority 排序（數字小 = 高優先）
// 3. 同 priority 按 updatedAt 降序（最近活動的在前）

func sortSessions(sessions []SessionInfo) {
    sort.Slice(sessions, func(i, j int) bool {
        wi, wj := stateWeight(sessions[i].State), stateWeight(sessions[j].State)
        if wi != wj {
            return wi < wj
        }
        if sessions[i].Priority != sessions[j].Priority {
            return sessions[i].Priority < sessions[j].Priority
        }
        return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
    })
}
```

---

## 5. Web UI 設計

### 5.1 概述

- 單一 `index.html`，用 `go:embed` 嵌入 binary
- 純 HTML + CSS + vanilla JS，不用框架
- 透過 SSE (`/api/events`) 即時更新
- 每個 session 卡片可展開編輯 priority 和 alias

### 5.2 頁面佈局

```
┌─────────────────────────────────────────────────────┐
│  Claude Session Monitor              🔔 通知: ON    │
├─────────────────────────────────────────────────────┤
│                                                     │
│  ▲ Needs Input (1)  ● Working (2)  ◉ Idle (0)      │  ← 狀態摘要
│                                                     │
│  ┌─────────────────────────────────────────────┐    │
│  │ ▲  acme @feat/auth       P:1   12s ago  │    │  ← session 卡片
│  │    sonnet-4 · 128k/200k (64%)               │    │  ← 模型 + context
│  │    Let me check the git status...           │    │
│  │    [Edit Priority] [Edit Alias]             │    │  ← 展開後可編輯
│  └─────────────────────────────────────────────┘    │
│  ┌─────────────────────────────────────────────┐    │
│  │ ●  acme @main            P:1   3s ago   │    │
│  │    Implementing auth middleware...           │    │
│  └─────────────────────────────────────────────┘    │
│  ┌─────────────────────────────────────────────┐    │
│  │ ●  api-gateway @main           P:5   45s ago  │    │
│  │    Running tests...                         │    │
│  └─────────────────────────────────────────────┘    │
│                                                     │
│  ── 通知設定 ──────────────────────────────          │
│  [x] Needs Input 通知   [ ] Idle 通知               │
│  冷卻: [30] 秒                                      │
└─────────────────────────────────────────────────────┘
```

### 5.3 Session 卡片配色

| 狀態 | 左邊框顏色 | 背景 |
|------|-----------|------|
| Needs Input | 橘色 `#f59e0b` | 淡橘 `#fffbeb` |
| Working | 綠色 `#10b981` | 淡綠 `#ecfdf5` |
| Idle | 灰色 `#6b7280` | 淡灰 `#f9fafb` |
| Inactive | 淡灰 `#d1d5db` | 白色 |

### 5.4 SSE 事件格式

```jsonc
// /api/events
event: session_update
data: {
  "sessions": [ /* 完整的排序後 session 列表 */ ],
  "updated_session_id": "13d805c4-..."  // 觸發更新的 session
}

event: config_update
data: { /* 完整 config */ }
```

### 5.5 API 互動範例

```javascript
// 更新 project priority
fetch('/api/config/project/-home-user-acme', {
  method: 'PUT',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ priority: 1, alias: "Acme 主專案" })
})

// 更新通知設定
fetch('/api/config/notify', {
  method: 'PUT',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    enabled: true,
    on_needs_input: true,
    on_idle: false,
    cooldown_secs: 30
  })
})
```

---

## 6. 實作重點

### 6.1 效能：反向讀取 JSONL 最後幾行

```go
// 不用讀整個檔案，從尾端讀取
func readLastLines(path string, n int) ([][]byte, error) {
    f, _ := os.Open(path)
    defer f.Close()
    
    stat, _ := f.Stat()
    bufSize := int64(8192) // 8KB 通常足夠取最後幾行
    if bufSize > stat.Size() {
        bufSize = stat.Size()
    }
    
    buf := make([]byte, bufSize)
    f.ReadAt(buf, stat.Size()-bufSize)
    
    // 從 buf 尾端往前找 n 個 '\n'
    // ...回傳最後 n 行
}
```

### 6.2 Debounce 機制

```go
// Claude Code 串流寫入時，一個回應可能觸發數十次 Write 事件
// 用 debounce 避免過度解析

type Debouncer struct {
    timers map[string]*time.Timer
    mu     sync.Mutex
    delay  time.Duration  // 建議 200ms
}

func (d *Debouncer) Trigger(key string, fn func()) {
    d.mu.Lock()
    defer d.mu.Unlock()
    if t, ok := d.timers[key]; ok {
        t.Stop()
    }
    d.timers[key] = time.AfterFunc(d.delay, fn)
}
```

### 6.3 目錄掃描與動態 Watch

```go
// fsnotify 不支援 recursive watch
// 需要：
// 1. Walk ~/.claude/projects/ 加入所有子目錄
// 2. 監聽 Create 事件 → 如果是新目錄，動態加入 watch
// 3. 定期 re-scan（每 30 秒）確保沒遺漏

claudeDir := filepath.Join(homeDir, ".claude", "projects")

// 初始掃描
filepath.WalkDir(claudeDir, func(path string, d fs.DirEntry, err error) error {
    if d.IsDir() {
        watcher.Add(path)
    }
    return nil
})
```

### 6.4 Session 過濾

```go
// 只監控「近期活躍」的 session
// - JSONL 最後修改時間在 24 小時內
// - 或有對應的 claude process 在跑
// 避免載入歷史 session 造成雜訊

func isRecentSession(path string) bool {
    stat, _ := os.Stat(path)
    return time.Since(stat.ModTime()) < 24*time.Hour
}
```

### 6.5 WSL 環境偵測

```go
func isWSL() bool {
    data, err := os.ReadFile("/proc/version")
    if err != nil {
        return false
    }
    lower := strings.ToLower(string(data))
    return strings.Contains(lower, "microsoft") || strings.Contains(lower, "wsl")
}

func detectNotifyCommand() string {
    if isWSL() {
        // 檢查 wsl-notify-send.exe 是否在 PATH
        if _, err := exec.LookPath("wsl-notify-send.exe"); err == nil {
            return "wsl-notify-send.exe"
        }
        // fallback: 用 powershell.exe 彈 toast
        return "powershell.exe"
    }
    if runtime.GOOS == "darwin" {
        return "osascript"
    }
    return "notify-send"  // Linux
}
```

---

## 7. 依賴

### Go Dependencies

| Package | 用途 | 必要性 |
|---------|------|--------|
| `github.com/fsnotify/fsnotify` | 檔案系統監聽 | 必要 |
| 標準庫 `encoding/json` | JSONL 解析 | 必要（內建） |
| 標準庫 `net/http` | Web server + SSE | 必要（內建） |
| 標準庫 `embed` | 嵌入 HTML | 必要（內建） |
| 標準庫 `os/exec` | 呼叫通知指令 | 必要（內建） |

### 外部依賴

| 工具 | 用途 | 安裝方式 |
|------|------|---------|
| `wsl-notify-send.exe` | WSL → Windows 通知 | [GitHub releases](https://github.com/stuartleeks/wsl-notify-send/releases) 下載放 PATH |

---

## 8. CLI 介面

```bash
# 預設啟動（Web server + 監控）
claude-session-monitor

# 指定 port
claude-session-monitor --port 9111

# 指定 Claude 資料目錄（非預設位置）
claude-session-monitor --claude-dir /custom/path/.claude

# 單次列出所有 session 狀態
claude-session-monitor --list

# JSON 輸出（方便腳本整合）
claude-session-monitor --list --json

# 關閉通知
claude-session-monitor --no-notify

# 顯示版本
claude-session-monitor --version
```

---

## 9. 設定檔

位置：`~/.config/claude-session-monitor/config.json`

首次執行自動建立預設設定。Web UI 的編輯操作會直接寫回此檔。

```json
{
  "port": 9111,
  "projects": {
    "-home-user-acme": {
      "priority": 1,
      "alias": "Acme 遊戲後端"
    },
    "-home-user-api-gateway": {
      "priority": 5,
      "alias": "API Gateway"
    }
  },
  "notify": {
    "enabled": true,
    "command": "",
    "on_needs_input": true,
    "on_idle": false,
    "cooldown_secs": 30
  }
}
```

`command` 為空字串時自動偵測平台。

---

## 10. 現有工具參考

建議在實作前快速瀏覽這些 repo 的原始碼作為參考：

| 工具 | 語言 | 特色 | GitHub |
|------|------|------|--------|
| **claude-sessions-monitor (csm)** | Go | TUI + Web dashboard，process 偵測 | [yepzdk/claude-sessions-monitor](https://github.com/yepzdk/claude-sessions-monitor) |
| **claude-watch-status** | Go | 最完整的狀態偵測邏輯（含 tool timeout），fsnotify + hooks | [sho7650/claude-watch-status](https://github.com/sho7650/claude-watch-status) |
| **claude-code-dashboard** | Node.js | 極簡 Express + chokidar，2 秒 polling | [Stargx/claude-code-dashboard](https://github.com/Stargx/claude-code-dashboard) |

重點參考 `claude-watch-status` 的 `internal/parser/` 目錄 — 它的狀態偵測決策樹和 tool-specific timeout 是目前最完善的實作。

---

## 11. 開發步驟建議

### Phase 1：核心功能（先跑起來）
1. `parser` — 反向讀取 JSONL + 狀態判定
2. `watcher` — fsnotify 監聽 + debounce
3. `server` — 基礎 API (`/api/sessions`)
4. `web/index.html` — 靜態頁面 polling `/api/sessions`（先不用 SSE）
5. 手動測試：開一個 Claude Code session，確認狀態正確

### Phase 2：通知 + 即時更新
6. `notifier` — wsl-notify-send 整合
7. `process/detector` — process 偵測
8. SSE 替換 polling

### Phase 3：設定 + 打磨
9. `config` — 設定檔讀寫
10. Web UI 編輯 priority/alias
11. CLI flags
12. `go build` → 單一 binary 測試

---

## 12. 測試策略

```bash
# 產生假的 JSONL 來測試
mkdir -p ~/.claude/projects/-test-project
cat > ~/.claude/projects/-test-project/test-session.jsonl << 'EOF'
{"type":"user","sessionId":"test-001","cwd":"/test/project","gitBranch":"main","timestamp":"2025-01-01T00:00:00Z","message":{"role":"user","content":"hello"}}
{"type":"assistant","sessionId":"test-001","timestamp":"2025-01-01T00:00:01Z","message":{"role":"assistant","model":"claude-sonnet-4-20250514","content":[{"type":"text","text":"Working on it..."}],"stop_reason":null}}
EOF

# 模擬狀態變化：追加一行
echo '{"type":"assistant","sessionId":"test-001","timestamp":"2025-01-01T00:00:05Z","message":{"role":"assistant","model":"claude-sonnet-4-20250514","content":[{"type":"tool_use","id":"toolu_test","name":"Bash","input":{"command":"ls"}}],"stop_reason":"tool_use"}}' >> ~/.claude/projects/-test-project/test-session.jsonl
```