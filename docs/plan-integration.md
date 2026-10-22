# Plan 整合實作規格

> 將 Plan（YAML 驅動的 Story Map 規劃工具）直接搬入 PIPER，
> 統一「規劃 Epic」與「管理 Claude Code session 實作」的工作流。
>
> **命名慣例：**
> - **Plan** — PIPER 中的功能模組名稱（規劃功能）
> - **Epic** — Plan 中的資料單位（一個跨專案的大故事）

---

## 架構決策摘要

| 決策 | 結論 |
|------|------|
| 整合方式 | 直接搬入（非 submodule），Plan 不再作為獨立工具 |
| 規劃單位 | Epic（跨多個實作 repo 的大型工作主體） |
| Epic 資料存放 | `~/.local/share/piper/epics/{name}/`，可 Git 版控 |
| Story 指向實作 | `targetProject` 欄位，對應 PIPER 已註冊的專案 |
| Brief 輸出 | 寫入目標專案 `{project}/.plan/briefs/story-{id}.md` |
| 前端語言 | 漸進引入 TypeScript（Plan 搬入保持 .tsx，PIPER 既有 .jsx 暫不改） |
| React 版本 | PIPER 已是 19，Plan 元件直接相容 |
| 主題 | 統一用 PIPER 的 useTheme，Plan CSS variable 加 `--plan-*` 前綴 |
| D2 圖表 | Phase 1 不搬，後續再評估 |
| 方法論指引 | 不放 CLAUDE.md 範本，改為 PIPER 專案內的 Skill（`/analyze-epic`、`/brief`） |

---

## Epic 目錄結構

```
~/.local/share/piper/epics/
├── user-auth/                  # Epic: 用戶認證系統
│   ├── story-map.yaml          # Story Map（含 targetProject 欄位）
│   ├── problem-statement.md    # 問題陳述
│   ├── adr/                    # 架構決策紀錄
│   │   └── 001-oauth-provider.md
│   ├── spikes/                 # 技術探針
│   │   └── session-storage.md
│   └── diagrams/               # 技術圖表（YAML）
│       └── {feature}/
├── cdn-optimization/           # 另一個 Epic
│   └── ...
```

---

## Story Map YAML Schema（擴充）

原有 Schema 不變，Story 新增 `targetProject` 選填欄位：

```yaml
meta:
  name: "Epic 名稱"
  updated: "2026-03-16"
  owner: "Miska"
  releases:
    - id: rel-xxxxx
      label: "MVP"
      color: blue

activities:
  - id: act-xxxxx
    name: "活動名稱"
    steps:
      - id: step-xxxxx
        name: "步驟名稱"
        stories:
          - id: story-xxxxx
            name: "故事標題"
            description: "身為 {角色}，我想要 {目標}，以便 {價值}"
            targetProject: backend    # 新增：對應 PIPER 已註冊的專案名稱（選填）
            tags: [backend, frontend]
            release: rel-xxxxx
            spike: null
            adr: null
            status: todo              # todo | in-progress | done | cut
```

ID 格式：`{prefix}-` + 5 字元英數字短碼（`rel-`、`act-`、`step-`、`story-`）。

---

## API 設計

### Epic 管理

| Method | Path | Body | 說明 |
|--------|------|------|------|
| GET | `/api/epics` | — | 列出所有 epic（掃描 `~/.local/share/piper/epics/`） |
| POST | `/api/epics` | `{name}` | 新建 epic（scaffold 目錄 + 範本） |
| DELETE | `/api/epics/{name}` | — | 刪除 epic |

### Story Map 讀寫

| Method | Path | Body | 說明 |
|--------|------|------|------|
| GET | `/api/epics/{name}/story-map` | — | 讀取 YAML（text/yaml） |
| PUT | `/api/epics/{name}/story-map` | YAML body | 回寫 YAML（atomic write） |

### Brief 與實作

| Method | Path | Body | 說明 |
|--------|------|------|------|
| POST | `/api/epics/{name}/brief` | `{storyId}` | 產出 brief → 寫入 targetProject |
| GET | `/api/epics/{name}/brief/{storyId}` | — | 讀取已產出的 brief |

### WebSocket 擴充

現有 `/ws/status` 新增事件類型：

```jsonc
{
  "type": "plan_update",
  "epic": "user-auth",
  "timestamp": "2026-03-16T..."
}
```

---

## 前端整合

### 入口方式

PIPER 是 sidebar + main view 架構，Plan 作為新的 main view：

```
Sidebar
├── Sessions 列表
├── [Plan] ← 新增分組
│   ├── 用戶認證系統      ← Epic
│   └── CDN 優化          ← Epic
├── Stats
└── Inbox

Main View
├── Terminal（選中 session 時）
├── StatsPanel（選中 stats 時）
├── PlanView（選中某個 epic 時）  ← 新增
└── EmptyState（預設）
```

