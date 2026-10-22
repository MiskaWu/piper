# piper — 開發指引

## 專案概述

透過 tmux 主動管理 Claude Code session 的本地工具，提供 Web UI 總覽 + 互動式終端。
單一 binary，Go 後端 + React 前端（Vite build 後 `go:embed` 嵌入）。

完整規格見 `docs/new-story.md`。

## 技術棧

- **語言**: Go (module: `piper`)
- **外部依賴**: `gorilla/websocket`（WebSocket）、`creack/pty`（PTY relay）
- **前端**: React 18 + TypeScript + Vite，build output 到 `static/`
- **終端**: xterm.js（@xterm/xterm + addon-fit + addon-web-links）

## 專案結構

```
├── main.go                      # 入口：flag、HTTP server、路由、graceful shutdown
├── routes.go                    # 所有 API 路由定義
├── internal/
│   ├── tmux/                    # tmux 操作（Commander 介面 + Client）+ lastLine 擷取
│   ├── watcher/                 # 定期 scan → capture-pane lastLine + git info → broadcast
│   ├── hookstatus/              # Claude Code hook/statusline 格式解析（legacy，保留供參考）
│   ├── ws/                      # WebSocket hub（Register/Unregister/Broadcast）
│   ├── pty/                     # PTY relay（tmux attach via creack/pty）
│   ├── store/                   # Session metadata + settings 持久化
│   ├── vault/                   # 統一 Vault git repo（epics + ideas）
│   ├── devcontainer/            # DevContainer 管理（base image + AI overlay 生成、建立、重建、日誌）
│   ├── usage/                   # 用量統計（rate limits 來自 hook + 工作時間 + completion 追蹤）
│   ├── summarizer/              # claude -p haiku 自動摘要 + 花費追蹤
│   ├── claude/                  # Claude CLI 整合（history 讀取、preflight 檢查）
│   ├── plan/                    # Epic 規劃（plan session、story-map、git sync）
│   ├── browse/                  # 目錄瀏覽 API
│   ├── clipboard/               # 剪貼簿（圖片 + 通用檔案上傳）
│   ├── mousedex/                # 老鼠圖鑑（DJB2 hash + mulberry32 PRNG）
│   ├── updater/                 # Claude Code 版本檢查與更新（host + container）
│   ├── digest/                  # 每日工作摘要（彙總 + Discord webhook + 排程）
│   ├── retro/                   # Retro 報告與 closure service（關 story + 產報告）
│   ├── autostop/                # 閒置自動停止
│   ├── preflight/               # Session 建立前置檢查
│   └── git/                     # Git 操作工具
├── ui/src/
│   ├── App.tsx                  # 主應用
│   ├── components/              # UI 元件（Sidebar, SessionCard, Terminal, UsageBar...）
│   ├── hooks/                   # React hooks
│   ├── locales/                 # i18n（en.json, zh-TW.json）
│   ├── styles/                  # CSS
│   └── types.ts                 # TypeScript 型別定義
├── static/                      # Vite build output（go:embed）
└── Makefile
```

## 開發規範

### Vault 資料操作
- Story Map 修改必須透過 `PUT /api/epics/{name}/story-map` API，禁止直接寫入檔案
- API 會執行驗證（ValidateStoryMap）、正規化（MarshalStoryMap）、WebSocket 通知

### Go 慣例
- 使用 `internal/` 隔離內部套件
- 依賴注入：`Commander` 介面抽象 exec.Command → 可 mock 測試
- 錯誤處理保留錯誤鏈：`fmt.Errorf("操作 session %s: %w", name, err)`
- 測試檔案與原始碼同目錄
- **外部指令 timeout**：網路相關的 `exec.Command`（git push/pull/ls-remote 等）必須用 `exec.CommandContext` + `context.WithTimeout`，避免 hang。參考 `internal/plan/git.go` 的 `runCtx` 方法。Timeout 建議值：驗證類 10s、推拉類 30s。

### 前端慣例
- **API timeout**：`api.ts` 的 `request()` 已內建 30s `AbortController` timeout
- **Selective save**：Settings 儲存只送出有變更的設定（dirty check），避免不相關的慢請求阻塞
- **Loading 回饋**：所有 async 按鈕在 loading 狀態時顯示 `.btn-spinner` 轉圈動畫（定義於 `styles/global.css`）

