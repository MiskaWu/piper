# Claude Session Manager — 完整規格書

## 專案定位
跑在 WSL 上的個人開發者工具，透過瀏覽器管理多個 Claude Code 的 tmux session，監看狀態、互動操作。

---

## 環境需求（WSL 本地）

| 工具 | 用途 | 安裝 | 階段 |
|------|------|------|------|
| **tmux** | 所有 session 管理的基礎 | `sudo apt install -y tmux` | Phase 1 |
| **Go 1.23+** | 編譯後端（build 完就不需要） | 已有 | Phase 1 |
| **Node.js 18+** | Build 前端（build 完就不需要） | 已有 | Phase 1 |
| **claude CLI** | tmux session 啟動的目標程式 | 已有（在 PATH） | Phase 1 |
| **devcontainer CLI** | 管理 devcontainer 生命週期 | `npm install -g @devcontainers/cli` | Phase 3 |
| **Docker** | devcontainer 底層 runtime | Docker Desktop 或 Docker Engine | Phase 3 |

不需要額外裝 xterm / terminal emulator（跑在瀏覽器）、nginx（Go 直接 serve）、或讓此工具本身跑在 Docker 裡。

---

## 技術棧

### 後端
- **Go 1.23+**，標準庫 `net/http`（Go 1.22+ routing patterns）
- `github.com/gorilla/websocket` — WebSocket
- `github.com/creack/pty` — PTY relay
- `embed` — 打包前端 build output
- 單一 binary 部署

### 前端
- **React 18 + Vite**
- `@xterm/xterm` + `@xterm/addon-fit` + `@xterm/addon-web-links`
- 純 CSS（CSS variables 做 theme），不用 Tailwind
- Build output 到 `../static/`，Go embed 打包

---

## 目錄結構

```
claude-session-manager/
├── cmd/
│   └── main.go              # 入口：flag 解析、HTTP server、路由、graceful shutdown
├── internal/
│   ├── tmux/
│   │   └── tmux.go          # tmux 操作：Check, ListSessions, CapturePaneContent,
│   │                        #   DetectStatus, GetLastSignificantLine,
│   │                        #   CreateSession, KillSession, RenameSession, SendKeys
│   ├── watcher/
│   │   └── watcher.go       # 定期 scan → capture-pane → detect status → broadcast
│   ├── ws/
│   │   └── hub.go           # WebSocket hub：Register/Unregister/Broadcast
│   ├── pty/
│   │   └── pty.go           # PTY manager：tmux attach via creack/pty，WebSocket I/O relay
│   ├── store/
│   │   └── store.go         # Session metadata 持久化（Phase 2）
│   │                        #   ~/.config/claude-session-manager/sessions.json
│   └── devcontainer/
│       └── devcontainer.go  # Devcontainer 管理（Phase 3）
├── frontend/
│   ├── index.html
│   ├── package.json
│   ├── vite.config.js
│   └── src/
│       ├── main.jsx
│       ├── App.jsx
│       ├── api.js
│       ├── hooks/
│       │   ├── useTheme.js
│       │   └── useStatusSocket.js
│       ├── components/
│       │   ├── Sidebar.jsx
│       │   ├── SessionCard.jsx
│       │   ├── StatusBadge.jsx
│       │   ├── Terminal.jsx
│       │   ├── EmptyState.jsx
│       │   └── Modal.jsx
│       └── styles/
│           └── global.css
├── static/                   # vite build output（被 Go embed）
├── Makefile
├── go.mod
└── go.sum
```

---

## Session 資料結構（完整，跨階段）

```go
type Session struct {
    // tmux 即時資料（Phase 1）
    Name     string `json:"name"`
    Windows  int    `json:"windows"`
    Created  string `json:"created"`
    Attached bool   `json:"attached"`
    Status   string `json:"status"`   // "working" | "needs_input" | "idle"
    LastLine string `json:"lastLine"`

    // Metadata — 本地 sessions.json（Phase 2）
    Title       string `json:"title"`
    Description string `json:"description"`
    Order       int    `json:"order"`

    // Devcontainer（Phase 3）
    ProjectPath string `json:"projectPath,omitempty"`
    ProjectName string `json:"projectName,omitempty"`
}
```