### 搬入的前端模組

**直接搬入（無需修改）：**

| 模組 | 來源路徑 | 說明 |
|------|---------|------|
| `story-map-actions.ts` | `src/lib/` | 11 個 immutable reducer |
| `yaml-loader.ts` | `src/lib/` | YAML 解析 + Release 自動推導 |
| `id-generator.ts` | `src/lib/` | 碰撞安全 ID 產生器 |
| `release-palette.ts` | `src/lib/` | 8 色調色盤（dark/light） |
| `types/story-map.ts` | `src/types/` | 型別定義（新增 targetProject） |
| `use-story-drag-drop.ts` | `src/hooks/` | 拖放狀態管理 |

**元件搬入（小幅調整 CSS）：**

| 元件 | 來源路徑 | 說明 |
|------|---------|------|
| `StoryMap.tsx` + `.css` | `src/components/` | 二維卡片牆，props 接口完備 |
| `StoryCard.tsx` + `.css` | `src/components/` | 可拖拉卡片 |
| `StoryDetail.tsx` + `.css` | `src/components/` | 側邊詳情面板 |
| `AddStoryButton.tsx` + `.css` | `src/components/` | Inline 新增故事 |
| `AddReleaseRow.tsx` + `.css` | `src/components/` | 新增 Release |
| `TagFilter.tsx` + `.css` | `src/components/` | 標籤篩選 |

**需重構：**

| 模組 | 改動 |
|------|------|
| `use-story-map-editor.ts` | 移除 `useYamlHmr`；API 改為 `/api/epics/{name}/story-map`；監聽改為 WebSocket `plan_update` |
| `saveStoryMap()` in `story-map-actions.ts` | 改呼叫 PIPER API |

**不搬入：**

| 模組 | 原因 |
|------|------|
| `DiagramViewer.tsx` | Phase 1 不做圖表 |
| `ThemeSwitcher.tsx` | PIPER 已有 |
| `useTheme.ts` | 統一用 PIPER 的 |
| `useYamlHmr.ts` | 改為 WebSocket |
| `src/lib/build/*` | D2 建置，Phase 1 不做 |
| `src/lib/api.ts` | API 路徑全部換 |
| `App.tsx` / `App.css` | 由 PlanView.tsx 取代 |

### CSS 整合

Plan 的 CSS variable 全部加 `--plan-*` 前綴：

```css
/* 加入 PIPER 的 global.css */
[data-theme="dark"] {
  --plan-bg: #1a1a2e;
  --plan-bg-surface: #16213e;
  --plan-bg-card: #0f3460;
  --plan-bg-card-hover: #1a4a7a;
  --plan-text: #e0e0e0;
  --plan-text-muted: #8899aa;
  --plan-text-dim: #556677;
  --plan-border: #2a3a5a;
  --plan-accent: #53a8e2;
  --plan-tag-bg: rgba(83, 168, 226, 0.15);
  --plan-drag-over-bg: rgba(83, 168, 226, 0.1);
  --plan-status-todo: #6b7280;
  --plan-status-in-progress: #3b82f6;
  --plan-status-done: #22c55e;
  --plan-status-cut: #ef4444;
}

[data-theme="light"] {
  /* 對應淺色值 */
}
```

Plan 元件 CSS 中所有 `var(--bg)` → `var(--plan-bg)` 等。

### TypeScript 引入

1. 新增 `frontend/tsconfig.json`（`allowJs: true`）
2. Plan 搬入的檔案保持 `.tsx` / `.ts`
3. PIPER 既有 `.jsx` 暫不改
4. 新增 `yaml` npm 依賴

---

## Go 後端新增

### 目錄結構

```
internal/
└── plan/
    ├── plan.go            # Epic CRUD + YAML 讀寫 + fsnotify 監聽
    ├── brief.go           # Brief 產出邏輯
    └── plan_test.go
templates/
├── story-map.yaml         # go:embed 嵌入
└── problem-statement.md
```

### plan.go 核心功能

- `ListEpics()` — 掃描 `~/.local/share/piper/epics/` 回傳 epic 列表
- `CreateEpic(name)` — 建立目錄 + scaffold story-map.yaml、problem-statement.md、adr/、spikes/
- `DeleteEpic(name)` — 刪除 epic 目錄
- `ReadStoryMap(name)` — 讀取 YAML 回傳 raw text
- `WriteStoryMap(name, yaml)` — atomic write（先寫 .tmp 再 rename）
- `WatchEpics()` — fsnotify 監聽 epics 目錄，YAML 變更時通知 WebSocket hub

