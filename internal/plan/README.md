# Epic Planning

Dual-Track Agile Story Map 的完整工作流：規劃 → 派送 Task Brief → 收割已完成 Task。

## 包含的 packages

| Package | 路徑 | 職責 |
|---------|------|------|
| **plan** | `internal/plan/` | 核心：Epic 管理、Story Map YAML 解析/驗證/正規化、Git 同步 |
| **vault** | `internal/vault/` | 統一 Git repo 管理（epics + ideas），封裝 `plan.EpicManager` |
| **dispatch** | `internal/dispatch/` | 從 Story Map 批次產出 Task Brief，寫入目標專案的 `.claude/` |
| **harvest** | `internal/harvest/` | 掃描目標專案已完成的 Task Brief，批次更新 Story Map 狀態 |

## 工作流

```
                   plan
                 (Story Map YAML)
                    │
        ┌───────────┼───────────┐
        ▼           │           ▼
    dispatch        │       harvest
  (產出 Brief)      │     (回收完成)
        │           │           ▲
        ▼           │           │
  目標專案          │      目標專案
  .claude/tasks/    │    .claude/tasks/done/
```

三個操作互斥（同一 Epic 同時只能做一個），透過 session 機制鎖定。

## 關鍵介面

### `StoryMapIO`

dispatch 和 harvest 都依賴此介面讀寫 Story Map：

```go
type StoryMapIO interface {
    ReadStoryMap(epicID string) (string, error)
    WriteStoryMap(epicID string, content string) error
}
```

### `ProjectResolver`

將專案名稱對應到檔案系統路徑：

```go
type ProjectResolver interface {
    ResolveProject(name string) (path string, ok bool)
}
```

## Story Map 格式

YAML 結構：Story → Task 兩層，每個 Task 有 status 和 project 欄位。
修改 Story Map **必須透過 API**（`PUT /api/epics/{name}/story-map`），禁止直接寫檔——API 會執行驗證、正規化、WebSocket 通知。

## 相關 API

```
GET    /api/epics                          → Epic 列表
POST   /api/epics                          → 建立 Epic
GET    /api/epics/{name}/story-map         → 取得 Story Map
PUT    /api/epics/{name}/story-map         → 更新 Story Map
GET    /api/epics/{name}/story-map/stats   → 統計
POST   /api/epics/{name}/dispatch-session  → 建立派送 session
POST   /api/epics/{name}/harvest-session   → 建立收割 session
```

### Project-side metadata（manifest + binding）

```
GET    /api/projects/{name}/manifest       → 讀 .plan/manifest.yaml（raw YAML）
GET    /api/bindings/{project}             → 列出本 project 所有 contract binding 的 id
GET    /api/bindings/{project}/{id}        → 讀單一 binding（raw YAML）
PUT    /api/bindings/{project}/{id}        → 寫入 binding（body YAML，id 需與 URL 一致）
DELETE /api/bindings/{project}/{id}        → 刪除 binding
```

> Binding namespace 刻意獨立為 `/api/bindings/...` 而非 `/api/projects/{name}/bindings/...`，避免 Go ServeMux 跟 `/api/projects/setup/...` 路徑衝突。

### Manifest 狀態與失準偵測

- `GET /api/projects` 回傳每個 project 多一個 `hasManifest` 欄位（掃 `.plan/manifest.yaml` 是否存在），前端可據此標記「缺 manifest」
- `POST /api/epics/{name}/contracts/{id}/generate` 回傳的 `artifacts[]` 若某個 target 的父目錄不存在 → `artifacts[].warnings[]` 會有一則「manifest may be stale」提示，使用者可依此判斷要不要跑 `/manifest rescan`

## Project-side Skills

以下 skill 部署到 target project 的 `.claude/skills/`（dispatch 時或 sync-infra 時寫入，`InfraVersion` bump 觸發更新）：

| Skill | 時機 | 職責 |
|---|---|---|
| `/manifest` | 首次 project 註冊 / codebase 結構大改 | 掃 codebase 產 `.plan/manifest.yaml`（transport 類型、test/types pattern、language、build/test 指令） |
| `/bind` | proto 契約的 service.method binding 缺失 / 重綁 | 讀本地 `.proto` 找 service.method → A-gate → PUT 到 `.plan/bindings/{contract-id}.yaml` |
| `/breakdown` | `/dispatch` 派 brief 後 | 拆 sub-tasks + waves；反查 proto 契約缺 binding 時 inline 補完 |
| `/submit` | 實作完成後 | 在 worktree 跑契約測試 + fix loop；綠燈推 status → `in-review`；proto 契約無 binding 時拒跑 |
| `/verify` | `/submit` 綠燈後 | 獨立驗證者做 C1/C2/Story 交叉；推 status → `done` + 搬 brief 到 done/；缺口分流退回 in-review / in-progress |

### Manifest（project 基礎設施宣告）

`.plan/manifest.yaml` 宣告本 project 的 transport 類型（`rest` / `proto` / `mixed`）、契約產物命名模板、建置/測試指令。`/design` 讀它決定產哪種類型契約與 `generates[].path`；`/submit` 讀它判斷測試指令。契約是**邏輯共識**，manifest 是**基礎設施事實**——兩者分層。

### Binding（project-local 契約-到-實作綁定）

Proto 契約在 `/design` 階段只定欄位語意（resource / action / request / response shape），不寫 `service.method` 與 message 名。那些 transport 綁定屬於**本 project 的實作細節**，落在 `.plan/bindings/{contract-id}.yaml`。契約保持 upstream 單一 writer（由 `/design` 唯一寫入），binding 則是 project-local 檔案（可進 git）。`/submit` 產 RPCX 測試骨架時合併契約（remote）+ binding（local）。

Binding 檔 schema：

```yaml
contract_id: dungeon-session-list
contract_version: "1.0"              # 綁到哪版契約
bound_at: 2026-04-21
proto:
  service: SessionService
  method: List
  file: proto/sessions.proto
  request: ListSessionsRequest
  response: ListSessionsResponse
```

## Target project 內的目錄

部署與工作檔案落在 project 的 `.plan/`：

```
.plan/
  manifest.yaml            # /manifest 產出（長期，進 git）
  bindings/                # /bind 產出（長期，進 git）
    {contract-id}.yaml
  briefs/                  # /dispatch 派送目標（暫態）
    task-{id}.md
    done/
      task-{id}.md
```
