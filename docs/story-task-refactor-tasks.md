# Story → Task 架構重構 - 任務追蹤

> 最後更新: 2026-03-30
> 設計討論: 對話紀錄（Story 下新增 Task 層，targetProject 下移到 Task，Breakdown 產出改名 Sub-task）
>
> **歷史記錄** (2026-04-22 重構後)：文中提到的 `done-skill.md` 已改名為
> `verify-skill.md`（因為 skill 職責重新劃分：原 /done 的「最終驗收」語意
> 搬到新 /verify）。原 /verify 的 fix loop 職責搬到新 /submit。實際實作
> 請參考 `CLAUDE.md`。

---

## 會話恢復指令

如果會話遺失，複製以下內容給 Claude：

```
請閱讀以下文件：
1. docs/story-task-refactor-tasks.md
2. internal/plan/storymap.go
3. internal/dispatch/dispatch.go
4. internal/harvest/harvest.go

請根據任務清單中未勾選的項目繼續工作。
查看「並行任務組」區塊，找出當前可執行的任務。
```

**關鍵設計決策**（恢復時需知道）：
1. Story 下新增 `tasks[]` 欄位，每個 Task 帶 `targetProject`；Story 移除 `targetProject`
2. Story 新增 `acceptance[]` 欄位（story 級驗收條件）
3. Task 為強制欄位 — 即使只涉及一個專案也要有 Task
4. 新增 `ready-to-verify` 狀態：所有 task done → story 自動標 ready-to-verify
5. 狀態流：`todo` → `in-progress` → `ready-to-verify` → `done`（+ `cut`）
6. Dispatch 改為派發 Task（一個 Task 一份 Brief）
7. Harvest 追蹤 task done → 全部完成時 story 標 ready-to-verify
8. Breakdown skill 產出從「Task」改名為「Sub-task」
9. Brief 標題從 Story Brief 改為 Task Brief，內容帶 parent story + sibling tasks

---

## 並行任務組

### Wave 1: 資料模型（完全獨立，可同時開始）

| 任務 ID | 檔案 | 狀態 | 說明 | 依賴 |
|---------|------|------|------|------|
| W1-A | `internal/plan/storymap.go`, `internal/plan/storymap_test.go` | [x] | Story 結構加 Task 層 + 驗證邏輯 | 無 |
| W1-B | `frontend/src/plan/types/story-map.ts` | [x] | 前端 TypeScript 型別更新 | 無 |

**並行策略**: W1-A（Go）和 W1-B（TS）完全獨立，可同時執行。

---

### Wave 2: Brief 模板與 Skill（依賴 Wave 1）

| 任務 ID | 檔案 | 狀態 | 說明 | 依賴 |
|---------|------|------|------|------|
| W2-A | `internal/plan/brief.go`, `internal/plan/templates/brief.md` | [x] | Brief 模板改為 Task Brief | W1-A |
| W2-B | `internal/plan/references/breakdown-skill.md` | [x] | Breakdown 產出 Task → Sub-task 改名 | 無 |
| W2-C | `internal/plan/references/done-skill.md` | [x] | Done skill 更新為 task 級別 | 無 |

**並行策略**: W2-B、W2-C 不依賴 Wave 1，可提前或與 W2-A 並行。

---

### Wave 3: Dispatch + Harvest 服務（依賴 Wave 1 + W2-A）

| 任務 ID | 檔案 | 狀態 | 說明 | 依賴 |
|---------|------|------|------|------|
| W3-A | `internal/dispatch/dispatch.go`, `internal/dispatch/dispatch_test.go` | [x] | Dispatch 改為以 Task 為單位派發 | W1-A, W2-A |
| W3-B | `internal/harvest/harvest.go`, `internal/harvest/harvest_test.go` | [x] | Harvest 追蹤 task → ready-to-verify 邏輯 | W1-A |

**並行策略**: W3-A 和 W3-B 互不依賴，可同時執行。

---

### Wave 4: API 路由 + 前端（依賴 Wave 3）

| 任務 ID | 檔案 | 狀態 | 說明 | 依賴 |
|---------|------|------|------|------|
| W4-A | `routes_epics.go`, `routes_test.go` | [x] | API handler 適配新 dispatch/harvest 介面（無需改動） | W3-A, W3-B |
| W4-B | `frontend/src/plan/lib/yaml-loader.ts`, `frontend/src/__tests__/normalize-story-map.test.ts` | [x] | YAML loader 解析 tasks + acceptance | W1-B |
| W4-C | `frontend/src/plan/lib/story-map-actions.ts` | [x] | Story map actions 新增 task CRUD | W1-B, W4-B |

**並行策略**: W4-A（Go）與 W4-B、W4-C（TS）可並行。W4-C 依賴 W4-B。

---

### Wave 5: 前端 UI + 整合驗證（依賴 Wave 4）