---

## 完整 API 一覽

### REST

| Method | Path | Body | 階段 | 說明 |
|--------|------|------|------|------|
| GET | `/api/sessions` | — | P1 | 列出所有 `claude*` session |
| POST | `/api/sessions` | `{name, title?, description?, projectPath?}` | P1+ | 建立新 session |
| DELETE | `/api/sessions/{name}` | — | P1 | Kill session（+ 清 metadata P2） |
| PUT | `/api/sessions/{name}/rename` | `{newName}` | P1 | 重命名 |
| PUT | `/api/sessions/{name}/meta` | `{title?, description?}` | P2 | 更新標題 / 描述 |
| PUT | `/api/sessions/order` | `{order: ["name1",...]}` | P2 | 更新排序 |
| POST | `/api/sessions/{name}/send-keys` | `{keys}` | P1 | 送按鍵（備用） |
| GET | `/api/projects` | — | P3 | 列出專案 + container 狀態 |
| POST | `/api/projects/{name}/rebuild` | — | P3 | 刪除舊 container 並重建 |
| GET | `/api/projects/{name}/logs` | — | P3 | Container 最近 log |

### WebSocket

| Path | 方向 | 階段 | 說明 |
|------|------|------|------|
| `/ws/status` | Server → Client | P1 | 每 2 秒推播 sessions 狀態 |
| `/ws/terminal/{name}` | 雙向 | P1 | Binary: I/O；Text: resize JSON |

---

# Phase 1：核心功能

> 目標：能跑起來、能看、能操作。

## 範圍
- Go HTTP server + embedded React frontend
- tmux session CRUD：列出、新增、刪除、重命名
- 狀態偵測（capture-pane → working / needs_input / idle）
- WebSocket 推播 session 狀態（每 2 秒）
- 互動式 terminal（xterm.js + PTY relay via WebSocket）
- Light / Dark 主題切換

## 狀態偵測邏輯

使用 `tmux capture-pane -t {name} -p -S -50` 讀畫面內容。

> **參考來源**：綜合 claude-tmux（Rust）、claude-squad（Go）、Codeman（Node）、claude-status（Swift）的偵測邏輯，整理出最完整的 pattern set。

### ANSI 清理
capture-pane 的輸出可能帶 ANSI escape codes，偵測前要先 strip：
```go
// 移除 ANSI escape sequences
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
cleaned := ansiRegex.ReplaceAllString(raw, "")
```

### 偵測演算法
從底部往上掃描非空白行，依優先順序匹配：

**1. Working（正在處理）**

Claude Code 正在思考或執行工具時，畫面底部會出現狀態行。

| Pattern | 說明 |
|---------|------|
| `esc to interrupt` 或 `ctrl+c to interrupt` | Claude 正在處理中的標準提示 |
| `⏳` | 等待 spinner |
| `Thinking` / `Moseying` / `Crystallizing` / `Cooking` | Claude Code 的隨機思考動詞 |
| prompt `❯` + 上方有 border line（`─`）+ 含 interrupt 字樣 | claude-tmux 的結構化偵測：TUI 框架特徵 |

**2. Needs Input（等待使用者輸入）**

Claude 問了問題或需要確認。

| Pattern | 說明 |
|---------|------|
| `[y/n]` / `[Y/n]` / `[yes/no]` / `(y/n)` | 確認提示 |
| `Do you want` / `Would you like` | 選擇性問句 |
| `? ` 開頭（問號 + 空格） | 互動式 prompt（如 inquirer 風格） |
| prompt `❯` + 上方有 border line（`─`）+ **無** interrupt 字樣 | claude-tmux 的結構化偵測：Claude idle 在等輸入 |
| `Select an option` / `Enter a value` / `Type a message` | 其他互動提示 |
| 以 `>` 結尾或開頭 | 通用 prompt 指示符 |

