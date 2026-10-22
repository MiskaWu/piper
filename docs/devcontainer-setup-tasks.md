# DevContainer 專案建立 - 任務追蹤

> 最後更新: 2026-03-17
> 功能：從 piper Web UI 建立新專案，用 Claude Opus 產生 DevContainer 配置，一鍵啟動容器

---

## 會話恢復指令

如果會話遺失，複製以下內容給 Claude：

```
請閱讀以下文件：
1. docs/devcontainer-setup-tasks.md
2. internal/devcontainer/devcontainer.go
3. internal/tmux/tmux.go（CreatePlanSession 模式參考）
4. ~/.claude/skills/devcontainer-setup/references/base-dockerfile.md
5. ~/.claude/skills/devcontainer-setup/references/devcontainer-json.md

請根據任務清單中未勾選的項目繼續工作。
```

**關鍵設計決策**：
1. 使用 `psetup-*` 前綴的短命 session，模式參考 `pplan-*`
2. 用 `claude -p --model opus` 產生配置（非互動，pipe 模式）
3. 參考文件 embed 進 Go binary，不依賴外部 skill 路徑
4. 進度透過 WebSocket 推播（`setup_progress` 事件）
5. Auto Mode 預設開啟
6. 路徑選擇：後端提供 `GET /api/fs/dirs` 列出目錄供前端選擇

---

## 並行任務組

### Wave 1: 後端核心（完全獨立，可同時開始）

| 任務 ID | 檔案 | 狀態 | 說明 | 依賴 |
|---------|------|------|------|------|
| W1-A | `internal/devcontainer/generator.go` | [x] | 配置產生器：呼叫 claude -p 產生 .devcontainer/ | 無 |
| W1-B | `internal/devcontainer/templates/` | [x] | Embed prompt 模板 + 參考文件 | 無 |
| W1-C | `internal/devcontainer/setup.go` | [x] | Setup 流程編排：產生 → 寫檔 → devcontainer up | 無 |
| W1-D | `internal/tmux/tmux.go` | [x] | 新增 Setup session 管理方法（類似 PlanSession） | 無 |

**並行策略**: W1-A/B/C 是 devcontainer package 內部，W1-D 是 tmux package，四個完全獨立

---

### Wave 2: API 路由 + Store（依賴 Wave 1）

| 任務 ID | 檔案 | 狀態 | 說明 | 依賴 |
|---------|------|------|------|------|
| W2-A | `internal/store/store.go` | [x] | 擴充 ProjectEntry：description 欄位 | W1-C |
| W2-B | `routes.go` | [x] | 新增 API endpoints | W1-A, W1-C, W1-D |
| W2-C | `routes.go` | [x] | 目錄瀏覽 API：已有 `GET /api/directories/browse` | 無（可與 W1 並行） |

**並行策略**: W2-C 不依賴 Wave 1，可提前開始；W2-A/B 等 Wave 1 完成

---

### Wave 3: WebSocket + 生命週期（依賴 Wave 2）

| 任務 ID | 檔案 | 狀態 | 說明 | 依賴 |
|---------|------|------|------|------|
| W3-A | `routes.go` + `main.go` | [x] | Setup 進度推播 + 孤兒清理 | W2-B |

---

### Wave 4: 前端（依賴 Wave 2/3）

| 任務 ID | 檔案 | 狀態 | 說明 | 依賴 |
|---------|------|------|------|------|
| W4-A | `frontend/src/api.ts` | [x] | 新增 API 呼叫（setupProject, getSetupStatus, cancelSetup） | W2-B, W2-C |
| W4-B | `frontend/src/components/NewProjectModal.tsx` | [x] | 建立專案對話框：名稱 + 路徑選擇 + 描述 + 進度 | W4-A |
| W4-C | `frontend/src/components/Sidebar.tsx` | [x] | Header 加「New Project」按鈕，觸發 Modal | W4-B |
| W4-D | `frontend/src/hooks/useStatusSocket.ts` | [x] | setup_progress WebSocket 事件分發到 window | W3-A, W4-A |

**並行策略**: W4-A 先行，W4-B/C/D 可並行

---

## 依賴關係圖