| 任務 ID | 檔案 | 狀態 | 說明 | 依賴 |
|---------|------|------|------|------|
| W5-A | `frontend/src/plan/components/StoryDetail.tsx`, `frontend/src/plan/hooks/use-story-map-editor.ts` | [x] | StoryDetail 顯示/編輯 tasks + acceptance | W4-C |
| W5-B | `frontend/src/plan/components/StoryCard.tsx` | [x] | StoryCard 顯示 task 數量指標 | W4-C |
| W5-C | 全專案 | [x] | 整合測試：`go test ./...` + `npm run build` | 全部 |

**並行策略**: W5-A 和 W5-B 可並行。W5-C 最後執行。

---

## 依賴關係圖

```
Wave 1 (完全並行)
┌────────┬────────┐
│ W1-A   │ W1-B   │
│ Go型別 │ TS型別 │
└───┬────┴───┬────┘
    │        │
    ▼        ▼
Wave 2 (部分並行)
┌────────┬────────┬────────┐
│ W2-A   │ W2-B   │ W2-C   │
│ Brief  │Breakdn │ Done   │
└───┬────┴────────┴────────┘
    │
    ▼
Wave 3 (完全並行)
┌────────┬────────┐
│ W3-A   │ W3-B   │
│Dispatch│Harvest │
└───┬────┴───┬────┘
    │        │
    ▼        ▼
Wave 4 (部分並行)
┌────────┬────────┬────────┐
│ W4-A   │ W4-B   │ W4-C   │
│ API    │YAMLldr │Actions │
└───┬────┴────────┴───┬────┘
    │                 │
    ▼                 ▼
Wave 5 (部分並行)
┌────────┬────────┬────────┐
│ W5-A   │ W5-B   │ W5-C   │
│Detail  │Card    │整合驗證│
└────────┴────────┴────────┘
```

---

## 詳細任務清單

### W1-A: Story 結構加 Task 層 `internal/plan/storymap.go`
- [ ] 新增 `Task` struct（ID, Name, Description, TargetProject, Status）
- [ ] `Story` 新增 `Tasks []Task` 欄位
- [ ] `Story` 新增 `Acceptance []string` 欄位
- [ ] `Story` 移除 `TargetProject` 欄位
- [ ] `validStatuses` 新增 `"ready-to-verify"`
- [ ] `ValidateStoryMap` 新增 Task 驗證：每個 story 必須有 ≥1 task、task ID 唯一、task 必須有 targetProject
- [ ] 新增 `TaskContext` struct（Task + parent Story 上下文 + sibling tasks）
- [ ] `FilterDispatchable` 改為回傳 `map[string][]TaskContext`（以 task 為單位，按 targetProject 分組）
- [ ] 新增 `FindTask(id)` 方法
- [ ] 新增 `SetTaskStatus(taskID, status)` 方法
- [ ] 更新 `SetStoryStatus` 保留但可能需要給 harvest 用（ready-to-verify）
- [ ] 編寫對應測試更新 `storymap_test.go`

### W1-B: 前端 TypeScript 型別 `frontend/src/plan/types/story-map.ts`
- [ ] 新增 `Task` interface（id, name, description, targetProject, status）
- [ ] `Story` 新增 `tasks: Task[]` 和 `acceptance: string[]`
- [ ] `Story` 移除 `targetProject`
- [ ] `StoryStatus` 新增 `'ready-to-verify'`

### W2-A: Brief 模板改為 Task Brief `internal/plan/brief.go`
- [ ] `BriefData` 更名/調整欄位：加 TaskID, TaskName, ParentStoryID, ParentStoryName, SiblingTasks
- [ ] 更新 `templates/brief.md`：標題改為「Task Brief」，上下文表加 Parent Story、Sibling Tasks 區塊
- [ ] 新增 `SiblingTask` struct（ID, Name, TargetProject, Status）
- [ ] 更新 `GenerateBrief` 函數簽名
- [ ] 編寫測試

### W2-B: Breakdown Skill 改名 `internal/plan/references/breakdown-skill.md`
- [ ] 所有「Task」改為「Sub-task」（在輸出格式區塊）
- [ ] 「Task 拆分」section 標題改為「Sub-task 拆分」
- [ ] 加入名詞定義：「本文件中的 Sub-task 指 Breakdown 拆分的執行單位；Task 指 Story Map 中的工作單位」

### W2-C: Done Skill 更新 `internal/plan/references/done-skill.md`
- [ ] 掃描目標從 `story-*.md` 改為 `task-*.md`（或沿用 ID 前綴）
- [ ] 驗收檢查對象調整為 Task Brief 的驗收條件
- [ ] 移動路徑維持 `.plan/briefs/done/`