### brief.go 核心功能

- `GenerateBrief(epicName, storyId)` — 從 YAML 提取 story + 關聯 ADR/Spike，組裝 brief markdown
- 輸出位置由 story 的 `targetProject` 決定，對應到 PIPER 已註冊的專案路徑
- 寫入 `{projectPath}/.plan/briefs/story-{id}.md`

---

## Claude Code Skills

已建立在 `piper/.claude/skills/`：

### `/analyze-epic [epic-name]`
- 讀取 epic 目錄，扮演 Dual-Track Agile 協作夥伴
- 引導寫 Problem Statement、整理 Story Map、提醒 ADR/Spike
- 包含完整方法論、模板格式、品質檢查規則

### `/brief [epic-name] [story-id]`
- 從 Epic 的 Story Map 提取 story，產出 handoff 文件
- 根據 `targetProject` 寫入目標專案的 `.plan/briefs/`

---

## 實作階段

### Phase 1：Story Map 顯示與編輯

**前端（可並行）：**

| # | 任務 | 依賴 |
|---|------|------|
| 1 | 前端加入 TypeScript 支援（tsconfig + allowJs） | — |
| 2 | 搬入 Plan 純邏輯模組（types、lib、drag-drop hook） | #1 |
| 3 | 搬入 Plan 元件（StoryMap 全家族 + CSS） | #2 |
| 4 | CSS variable 重命名 `--plan-*` 前綴，合併進 global.css | #3 |
| 7 | 重構 `useStoryMapEditor`（移除 HMR、改用 Epic API） | #2, #5 |
| 8 | 建立 `PlanView.tsx`（整合容器） | #3, #4, #7 |
| 9 | Sidebar 加入 Plan 分組（列出 Epics） | #8 |

**後端（可並行）：**

| # | 任務 | 依賴 |
|---|------|------|
| 5 | `internal/plan/plan.go`：Epic CRUD + YAML 讀寫 API | — |
| 6 | Templates go:embed + init 邏輯 | #5 |
| 10 | fsnotify 監聽 + WebSocket 推播 `plan_update` | #5 |

**並行策略：** #1-4（前端搬入）和 #5-6（後端 API）可同時進行。#7 是銜接點，需要前後端都完成。

### Phase 2：Story → Session 串接

| # | 任務 | 依賴 |
|---|------|------|
| 11 | Story 型別加入 `targetProject` + StoryDetail UI 顯示/編輯 | Phase 1 |
| 12 | `internal/plan/brief.go`：Brief 產出 API | Phase 1 |
| 13 | StoryDetail 加「▶ 開始實作」按鈕 | #11, #12 |
| 14 | 串接：Brief → 建立 Session → 注入 prompt | #13 |
| 15 | Session metadata 記錄 epicName + storyId | #14 |
| 16 | Story status 雙向同步（Session 完成 → Story done） | #15 |

### Phase 3：D2 圖表（可選）

| # | 任務 |
|---|------|
| 17 | 評估前端 WASM vs Go CLI 方案 |
| 18 | 搬入 diagram 渲染邏輯 |
| 19 | 搬入 DiagramViewer 元件 |

---

## 核心工作流

### 規劃流程

```
PIPER UI → Plan 分組 → 選擇 Epic（或新建）
  → 看到 Story Map 二維卡片牆
  → 拖拉、新增、編輯 story（每次操作即時回寫 YAML）
  → 需要深度分析 → 在 PIPER 開 Claude Code session，跑 /analyze-epic
```

### Story → Session 實作流程（Phase 2）

```
Story Map → 點擊某個 story → 側邊面板
  → 點「▶ 開始實作」
  → 後端：產出 brief → 寫入 targetProject/.plan/briefs/
  → 後端：在 targetProject 建立 PIPER session
  → Session 初始 prompt："請閱讀 .plan/briefs/story-xxxxx.md 了解需求"
  → Story status → in-progress
  → 前端跳轉到該 session 終端
```

---

## Plan 原始碼參考

所有要搬入的程式碼來自 `/home/user/projects/plan/`：

| 來源目錄 | 目標目錄 | 說明 |
|---------|---------|------|
| `src/components/StoryMap.tsx` 等 | `frontend/src/plan/components/` | UI 元件 |
| `src/hooks/use-story-*.ts` | `frontend/src/plan/hooks/` | Hooks |
| `src/lib/*.ts` | `frontend/src/plan/lib/` | 業務邏輯 |
| `src/types/story-map.ts` | `frontend/src/plan/types/` | 型別 |
| `templates/story-map.yaml` | `templates/` (go:embed) | 範本 |
| `templates/problem-statement.md` | `templates/` (go:embed) | 範本 |