### 測試
- Go 標準 `testing` 套件
- 執行測試：`go test ./...`
- 測試命名：`TestFunctionName_情境描述`
- testdata 目錄放測試用假資料

### 建置與執行
- 建置：`make build`（先 build UI 再 build Go）
- 開發：`make dev-backend`（Go）+ `make dev-ui`（Vite）
- 預設 port: 8622

### 本地 API 存取（Sandbox 注意事項）
- Piper 運行於 `127.0.0.1:8622`，但 Linux/WSL2 的 sandbox（bubblewrap）會建立獨立 network namespace，導致 sandbox 內無法連到 host 的 localhost
- 呼叫本地 API 的 Bash 指令（如 `curl http://127.0.0.1:8622/...`）**必須使用 `dangerouslyDisableSandbox: true`**，否則會 connection refused
- 這只影響網路存取，檔案系統操作仍應在 sandbox 內執行

## 核心設計決策

### 狀態偵測
三種狀態：`working` / `needs_input` / `idle`

**主要來源（DevContainer sessions）**：Claude Code Hooks → Piper API（push 模式）
- 容器模板（v10+）內建 `piper-hook.mjs` 腳本，透過 Claude Code hooks 接收生命週期事件
- Hook 事件透過 `POST /api/internal/hook-status` 推送到 Piper（需 `PIPER_HOST` 環境變數）
- 事件映射：`UserPromptSubmit`/`PreToolUse`/`SubagentStart` → working、`Stop` → idle、`StopFailure`/`Notification[permission]` → needs_input
- 另有 `piper-statusline.mjs` 透過 `POST /api/internal/session-meta` 推送 session metadata（cost、context%、model、rateLimits）
- `PIPER_HOST` 由 Piper 從 `--host` flag 解析非 loopback 地址後注入 DevContainer（`devcontainer exec --remote-env`）
- Hook 設定透過 `postStartCommand` 合併到 `/workspace/.claude/settings.local.json`（與使用者設定累加，不覆蓋）
- 狀態與 metadata 存在 Piper 記憶體中（`hookDataStore`），不再寫入檔案

**輔助來源**：`tmux capture-pane -p -S -50` 仍用於擷取 `lastLine`（最後有意義的輸出行）

### API 路由

**Sessions**
```
GET    /api/sessions                    → session 列表 + 狀態
POST   /api/sessions                    → 建立 session
DELETE /api/sessions/{name}             → Kill session
PUT    /api/sessions/{name}/rename      → 重命名
PUT    /api/sessions/{name}/meta        → 更新 metadata（title/description/projectPath）
POST   /api/sessions/{name}/send-keys   → 送按鍵
PUT    /api/sessions/order              → 排序
```

**Projects**
```
GET    /api/projects                    → 專案列表 + 容器狀態 + autoMode + updateAvailable + hasManifest
POST   /api/projects                    → 註冊專案
DELETE /api/projects/{name}             → 移除專案
PUT    /api/projects/{name}/auto-mode   → 設定全自動模式
POST   /api/projects/{name}/rebuild     → 重建容器
POST   /api/projects/{name}/upgrade     → 升級 DevContainer 配置（保留語言層，更新基底模板）
POST   /api/projects/{name}/update-claude → 更新容器內 Claude Code 到最新版
GET    /api/projects/{name}/manifest    → 讀 .plan/manifest.yaml（raw YAML，/design 用來決定契約 transport）
GET    /api/projects/{name}/logs        → 容器日誌
PUT    /api/projects/order              → 排序
POST   /api/projects/setup              → DevContainer 設定流程（base image + 空語言層；使用者進容器後自行加裝）
GET    /api/projects/setup/{id}/status  → 設定進度
DELETE /api/projects/setup/{id}         → 取消設定
GET    /api/directories/browse          → 瀏覽目錄（僅目錄）
GET    /api/directories/files           → 瀏覽檔案與目錄（Files Tree 用）
POST   /api/files/copy                  → 複製檔案/目錄（sources[] + destination，自動解決命名衝突）
POST   /api/files/move                  → 移動檔案/目錄（同上，移動後刪除來源）
```