### W3-A: Dispatch 改為 Task 派發 `internal/dispatch/dispatch.go`
- [ ] `PreviewResult.Groups` value 型別從 `[]PreviewStory` 改為 `[]PreviewTask`
- [ ] 新增 `PreviewTask` struct
- [ ] `DispatchResult.Dispatched` 改為 `[]DispatchedTask`
- [ ] `Execute` 遍歷 story.tasks（而非 stories），以 task 為單位生成 brief
- [ ] `dispatchStory` 改為 `dispatchTask`：生成 Task Brief
- [ ] Story 狀態更新：任一 task 被 dispatch → story 設為 `in-progress`
- [ ] Brief 檔名從 `{storyID}.md` 改為 `{taskID}.md`
- [ ] 更新測試 `dispatch_test.go`

### W3-B: Harvest 追蹤 Task `internal/harvest/harvest.go`
- [ ] 掃描邏輯改為：遍歷 in-progress stories 的 tasks
- [ ] 檢查每個 task 的 done file：`.plan/briefs/done/{taskID}.md`
- [ ] 單個 task done → `SetTaskStatus(taskID, "done")`
- [ ] 所有 tasks 都 done → `SetStoryStatus(storyID, "ready-to-verify")`
- [ ] Preview 回傳改為 task 級別的完成狀態
- [ ] 更新測試 `harvest_test.go`

### W4-A: API handler 適配 `routes_epics.go`
- [ ] Dispatch preview/execute handler 適配新的 service 回傳型別
- [ ] Harvest preview/execute handler 適配新的 service 回傳型別
- [ ] 更新 `routes_test.go` 中 dispatch/harvest 相關測試

### W4-B: YAML Loader 解析 Tasks `frontend/src/plan/lib/yaml-loader.ts`
- [ ] `normalizeStory` 新增 tasks 和 acceptance 解析
- [ ] 新增 `normalizeTask` 函數
- [ ] `serializeStoryMap` 確保 tasks 正確序列化
- [ ] 更新測試 `normalize-story-map.test.ts`

### W4-C: Story Map Actions `frontend/src/plan/lib/story-map-actions.ts`
- [ ] 新增 `addTask(data, storyId, task)` action
- [ ] 新增 `updateTask(data, storyId, taskId, changes)` action
- [ ] 新增 `deleteTask(data, storyId, taskId)` action
- [ ] 新增 `updateStoryAcceptance(data, storyId, acceptance)` action
- [ ] 移除 targetProject 相關的 story-level 邏輯（如果有）

### W5-A: StoryDetail UI `frontend/src/plan/components/StoryDetail.tsx`
- [ ] 顯示 acceptance 清單（可編輯）
- [ ] 顯示 tasks 列表（ID、名稱、targetProject、狀態）
- [ ] 新增/刪除 task 的 UI
- [ ] 編輯 task 的 targetProject（下拉選單或輸入框）

### W5-B: StoryCard 指標 `frontend/src/plan/components/StoryCard.tsx`
- [ ] 顯示 task 數量 badge（例如 "3 tasks"）
- [ ] 可選：顯示 task 完成進度（2/3）

### W5-C: 整合驗證
- [ ] `go test ./...` 全部通過
- [ ] `cd frontend && npm run build` 通過
- [ ] 手動測試：建立含 tasks 的 story map → dispatch → harvest 完整流程

---

## 檔案變更總覽

### 修改檔案
| 檔案 | Wave | 狀態 |
|------|------|------|
| `internal/plan/storymap.go` | W1-A | [ ] |
| `internal/plan/storymap_test.go` | W1-A | [ ] |
| `frontend/src/plan/types/story-map.ts` | W1-B | [ ] |
| `internal/plan/brief.go` | W2-A | [ ] |
| `internal/plan/templates/brief.md` | W2-A | [ ] |
| `internal/plan/references/breakdown-skill.md` | W2-B | [ ] |
| `internal/plan/references/done-skill.md` | W2-C | [ ] |
| `internal/dispatch/dispatch.go` | W3-A | [ ] |
| `internal/dispatch/dispatch_test.go` | W3-A | [ ] |
| `internal/harvest/harvest.go` | W3-B | [ ] |
| `internal/harvest/harvest_test.go` | W3-B | [ ] |
| `routes_epics.go` | W4-A | [ ] |
| `routes_test.go` | W4-A | [ ] |
| `frontend/src/plan/lib/yaml-loader.ts` | W4-B | [ ] |
| `frontend/src/__tests__/normalize-story-map.test.ts` | W4-B | [ ] |
| `frontend/src/plan/lib/story-map-actions.ts` | W4-C | [ ] |
| `frontend/src/plan/components/StoryDetail.tsx` | W5-A | [ ] |
| `frontend/src/plan/hooks/use-story-map-editor.ts` | W5-A | [ ] |
| `frontend/src/plan/components/StoryCard.tsx` | W5-B | [ ] |