**3. Idle（閒置）**

上面都不匹配 → idle。

### 偵測優先順序

```
capture-pane content
  ↓ strip ANSI
  ↓ 從底部往上找最後一行非空白
  ↓
  ├─ 匹配 Working patterns → "working"
  ├─ 匹配 Needs Input patterns → "needs_input"
  └─ 都不匹配 → "idle"
```

### 結構化偵測（進階，可選）

claude-tmux 的做法更精確：不只看文字，而是看 Claude Code TUI 的畫面結構。Claude Code 的 prompt 區域有固定的視覺結構：

```
───────────────────────────────  ← border line（含連續 ─ 字元）
❯                               ← input prompt
```

如果 border line 上方有 `ctrl+c to interrupt` 或 `esc to interrupt` → Working。
如果有 border + prompt 但沒有 interrupt → Idle（等待輸入）。

這種結構化偵測更不容易誤判，建議 Phase 1 先用簡單的文字匹配，運作穩定後再升級到結構化偵測。

### UI 對應

| 狀態 | 顏色 | 效果 |
|------|------|------|
| **working** | 🟠 橘色 | 脈衝動畫（status dot 閃爍） |
| **needs_input** | 🔵 藍色 | 靜態 |
| **idle** | ⚫ 灰色 | 靜態 |

## 視覺風格
- Light / Dark 切換，localStorage 持久化，Dark 預設
- 字體：JetBrains Mono（terminal）+ IBM Plex Sans（UI）
- GitHub-style 色系

## 關鍵實作

### PTY Relay (`internal/pty/pty.go`)
```
HandleTerminalWS(sessionName):
  1. WebSocket upgrade
  2. exec.Command("tmux", "attach-session", "-t", sessionName)
  3. pty.Start(cmd) → ptmx
  4. goroutine: ptmx.Read → ws.WriteMessage(BinaryMessage)
  5. goroutine: ws.ReadMessage → 判斷 resize JSON 或寫入 ptmx
  6. defer: ptmx.Close, cmd.Process.Kill
```

### Terminal Component
```
Terminal.jsx:
  props: sessionName
  useEffect on sessionName change:
    1. 清掉舊 xterm + ws
    2. new Terminal({ theme, fontFamily, fontSize })
    3. new FitAddon → term.loadAddon → fitAddon.fit()
    4. WebSocket connect /ws/terminal/{sessionName}
    5. ws.onmessage → term.write(data)
    6. term.onData → ws.send(data)
    7. ResizeObserver → fitAddon.fit() → ws.send resize JSON
  cleanup: dispose term, close ws
```

### Session Filter
- `ListSessions` 只回傳 session name 帶 `claude` 前綴的
- `CreateSession` 如果 name 不帶 `claude` 前綴，自動加 `claude-`
- `tmux list-sessions` 失敗時（no server / no sessions）回傳空陣列不報錯

### Theme
- `<html data-theme="dark|light">` 控制 CSS variables
- useTheme hook：state + localStorage + toggle
- Header 放 sun/moon icon button

## Makefile

```makefile
.PHONY: dev build clean

dev-frontend:
	cd frontend && npm run dev

dev-backend:
	go run ./cmd/main.go

build: build-frontend build-backend

build-frontend:
	cd frontend && npm ci && npm run build

build-backend: build-frontend
	go build -o claude-session-manager ./cmd/main.go

clean:
	rm -rf static/* claude-session-manager
```

## 預設值
- HTTP port: `8622`
- Poll interval: `2` 秒
- Terminal: `xterm-256color`，JetBrains Mono 14px
- 初始 PTY size: 80x24（連接後被 FitAddon resize）

## 備註
- WebSocket 斷線自動重連（3 秒 delay）
- 前端開發時 Vite proxy 轉發 `/api` 和 `/ws` 到 `localhost:8622`

---

# Phase 2：Session Metadata（排序 + 標題描述）