**Epics & Ideas（需 VaultManager）**
```
GET    /api/epics                       → Epic 列表
POST   /api/epics                       → 建立 Epic
DELETE /api/epics/{name}                → 刪除 Epic
PUT    /api/epics/{name}/rename         → 重命名
GET    /api/epics/{name}/story-map      → 取得 story map
GET    /api/epics/{name}/story-map/stats → story/task 統計（by status、release、project）
PUT    /api/epics/{name}/story-map      → 更新 story map
POST   /api/epics/{name}/plan-session       → 建立/取得規劃 session（冪等）
GET    /api/epics/{name}/plan-session       → 查詢 plan session 狀態
DELETE /api/epics/{name}/plan-session       → 結束 plan session
POST   /api/epics/{name}/dispatch-session   → 建立/取得派送 session（冪等）
GET    /api/epics/{name}/dispatch-session   → 查詢 dispatch session 狀態
DELETE /api/epics/{name}/dispatch-session   → 結束 dispatch session
POST   /api/epics/{name}/retro-session       → 建立/取得 retro session（冪等）
GET    /api/epics/{name}/retro-session       → 查詢 retro session 狀態
DELETE /api/epics/{name}/retro-session       → 結束 retro session
GET    /api/epics/{name}/problem-statement  → 讀取 problem statement
PUT    /api/epics/{name}/problem-statement  → 寫入（驗證含「不做什麼」section）
GET    /api/epics/{name}/adr                → ADR 列表
GET    /api/epics/{name}/adr/{file}         → 讀取 ADR
PUT    /api/epics/{name}/adr/{file}         → 寫入 ADR（驗證含「背景/決定/後果」）
DELETE /api/epics/{name}/adr/{file}         → 刪除 ADR
GET    /api/epics/{name}/spikes             → Spike 列表
GET    /api/epics/{name}/spikes/{file}      → 讀取 Spike
PUT    /api/epics/{name}/spikes/{file}      → 寫入 Spike
DELETE /api/epics/{name}/spikes/{file}      → 刪除 Spike
GET    /api/epics/{name}/specs/summary      → 讀取 specs summary
PUT    /api/epics/{name}/specs/summary      → 寫入 specs summary
GET    /api/epics/{name}/specs/changelog    → 讀取 specs changelog
PUT    /api/epics/{name}/specs/changelog    → 寫入 specs changelog
GET    /api/epics/{name}/specs/extracts     → 列出所有 extract
GET    /api/epics/{name}/specs/extracts/{file} → 讀取特定 extract
POST   /api/epics/{name}/specs/extract      → 提取來源檔（sourcePath → 執行 Python 腳本）
GET    /api/epics/{name}/contracts          → 契約列表
GET    /api/epics/{name}/contracts/{id}     → 讀取契約 YAML
PUT    /api/epics/{name}/contracts/{id}     → 寫入契約（驗證 id/version/type + 類型專屬欄位）
DELETE /api/epics/{name}/contracts/{id}     → 刪除契約
POST   /api/epics/{name}/contracts/{id}/generate → 依 generates[] 生成測試骨架/類型檔（dryRun/overwrite）；artifacts[].warnings 標記失準路徑提示 rescan manifest
POST   /api/epics/{name}/retro/preview     → 預覽：哪些 story 要關、哪些 release 要產報告 / skip / regenerate
POST   /api/epics/{name}/retro/execute     → 執行：關 story + 產報告（body: {epicId, forceReleases?}，冪等可重跑）
POST   /api/epics/{name}/retro/generate    → 單一 release 直接重產報告（低階 API；也寫 .json snapshot）
GET    /api/epics/{name}/retro/trends      → 跨 release 指標聚合（UI 趨勢面板）
GET    /api/epics/{name}/retro/reports     → Retro 報告列表
GET    /api/epics/{name}/retro/reports/{release} → 讀取特定報告（markdown；加 ?format=json 讀 snapshot）
GET    /api/epics/{name}/retro/actions     → 讀取改善行動（YAML；加 ?format=json 讀結構化）
PUT    /api/epics/{name}/retro/actions     → 整檔寫入改善行動（power user 用）
POST   /api/epics/{name}/retro/actions     → 新增單筆改善行動（body: {description, fromRetro?}）
PATCH  /api/epics/{name}/retro/actions/{id} → 改 action 狀態（body: {status: "todo"|"done"}）
DELETE /api/epics/{name}/retro/actions/{id} → 刪除 action
POST   /api/epics/{name}/retro/decisions   → Append 一筆推導決策到 decisions.jsonl（plan session 四類自動推導用）
GET    /api/epics/{name}/retro/decisions   → 讀取 decisions.jsonl 原文（retro 報告產生時用）
GET    /api/epics/claude-version        → Epic image 的 Claude Code 版本狀態（current/latest/updating）
POST   /api/epics/claude-update         → 背景重建 epic image 至最新 Claude Code（build-arg cache-bust）
GET    /api/ideas                       → Idea 列表
POST   /api/ideas                       → 建立 Idea
PUT    /api/ideas/{id}                  → 編輯 Idea 文字
DELETE /api/ideas/{id}                  → 刪除 Idea
POST   /api/ideas/{id}/enrich           → AI 充實 Idea 內容
```