```
Wave 1 (完全並行)
┌──────────┬──────────┬──────────┬──────────┐
│ W1-A     │ W1-B     │ W1-C     │ W1-D     │
│ Generator│ Templates│ Setup    │ Tmux     │
└────┬─────┴────┬─────┴────┬─────┴────┬─────┘
     │          │          │          │
     ▼          ▼          ▼          ▼
Wave 2                          ┌──────────┐
┌──────────┬──────────┐         │ W2-C     │
│ W2-A     │ W2-B     │         │ FS API   │
│ Store    │ Routes   │         │ (獨立)   │
└────┬─────┴────┬─────┘         └────┬─────┘
     │          │                    │
     ▼          ▼                    │
Wave 3                               │
┌──────────────────┐                 │
│ W3-A WS + 清理   │                 │
└────┬─────────────┘                 │
     │                               │
     ▼                               ▼
Wave 4
┌──────────────────────────────────────┐
│ W4-A API Client                      │
├──────────┬──────────┬────────────────┤
│ W4-B     │ W4-C     │ W4-D          │
│ Modal    │ Sidebar  │ Progress      │
└──────────┴──────────┴────────────────┘
```

---

## 詳細任務清單

### W1-A: 配置產生器 `internal/devcontainer/generator.go`

核心：呼叫 `claude -p --model opus` 產生 DevContainer 配置檔案

- [ ] 定義 `GenerateConfig` 結構（ProjectName, ProjectPath, Description, AutoMode）
- [ ] 定義 `Generator` struct，注入 Commander
- [ ] 實作 `Generate(ctx, cfg) (GeneratedFiles, error)` 方法
  - 組裝 prompt（系統指令 + 參考文件 + 使用者描述）
  - 呼叫 `claude -p --model opus --output-format json`
  - 解析回傳的檔案內容（Dockerfile, devcontainer.json, docker-compose.yml, init-firewall.sh）
- [ ] 定義 `GeneratedFiles` struct（map[string]string: 檔案路徑 → 內容）
- [ ] 編寫測試：mock Commander 驗證 prompt 組裝 + 輸出解析

### W1-B: Embed 模板與參考文件 `internal/devcontainer/templates/`

- [ ] 從 skill 複製參考文件到 `templates/` 目錄
  - `base-dockerfile.md` — Dockerfile 撰寫規範
  - `devcontainer-json.md` — devcontainer.json 規範
  - `init-firewall.sh` — 防火牆腳本（直接使用，不需 Claude 產生）
- [ ] 建立 `prompt.md.tmpl` — 給 Claude 的系統 prompt 模板
- [ ] 使用 `go:embed` 嵌入所有檔案
- [ ] 編寫測試：驗證 embed 檔案可讀取、prompt 模板可渲染

### W1-C: Setup 流程編排 `internal/devcontainer/setup.go`

- [ ] 定義 `SetupPhase` 常數（Generating, Writing, Building, Running, Done, Failed）
- [ ] 定義 `SetupStatus` struct（Phase, Message, Error）
- [ ] 定義 `ProgressCallback func(SetupStatus)`
- [ ] 實作 `Setup(ctx, cfg, progressCb) error` 方法
  - Phase 1: Generating — 呼叫 Generator.Generate()
  - Phase 2: Writing — 將 GeneratedFiles 寫入 ProjectPath/.devcontainer/
  - Phase 3: Building — 呼叫 EnsureRunning()
  - Phase 4: Done
- [ ] 錯誤處理：任一階段失敗 → Failed phase + 錯誤訊息
- [ ] 編寫測試：mock Generator + Commander，驗證各階段回呼順序

### W1-D: Setup Session 管理 `internal/tmux/tmux.go`

參考 `CreatePlanSession` / `ListPlanSessions` / `CleanOrphanPlanSessions` 模式

- [ ] 新增 `ListSetupSessions() ([]TmuxSession, error)` — 過濾 `psetup-*` 前綴
- [ ] 新增 `CreateSetupSession(projectName, workDir string) (string, error)`
  - Session 名: `psetup-{projectName}`
  - 冪等：已存在則返回
- [ ] 新增 `CleanOrphanSetupSessions(validProjects map[string]bool) error`
- [ ] 編寫測試：table-driven，mock Commander

---

### W2-A: Store 擴充 `internal/store/store.go`

- [ ] `ProjectEntry` 新增 `Description string` 欄位
- [ ] 新增 `AddProjectWithDescription(path, description string) error`
- [ ] 確保向後相容（舊資料無 description 欄位不報錯）
- [ ] 編寫測試

### W2-B: API Routes `routes.go`

新增 endpoints：

- [ ] `POST /api/projects/setup` — 觸發完整建立流程
  - Request: `{ "name": "string", "path": "string", "description": "string", "autoMode": true }`
  - Response: `{ "project": Project, "setupID": "string" }`
  - 非同步執行 Setup，立即回傳 setupID
- [ ] `GET /api/projects/setup/{id}/status` — 查詢 setup 進度
  - Response: `{ "phase": "generating|writing|building|running|done|failed", "message": "string" }`
- [ ] `DELETE /api/projects/setup/{id}` — 取消 setup
- [ ] Handler 測試：成功路徑 + 驗證失敗（缺名稱/缺描述）+ 內部錯誤