> 目標：能辨識每個 session 在做什麼、能按優先度排列。

## 範圍
- 本地 metadata 持久化（`~/.config/claude-session-manager/sessions.json`）
- 每個 session 可設 title（標題）和 description（描述）
- 拖拉排序，代表優先度
- Inline edit title / description

## Metadata Store (`internal/store/store.go`)

**儲存位置**：`~/.config/claude-session-manager/sessions.json`

**結構**：
```json
{
  "sessions": {
    "claude-acme": {
      "title": "Acme 後端 - API 開發",
      "description": "正在做 user auth 模組",
      "order": 0,
      "projectPath": ""
    },
    "claude-frontend": {
      "title": "GM 管理後台",
      "description": "Ant Design Pro 切版中",
      "order": 1,
      "projectPath": "/home/user/projects/admin-panel"
    }
  }
}
```

**Store 介面**：
```
Load()  → 讀 JSON，不存在就初始化空的
Save()  → atomic write（先寫 .tmp 再 rename）
GetMeta(name) → SessionMeta
SetMeta(name, meta) → 更新單一 session
SetOrder(names []string) → 用陣列順序覆蓋所有 order 值
DeleteMeta(name) → 刪除 entry
MergeWithTmux(tmuxSessions) → 合併邏輯：
  - tmux 有 + store 有 → 合併
  - tmux 有 + store 沒有 → 自動建 entry，order 放最後
  - tmux 沒有 + store 有 → 保留 metadata（30 天後自動清或手動清）
```

`GET /api/sessions` 回傳時，tmux 即時資料合併 metadata，前端按 `order` 排序。

## Session Card 顯示結構

```
┌──────────────────────────────────┐
│ [drag handle ⠿]                 │
│ 📝 Acme 後端 - API 開發     │  ← title（粗體，主辨識）
│ 正在做 user auth 模組            │  ← description（灰色小字）
│ claude-acme  🟠 Working      │  ← session name + status badge
│ > Analyzing auth middleware...   │  ← lastLine（mono，最底）
└──────────────────────────────────┘
```

- title 為空 → 不顯示，session name 升格為主標題
- description 為空 → 不佔空間
- 點 title / description 文字 → inline edit（click 變 input，blur 或 Enter 送 `PUT /meta`）
- drag handle 只在 hover 時顯示

## 拖拉排序

- HTML5 Drag and Drop API，不引入額外 library
- `draggable` 設在 drag handle，不是整張 card（避免跟點擊選取衝突）
- 拖拉中 CSS `transform` 做佔位動畫
- drop 完成 →
  1. 前端 state 立刻更新
  2. `PUT /api/sessions/order` 持久化
  3. 後端寫入 `sessions.json`
- watcher 推播的 sessions 含 order，前端按 order 排序，但**拖拉進行中暫停套用**（避免跳動）

---

# Phase 3：Devcontainer 整合

> 目標：從 UI 選一個本地專案的 devcontainer，啟動它，進去開 claude session。

## 運作原理

tmux session 始終跑在 WSL host 上，session 裡的指令改成：
```bash
devcontainer exec --workspace-folder {path} -- claude
```

對 tmux 來說這跟直接跑 `claude` 沒有差別 — 它只管 PTY 裡的文字流。所有現有的 capture-pane、attach、status detection、PTY relay **全部不用改**。不需要在 container 內裝 tmux。

降級方案：如果 `devcontainer exec` 的 TTY passthrough 有問題，改用 `docker exec -it {container_id} claude`，只需多一步「從 project path 查 container ID」。

## 使用流程

1. 「New Session」modal → 選「Devcontainer」tab
2. 看到專案列表 + container 狀態：
   ```
   acme         🟢 Running        [開啟 Session]
   my-side-project  ⏹ Stopped         [啟動並開啟]
   experiment       ⚠ Error           [查看錯誤] [重建]
   new-idea         ○ No Container    [首次建立並開啟]
   ```
3. 點按鈕 → 後端處理 → session 建立完成

## 專案與 Container 狀態偵測