**Dashboard & Settings**
```
GET    /api/dashboard-stats             → 月度用量統計（token + 工作時間 + 花費）
GET    /api/summary-stats               → 摘要統計
POST   /api/clipboard/image             → 上傳圖片
GET    /api/clipboard/image/{id}        → 取得圖片
POST   /api/clipboard/file              → 上傳任意檔案（拖放用）
GET    /api/clipboard/file/{id}         → 取得已上傳檔案
GET    /api/settings/models             → AI 模型設定
PUT    /api/settings/models             → 更新模型設定
GET    /api/settings/epic-git           → Epic Git 同步設定
PUT    /api/settings/epic-git           → 更新 Git 設定
POST   /api/settings/epic-git/sync      → 手動觸發同步
GET    /api/settings/epic-git/status    → 同步狀態
GET    /api/settings/firewall          → 防火牆設定（mode + 白名單域名）
PUT    /api/settings/firewall          → 更新防火牆設定
GET    /api/settings/digest            → 每日摘要設定（webhookUrl + enabled + sendHour）
PUT    /api/settings/digest            → 更新摘要設定
GET    /api/digest                     → 每日工作摘要（?date=YYYY-MM-DD，預設昨天）
POST   /api/digest/send               → 手動發送摘要到 Discord webhook
GET    /api/mousedex                    → 老鼠圖鑑完整資料
GET    /api/mousedex/stats              → 圖鑑收集摘要（collected/total）
```

**Git**
```
GET    /api/git/available               → git 是否可用
GET    /api/git/branches                → 分支列表（path）
GET    /api/git/graph                   → commit 線圖（path + limit + ref + hideRemotes）
GET    /api/git/commit-body             → 完整 commit message
POST   /api/git/init                    → 初始化 repo（可選 remote）
POST   /api/git/remote                  → 新增 remote
POST   /api/git/push                    → Push to origin（含 SSH 檢查）
GET    /api/git/tags                    → Tag 列表
POST   /api/git/tag                     → 建立 tag（name + hash）
DELETE /api/git/tag                     → 刪除本地 tag
POST   /api/git/tag/push               → 推送 tag 到 remote（含 SSH 檢查）
GET    /api/git/file-status             → 逐檔 git status（M/A/D/?/R）
GET    /api/git/submodules              → Submodule 列表 + 各自未推 commit 數
POST   /api/git/submodules/push         → 推送指定 submodule（含 SSH 檢查）
```

**Internal（DevContainer hooks → Piper）**
```
POST   /api/internal/hook-status        → 接收 hook 狀態推送（sessionName + status + hookEvent）
POST   /api/internal/session-meta       → 接收 statusline metadata 推送（cost、context%、model、rateLimits）
```

**References（plan/dispatch/retro session 按需 curl 載入的規則）**
```
GET    /api/references/story-splitting  → Story 拆分原則（INVEST / 漏洞檢查 🔴🟡🟢 / Independence）
```

**Bindings（project-local proto 契約實作綁定）**
```
GET    /api/bindings/{project}          → 列出本 project 所有 binding 的 contract id
GET    /api/bindings/{project}/{id}     → 讀單一 binding（raw YAML）
PUT    /api/bindings/{project}/{id}     → 寫入 binding（body YAML，id 需與 URL 一致）
DELETE /api/bindings/{project}/{id}     → 刪除 binding
```
> 獨立 namespace（非 `/api/projects/{name}/bindings/...`）避免與 `/api/projects/setup/...` 路由衝突。Binding 本身是 project-local 實作細節，對應 `.plan/bindings/{contract-id}.yaml`，供 proto 契約補 service.method/message 綁定；契約保持 upstream 單一 writer，binding 由下游寫。

**WebSocket**
```
WS     /ws/status                       → 狀態推播（每 2 秒）
WS     /ws/terminal/{name}              → PTY relay
```