### W2-C: 目錄瀏覽 API `routes.go`

- [ ] `GET /api/fs/dirs?path={path}` — 列出子目錄
  - 預設 path = `$HOME`
  - Response: `{ "current": "/home/user", "parent": "/home", "dirs": ["projects", "work", ...] }`
  - 安全：不顯示隱藏目錄（`.` 開頭），不跟隨 symlink 到系統目錄
- [ ] Handler 測試

---

### W3-A: WebSocket 推播 + 啟動清理 `routes.go` + `main.go`

- [ ] Setup 進度透過 WebSocket 推播 `setup_progress` 事件
  ```json
  { "type": "setup_progress", "setupID": "xxx", "phase": "building", "message": "..." }
  ```
- [ ] `main.go` 啟動時呼叫 `CleanOrphanSetupSessions()`
- [ ] 編寫測試

---

### W4-A: Frontend API `frontend/src/api.ts`

- [ ] `setupProject(name, path, description, autoMode): Promise<{ project: Project, setupID: string }>`
- [ ] `getSetupStatus(setupID): Promise<SetupStatus>`
- [ ] `cancelSetup(setupID): Promise<void>`
- [ ] `listDirs(path?): Promise<{ current: string, parent: string, dirs: string[] }>`

### W4-B: 建立專案對話框 `frontend/src/components/NewProjectModal.tsx`

- [ ] 表單欄位：專案名稱（必填）、路徑（目錄瀏覽器）、描述（textarea，必填）
- [ ] Auto Mode toggle（預設開）
- [ ] 目錄瀏覽器：點擊進入子目錄、返回上層、顯示當前路徑
- [ ] 提交後顯示進度（接 W4-D）

### W4-C: Sidebar 整合 `frontend/src/components/Sidebar.tsx`

- [ ] Projects 區塊標題旁加「+」按鈕
- [ ] 點擊開啟 NewProjectModal

### W4-D: Setup 進度元件 `frontend/src/components/SetupProgress.tsx`

- [ ] 監聽 WebSocket `setup_progress` 事件
- [ ] 顯示當前階段 + 進度條/步驟指示器
- [ ] 完成後自動關閉 Modal，刷新 Projects 列表
- [ ] 失敗時顯示錯誤 + 重試按鈕

---

## 檔案變更總覽

### 新增檔案
| 檔案 | Wave | 狀態 |
|------|------|------|
| `internal/devcontainer/generator.go` | W1-A | [ ] |
| `internal/devcontainer/generator_test.go` | W1-A | [ ] |
| `internal/devcontainer/setup.go` | W1-C | [ ] |
| `internal/devcontainer/setup_test.go` | W1-C | [ ] |
| `internal/devcontainer/templates/prompt.md.tmpl` | W1-B | [ ] |
| `internal/devcontainer/templates/base-dockerfile.md` | W1-B | [ ] |
| `internal/devcontainer/templates/devcontainer-json.md` | W1-B | [ ] |
| `internal/devcontainer/templates/init-firewall.sh` | W1-B | [ ] |
| `frontend/src/components/NewProjectModal.tsx` | W4-B | [ ] |
| `frontend/src/components/SetupProgress.tsx` | W4-D | [ ] |

### 修改檔案
| 檔案 | Wave | 狀態 |
|------|------|------|
| `internal/tmux/tmux.go` | W1-D | [ ] |
| `internal/tmux/tmux_test.go` | W1-D | [ ] |
| `internal/store/store.go` | W2-A | [ ] |
| `internal/store/store_test.go` | W2-A | [ ] |
| `routes.go` | W2-B, W2-C, W3-A | [ ] |
| `main.go` | W3-A | [ ] |
| `frontend/src/api.ts` | W4-A | [ ] |
| `frontend/src/components/Sidebar.tsx` | W4-C | [ ] |

---

## 驗證任務

### 單元測試
- [ ] Generator: prompt 組裝正確、Claude 輸出解析正確
- [ ] Setup: 階段回呼順序正確、錯誤處理正確
- [ ] Tmux: psetup-* session CRUD
- [ ] Store: description 欄位存取、向後相容
- [ ] Routes: API 契約測試（所有 endpoint）

### 整合測試
- [ ] 端到端：建立專案 → 產生配置 → 檔案寫入正確
- [ ] WebSocket 推播：各階段事件正確送達
- [ ] 目錄瀏覽：安全性驗證（不能讀 /etc 等）

### 手動驗證
- [ ] UI 操作流程順暢
- [ ] 產生的 DevContainer 配置可正常 `devcontainer up`
- [ ] Auto Mode 正確注入 postAttachCommand