### 第一層：掃描專案目錄
掃 `--projects-dir`（預設 `~/projects`）底下有 `.devcontainer/` 的目錄。

### 第二層：查 container 狀態
devcontainer 啟動的 container 帶 label `devcontainer.local_folder={absolute_path}`：
```bash
docker ps -a \
  --filter "label=devcontainer.local_folder" \
  --format '{{.Label "devcontainer.local_folder"}}\t{{.State}}\t{{.Status}}\t{{.Names}}'
```

### 合併後四種狀態

| 狀態 | 條件 | UI | 操作 |
|------|------|-----|------|
| **running** | State=running | 🟢 Running | 直接開 session（秒級） |
| **stopped** | State=exited, exit code 0 | ⏹ Stopped | `devcontainer up`（數秒） |
| **error** | State=exited + exit code ≠ 0，或 State=dead | ⚠ Error + 錯誤摘要 | 查看錯誤 / 重建 |
| **none** | 有 `.devcontainer/` 但無 container | ○ No Container | 首次 build（可能較慢） |

### Error 處理
- `docker inspect {id}` → `State.ExitCode` + `State.Error`
- `docker logs --tail 20 {id}` → 最後幾行 log
- UI 顯示錯誤摘要，可展開完整 log
- 「重建」= `docker rm {id}` → `devcontainer up`
- `devcontainer up` 失敗時 stderr 完整回傳前端，不只給 generic error

## 後端 — `internal/devcontainer/devcontainer.go`

```go
type Project struct {
    Name           string `json:"name"`           // 目錄名
    Path           string `json:"path"`           // 絕對路徑
    ContainerState string `json:"containerState"` // "running" | "stopped" | "error" | "none"
    ContainerID    string `json:"containerId,omitempty"`
    ErrorMessage   string `json:"errorMessage,omitempty"`
}

func ListProjects(projectsDir string) ([]Project, error)
func EnsureRunning(projectPath string) error
func Rebuild(projectPath string) error
func GetContainerLogs(containerID string, lines int) (string, error)
```

### 建立 session 流程
```
CreateDevcontainerSession(projectPath, sessionName):
  1. project := FindProject(projectPath)
  2. if project.ContainerState == "error":
       return error("container 狀態異常，請先重建")
  3. if project.ContainerState != "running":
       err := EnsureRunning(projectPath)
       if err: return error(err)  // 含 stderr
  4. tmux new-session -d -s {sessionName} \
       "devcontainer exec --workspace-folder {projectPath} -- claude"
```

## 前端變更

**New Session Modal：**
- 上方 tab：「WSL」/「Devcontainer」
- Devcontainer tab 顯示專案列表 + 狀態 badge + 操作按鈕
- Running → 一鍵開啟
- Stopped / None → loading spinner（5s ~ 2min）
- Error → 紅色 badge + 可展開錯誤 + 重建按鈕
- 新增 `ProjectErrorDetail` component

**Session Card 擴充：**
- 小 icon 區分 WSL vs devcontainer session
- Devcontainer session 顯示專案名

## 設定
- CLI flag `--projects-dir`（預設 `~/projects`）
- 未來可擴充 config file 支援多個路徑

## 前提
- Container 內有 `claude` CLI（devcontainer features 或 Dockerfile）
- Docker 在 WSL 可用
- 專案有 `.devcontainer/devcontainer.json`

---

# 開發順序摘要

| 階段 | 內容 | 交付物 |
|------|------|--------|
| **Phase 1** | tmux CRUD + 狀態偵測 + 互動式 terminal + 主題切換 | 可用的完整工具 |
| **Phase 2** | Session title/description + 拖拉排序 + metadata 持久化 | 更好的多 session 管理體驗 |
| **Phase 3** | Devcontainer 專案列表 + 狀態偵測 + 啟動/重建 + error 顯示 | 跨專案 devcontainer 整合 |

每個 Phase 都是獨立可交付的，後一個 Phase 不會動到前一個 Phase 的核心邏輯。