### Session 命名與分組
- Session name 格式：`ragent-{project}-{shortID}`（e.g. `ragent-piper-a3f1b2c9d0`）
- `{project}` 從 projectPath/workDir 的 basename 經 `SanitizeProjectName` 產生（小寫、去特殊字元、截 20 字元）
- 分組排序的 source of truth 是 session name，透過 `ExtractGroupFromName` 即時解析
- Plan/Dispatch/Retro session 使用 `pplan-`/`pdisp-`/`pretro-` 前綴，不出現在 sidebar
- Epic session 支援兩種模式（啟動時自動偵測 Docker 可用性）：
  - **Docker 模式**：容器化隔離執行（`--dangerously-skip-permissions`），透過 Piper API 操作 vault
  - **Native 模式**：Docker 不可用時的 fallback，直接在 tmux 中運行 claude（保留 permission review）

### Task 狀態機（四態）

```
todo ──→ in-progress ──/submit──→ in-review ──/verify──→ done
           ↑   ↑                    │           │            │
           │   └────────────────────┘           │            │
           │     (/submit 失敗 / /verify 發現產品碼缺口)      │
           │                                    │            │
           └────────────────────────────────────┘            │
                    (/verify 發現 brief/evidence 缺口退回)
```

| 狀態 | 意義 |
|---|---|
| `todo` | 尚未開工 |
| `in-progress` | 實作中（寫 code / 修 code） |
| `in-review` | 自測通過（測試綠、evidence 寫好），等獨立驗證者驗收 |
| `done` | 獨立驗收通過，完成 |

合法轉移：`ValidateStatusTransition` 擋住「跳步」（如 `in-progress → done`），只有 `done → in-progress` 的 reopen 特例。舊資料的 `ready-to-verify` / `verifying` 在 load 時會被 `normalizeLegacyStatuses` 升級為 `in-review`；舊的 `cut` 轉為 `done`（= 放棄等同完結，不再追蹤）。

### Skill 閘門（防自做自驗）

| Skill | 入口 guard | 成功終點 | 職責 |
|---|---|---|---|
| `/submit` | `status == in-progress` | `in-review` | **實作者**自測：worktree + fix loop + diff-gate |
| `/verify` | `status == in-review` | `done`（+ mv brief） | **獨立驗證者**做 C1 evidence / C2 抽驗 / Story 交叉；缺口分流退回 in-review（brief）或 in-progress（產品碼）|

關鍵：`/submit` 和 `/verify` 故意在**不同 session / 不同環境**跑，對抗「自己改自己驗」。C2 抽驗是防作弊主力——重跑 brief 的 `**verify:**` 指令，能抓「evidence 偽造」或「繞過 submit 直接勾 checkbox」。

### Retro 儀式（取代舊 harvest）

Retro 是「關閉 story + 啟 retro 報告」的複合動作，純手動觸發、冪等可重跑：

- **觸發**：`POST /api/epics/{name}/retro/execute`（UI「Retro」按鈕）
- **動作 1**：掃 story map，task 全 done 但 story 還 in-progress / in-review 的 story → 推到 `done`
- **動作 2**：對每個 release，若所有 story 都 done 且無有效 retro 報告（> 200 字）→ 呼叫 `GenerateReport` 並寫入
- **冪等**：已有有效報告的 release 預設 skip；`forceReleases` 可強制重跑
- **失敗判定**：retro 報告檔案 > 200 字才算成功（常數 `MinRetroReportSize`），低於閾值下次 retro 自動重跑

### Retro UI Panel

PlanView 右上角「📊 回顧報告」按鈕開關。Panel 垂直三段：

1. **Trends（趨勢卡片 + sparkline）**：一次通過率、Gap 總數、平均 fix-loop、A/A/N 差距；delta 顏色編碼（綠好黃平紅壞）。資料來自 `.json` snapshot
2. **Actions（改善行動）**：todo/done 清單，可勾選 toggle、inline 新增、hover 刪除；對應 `actions.yaml`
3. **Report Viewer**：release tabs 切換，以 `<pre>` 顯示 markdown 原文（不引入 markdown lib）

**資料持久化**：每次 retro 產報告時同時寫 `release-{label}.md` 和 `release-{label}.json`（`ReportData` 結構化 snapshot）。Legacy report 只有 `.md` 時，trends 以 `HasSnapshot: false` 標記；未來新 release 自動有完整資料。

## Git 慣例
- Commit message 使用**繁體中文**
- 格式：Conventional Commits（feat/fix/refactor/docs/test/chore）
