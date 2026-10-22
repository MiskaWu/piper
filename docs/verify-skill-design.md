# 獨立驗證技能設計

> 日期：2026-04-17
> 狀態：Phase 1 設計完成，待 Design session 生命週期決定後開工
>
> **歷史記錄** (2026-04-22 重構後)：本文件保留當時設計脈絡。Skill 命名已調整：
> 文中的 `/verify`（跑測試 + fix loop）已改名為 `/submit`；文中的 `/done`
> （事後驗收）已改名為 `/verify`。實際實作請參考 `CLAUDE.md` 的 Task 狀態機
> 與 Skill 閘門章節。

## 背景

Dungeon 模組近期 5 個上線後才發現的 bug，其中 4 個可透過適當測試在開發階段攔截。分析後發現問題不在測試量不足，而在**信任邊界**。

### 兩種失敗模式

| | 偷懶 / 幻覺 | 作弊 |
|---|---|---|
| 行為 | 沒做完、跳邊界、幻想 API 存在 | 勾了驗收 checkbox 但沒真的滿足 |
| 特徵 | 非故意，能力不足或 context 不夠 | 目標導向——「完成 task」的壓力下走捷徑 |
| 測試能抓嗎 | 能——只要測試本身是對的 | 不能——因為打勾的人就是做事的人 |
| 解法 | 更好的測試流程 | 隔離「做事」和「驗收」的 agent |

**偷懶/幻覺**已有流程處理（測試、`/done` checklist）。**作弊**是目前流程的盲點，本技能的目標是關閉這個盲點。

### 核心洞察：為什麼單純「加獨立 agent」不夠

第一版想法是加個獨立驗證 agent 來檢查實作，但更深的問題是：

```
Task 驗收條件是誰寫的？→ Breakdown agent（沒有人 review）
誰實作？→ 實作 agent
誰驗收？→ 同樣是 agent
```

等於 **agent A 出題 → agent B 答題 → agent B 自己打勾**。題目沒有人審過。

單純加個 agent C 驗證，仍是「agent 驗 agent」——如果驗收條件本身很模糊或很弱，C 也只能照著弱標準過關。

**真正的解法**：把驗收條件從「agent 自由發揮的自然語言」轉為**機器可驗證的契約**，讓驗證從「agent 判斷」變成「確定性斷言」。

## 三層驗收模型

釐清每層驗收的歸屬與表達方式：

| 層級 | 內容 | 歸屬 | 表達形式 |
|---|---|---|---|
| **L1 — Story 驗收** | 使用者可觀察的行為（「可以看到道具列表」） | Plan agent 推導 + 人 review | 自然語言 |
| **L2 — Task 驗收切分** | L1 按 targetProject 分給對應 task（UI 行為 → 前端 task，API 合約 → 後端 task） | Plan 或 Breakdown（取決於選項） | 自然語言 |
| **L3 — Task 驗證指令** | 每條驗收可執行的驗證命令（curl、test case） | Breakdown | 指令 + 預期輸出 |

**關鍵**：Story 不講 API（只講使用者行為），所以後端 task 的驗收不能直接從 L1 切出來。後端 task 的驗收目標是**契約**，契約就是跨專案共識的機器可驗證規格。

## 契約（Contract）

### 契約解決的問題

- **前端/後端可並行開發**：不用等對方先做完
- **驗收不靠 agent 判斷**：契約測試由 framework 產生斷言，驗證變確定性
- **跨專案共識有單一真實來源**：多個專案（Go backend、TS frontend）從同一份 YAML 契約產生各自格式
- **防作弊**：實作 agent 看到的是「必須通過這組契約測試」，不是「請自己判斷有沒有做到」

### 契約放在哪裡

放在 Epic Vault 的 story map 同層目錄：

```
.vault/epics/{epic-name}/
├── story-map.yaml
├── problem-statement.md
├── adr/
├── spikes/
└── contracts/           ← 新增
    ├── {contract-id}.yaml
    └── ...
```

### 契約 YAML schema（Phase 1 最小版本）

```yaml
id: api-session-list
version: 1.0
status: draft | approved | deprecated
owners: [story-id-list]          # 哪些 story 依賴這份契約
type: http-api | message | ui-props | data-schema

# 對 HTTP API 的情況
http:
  method: GET
  path: /api/sessions
  request:
    query: { ... }
    body: { ... }
  response:
    success:
      status: 200
      body: { status: string, items: [...] }
    errors:
      - { status: 400, when: "缺少必要欄位" }
      - { status: 500, when: "內部錯誤" }

# 產物：後端生成 Go handler 簽名 / 測試骨架，前端生成 TS type
generates:
  - project: piper-backend
    format: go-handler-test
    path: internal/handlers/session_test.go
  - project: piper-ui
    format: ts-types
    path: ui/src/types/api.ts

changelog:
  - version: 1.0
    date: 2026-04-17
    type: initial
    summary: 初始契約
```

### 契約的演化機制

#### 自動分類 Breaking / Additive

當契約 YAML 被修改時，掃描 diff 自動分類：

| 變更類型 | 分類 | 處理 |
|---|---|---|
| 新增 optional field | Additive | version bump patch（1.0 → 1.1） |
| 新增 endpoint | Additive | version bump minor |
| 刪除欄位 / 改名 / 改型別 | Breaking | version bump major（1.x → 2.0），需標記 migration plan |
| 改 path / method | Breaking | 同上 |

#### Changelog 與 feedback 迴圈

契約修改必須寫 changelog 條目，並填「觸發原因」：

```yaml
changelog:
  - version: 1.1
    date: 2026-05-01
    type: additive
    summary: 新增 createdAt 欄位到 response
    reason: verify-retro-feedback    # 來自 retro 指標
    retro_item: "UI 需要排序但契約沒有時間欄位"
```

**feedback 來源**：
- Verify 失敗時，若發現契約有缺陷（條件模糊、欄位不足）→ 記到 retro 指標
- Retro 討論後若決定修契約 → 在 changelog 註記 `reason: verify-retro-feedback`
- 這樣未來回看可以統計：有多少契約變更是因為原始設計不足

## 決策總結

### A. 驗收條件擁有權（選項 A/B/C）→ **選 A**

**選 A：Breakdown 後加人工 review 閘門**（Phase 1 採用）

- Breakdown 產出 L3 驗證指令後，人看一眼確認品質
- 人確認 OK 後才開始實作
- 理由：Phase 1 以穩為主，人的判斷暫時無法被 agent 完全取代
- **Phase 2** 考慮：人 review 可否透過統計 retro 指標自動化

### B. 契約測試產出方式 → **B2 框架生成**

- 契約 YAML → script 自動產生測試骨架（Go httptest / TS jest）
- 產生的測試檔案實作 agent **不能修改**（diff gate 擋）
- 理由：避免「agent 寫測試 agent 測 agent」的無限迴圈

### C. Checkbox 機制 → **C1 強綁定 + C2 隨機抽驗**

- **C1**：打勾必須附「執行證據」（測試 log 路徑、command output hash）
- **C2**：`/done` 隨機抽 1-2 條驗收重跑，不通過視為作弊

### D. 防作弊路徑 → **全採用**

| 路徑 | 怎麼發生 | 緩解方式 |
|---|---|---|
| 不跑就打勾 | 偷懶 | 打勾必須附執行證據（command output） |
| 跑了紅燈仍打勾 | 判斷模糊條件時 | 驗收條件必須帶明確判定標準 |
| 捏造執行結果 | 技術上可行 | `/done` 抽驗（隨機重跑 1-2 條） |
| 弱化驗證方式 | 條件不精確時 | 驗證指令在 breakdown 階段固定，人 review 確認 |

### E. 驗證失敗處理 → **E1a / E2b / E3a / E4b**

- **E1a**：契約測試失敗 → 驗證 agent 判定紅燈，寫報告
- **E2b**：紅燈後啟動 fix subagent（獨立 context，只能改實作碼）
- **E3a**：Fix → 重跑 verify → 仍紅 → 再 fix，直到綠或達上限
- **E4b**：達上限後停止，回報給人，不自動 merge

## Verify 執行環境（A 的具體機制）

### 核心流程

```
Implementation agents 完成 → commit → 標記 ready-to-verify
                                 ↓
                    ┌────────────────────────────┐
                    │ Verify session 啟動        │
                    │ (git worktree 隔離)        │
                    │                            │
                    │ 1. pin commit hash        │
                    │ 2. 生成契約測試            │
                    │ 3. 跑測試                  │
                    │ 4. 紅燈 → fix subagent    │
                    │ 5. loop 到綠或上限        │
                    │ 6. diff gate 檢查         │
                    └────────────────────────────┘
                                 ↓
                    綠燈 → rebase 回 main → checkbox 打勾
                    紅燈 → 回報人 → 保留 worktree 供 debug
```

### A 的子決策

#### α 粒度：commit lock 鎖「task 範圍內的檔案」

- 一個 task 有多個 sub-task 檔案，整個 task 一起鎖
- Phase 1 採用，未來若 retro 顯示「鎖太粗」再調整

#### iii 鎖的實作：state machine + commit hash pinning

**狀態機**（複用現有 status 欄位）：

```
todo → in-progress → ready-to-verify → (verifying) → done
                            ↑                ↓
                            └── 紅燈 ─────────┘ (fix subagent 重新改)
```

| 狀態 | 誰能改什麼 |
|---|---|
| `todo` / `in-progress` | 實作 agent 可改實作 |
| `ready-to-verify` | 實作 agent 不可改，等 verify 接手 |
| `verifying`（verify session 中）| 只有 verify / fix subagent 可改產品碼（不可改測試） |
| `done` | 鎖死，任何修改要開新 task |

**commit hash pinning**：verify 啟動時記錄當下 HEAD hash，整個 loop 對著那個 hash 跑（存在 worktree），避免實作 agent 同時 push 干擾。

**狀態機的作用**：讓 `/done`、verify skill、breakdown 這些技能都能用同一份狀態判斷「現在能不能動這個 task」，而不是每個技能各自用 ad-hoc 的條件檢查。未來 retro 若發現作弊新路徑（比如 verify 中有人偷偷改 status），狀態機能統一加強化規則。

#### γ3 跨 task 整合：Phase 1 不做

- 每個 task 契約各自通過，但整合後可能出 bug
- Phase 1 不處理，由 `/done` 跑全量測試兜底
- **Retro 指標追蹤**：「契約各 task 都通過但整合後出 bug 的次數」由我（assistant）每次 verify 後檢查標記
- 未來若此指標偏高 → retro 討論是否加 integration verify wave

### Loop 終止條件

研究過 Claude agent 機制後決定：

1. **主要信號**：Fix subagent fingerprint（錯誤訊息 hash）**連續重複 3 次** → 判定無進展，停止
2. **退化信號**：每次 fix 後通過的測試數 **回退** → 立即停止（避免越修越糟）
3. **硬上限**：N=10（對齊 Ralph Loop 慣例）

## 術語表（給未來實作時參考）

### git worktree

- **是什麼**：Git 支援同一個 repo 在多個目錄各自 checkout 不同分支
- **在 verify 裡的用途**：Verify session 在獨立 worktree 操作，實作 agent 的 main 分支同時可繼續工作，互不干擾
- **指令**：`git worktree add ../verify-xyz branch-name`
- **清理**：`git worktree remove ../verify-xyz`

### commit lock

- **是什麼**：「某個狀態下不准改這些檔案」的機制
- **我們的實作**：用 state machine 判斷 + commit hash pinning 凍結驗證目標
- **不是**：真的 git-level lock（git 沒這東西）
- **為什麼需要**：避免 verify 在跑時實作 agent 又 push 新 commit，導致「測的是舊版、合的是新版」

### diff gate

- **是什麼**：Agent session 結束後，檢查 git diff，如果動到「不該動的檔案」就擋下合併
- **在 verify 裡**：Fix subagent 結束後檢查有沒有碰測試檔，碰了就視為作弊
- **實作方式**：`git diff --name-only main..HEAD | grep test` → 有命中就 abort

### fix subagent

- **是什麼**：Verify agent 判定紅燈後，啟動的獨立修復 agent
- **為什麼分開**：Verify 自己修會有利益衝突（「我說紅燈，但我自己修就改判綠」）
- **Context 隔離**：Fix subagent 只看紅燈報告 + 產品碼，看不到測試實作細節
- **權限**：只能改產品碼（透過 diff gate 強制）

### 契約測試 framework 生成

- **是什麼**：給一份 YAML 契約 → script 自動產生測試檔（Go httptest 骨架 / TS jest case）
- **為什麼是「生成」不是「手寫」**：手寫會引入 agent 判斷；生成出來的測試是契約的 1:1 翻譯，沒有解釋空間
- **Phase 1 實作**：最小可行版本，支援 HTTP API 契約 → Go `*_test.go` + TS `*.test.ts`

### state machine

- **是什麼**：task status 欄位的合法轉移圖
- **作用**：所有動 task 的技能（breakdown/verify/done）共用同一份狀態判斷邏輯
- **目前**：status 值是自由字串（只校驗值在列表中），Phase 1 加上 transition 校驗

### commit hash pinning

- **是什麼**：記錄 verify 啟動時的 HEAD hash，整個 verify loop 對著那個 hash 跑
- **目的**：避免驗證目標在 loop 期間被移動

### PIPER_HOST 機制（既有）

- **是什麼**：Piper 自動偵測 host IP 注入 DevContainer 的 env var
- **位置**：`hosts.go` 的 `resolvePiperHost`、`internal/tmux/tmux.go:67` 的 `SetPiperHost`
- **在 verify 裡的用途**：Verify session 在獨立 worktree / container 跑時，仍能透過 `$PIPER_HOST` 呼叫 Piper API（讀契約、更新 task status）
- **Phase 1 假設**：只支援 DevContainer；非 DevContainer 的 fallback 放 Phase 2

## Phase 1 實作 checklist

### 資料層
- [ ] 在 `internal/plan/storymap.go` Task struct 加 `Acceptance []AcceptanceItem` 欄位
  - `AcceptanceItem`: `{ text, verifyCommand, expectedOutput, evidence }`
- [ ] Task status 加 transition 校驗（state machine）
- [ ] Vault 目錄結構支援 `contracts/*.yaml`

### 契約層
- [ ] 契約 YAML schema 定義（Phase 1 只支援 http-api type）
- [ ] 契約 → Go httptest 骨架生成 script
- [ ] 契約 → TS type 生成 script
- [ ] Changelog 自動分類 breaking/additive

### API 層
- [ ] `GET /api/epics/{name}/contracts` 列出契約
- [ ] `GET/PUT /api/epics/{name}/contracts/{id}` CRUD
- [ ] `POST /api/epics/{name}/contracts/{id}/generate` 觸發生成

### Verify skill（source of truth：`internal/plan/references/verify-skill.md`，dispatch 部署到 target 的 `.claude/skills/verify/SKILL.md`）
- [ ] 流程定義
- [ ] Worktree 建立 / 清理 helper
- [ ] Fix subagent 定義（prompt + diff gate）
- [ ] Loop 終止邏輯（fingerprint 比對 + regression 偵測 + N=10）
- [ ] Verify session 生命週期（等討論）

### Breakdown 調整
- [ ] 補充 L3 驗證指令（原本只補驗收文字，現在要補可執行指令）
- [ ] 產出後暫停，等人 review（A 的閘門）

### Done skill 加強
- [ ] C1 強綁定：checkbox 勾選必帶 evidence 欄位
- [ ] C2 抽驗：隨機重跑 1-2 條驗收

## Retro 指標（觀察與迭代）

### 契約層
- 契約修改次數（breaking / additive 分別統計）
- 契約修改中「reason: verify-retro-feedback」的比例
- 平均一個 epic 有幾份契約

### Verify 層
- Verify 通過率（第一次就綠 / 紅燈→fix→綠 / 達上限失敗）
- Fix subagent loop 平均次數 / 觸發硬上限的比例
- Diff gate 擋下的次數（代表 fix subagent 想作弊）
- `/done` 抽驗失敗次數（代表前面的 checkbox 是假的）

### 跨 task 整合（手動標記）
- 契約各 task 都通過但整合後出 bug 的次數（我每次 verify 後檢查 + 在 retro 報告中列出）
- 若此指標 > 0 → retro 討論是否加 integration verify wave

### 作弊路徑追蹤
- 發現新的作弊模式 → 加入 retro 報告的「作弊觀察」欄位
- 每次 retro 檢視是否需要新增防線（新增路徑到 D 表）

## 契約產出方式（Phase 1）

**Phase 1：不做專屬 session，由 assistant 在對話中產出**

- Plan session 完成後，使用者開一般對話要求 assistant 寫契約 YAML
- Assistant 讀 story map + 探索 codebase（現有 endpoint、命名慣例）→ 產出契約
- 人 review 契約後放進 `.vault/epics/{name}/contracts/`

**為什麼 Phase 1 不做 session**
- 契約本身的價值還沒被驗證，先把 session 基建做下去風險太高
- 實際寫過幾份後才知道「plan agent 夠不夠」還是「需要獨立 design session」
- 這份決定會在 retro 被觀察：契約品質低 → 考慮升級為專屬 session

## Phase 2：契約由 assistant 產出（決定：先 A 後 C）

Phase 1 在 commit `a6607db` 完成，契約目前手寫。Phase 2 讓 assistant 接手。

### 2A. `/design` skill（先做，獨立 slash command）

**觸發**：`/design {epic} story-xxx` 或 `/design {epic} task-xxx`（**host side** 跑，plan 完成後 → dispatch 之前）

**流程**：
1. 讀 vault story map 找目標 story/task 的上下文（A/A/N、驗收條件、target project）
2. 讀 1–2 個同 epic 內既有契約當風格錨點（**不**探 target project codebase — 契約是邏輯共識，不是現有 code 的鏡子）
3. 產出契約 YAML（Phase 1 只支援 http-api type）
4. A-gate：使用者確認 → `PUT /api/epics/{epic}/contracts/{id}` 寫入 vault
5. `POST /api/epics/{epic}/contracts/{id}/generate?dryRun=true` 讓使用者預覽會生成什麼檔
6. 提示「可跑 `/verify task-xxx`」

**檔案**：
- `internal/epiccontainer/references/design-skill.md`（source of truth，go:embed 進 binary）
- 由 `epiccontainer.PrepareWorkspace` / `PrepareNativeWorkspace` 在 `SessionPlan` 時寫入 plan workspace 的 `.claude/skills/design/SKILL.md`
- 不部署到 host 全局 `~/.claude/skills/`、不 dispatch 到 target project — 只在 plan session 內可見

**不做**：
- 自動 generate（只 dry-run 預覽，真正生成留給 `/verify`）
- 自動寫 vault（A-gate 強制）
- 多 task 批量（一次一個，避免 context 爆）

**驗收**：拿 piper 自己的一個新 API task 跑 `/design` → 產出契約能通過 `PUT` 驗證且命名風格與既有 route 一致。

### 2C. Design session（後做，Piper 側整合）

當 `/design` 用起來品質穩定後，升級成容器化 session，與 plan/dispatch/harvest 一致：
- `POST /api/epics/{name}/design-session` 建/取得 session（冪等）
- 容器內自動推進 `/design` 流程，完成後自動結束
- Sidebar 顯示 design session 狀態

**開工條件**：`/design` skill 跑過 ≥3 個真實 epic + retro 指標顯示契約品質穩定（能一次通過 `/verify` 的比例 > 60%）。

### 2B（不採用）. 擴充 `/breakdown` 同時產契約

Breakdown context 已重，再加契約職責會爆。留作備案。

## Phase 3+ 候選路線圖

依 Phase 2 retro 回饋調整順序：

- **自動觸發 `/verify`**：/done 之前自動跑 verify wave，而非手動
- **跨 task 整合 verify**：release 級別契約 compatibility check（原設計 §γ3 Phase 1 跳過）
- **多契約類型 generator**：message queue / ui-props / data-schema
- **ClassifyChange 調優**：依 retro 誤判率調 breaking/additive 規則
- **防作弊路徑擴充**：retro 觀察到新繞過方式就補防線（diff-gate 只是第一道）
- **非 DevContainer fallback**：Phase 1 假設在 DevContainer，需要支援 host 模式

## 下一步

Phase 1 於 commit `a6607db`、Phase 2A 於 commit `2826be6` 完成 — 先以 `/design` 跑真實 epic 累積 retro 資料（≥3 個 + 一次通過 `/verify` 比例 > 60%）再開 Phase 2C；Phase 3+ 候選待 retro 回饋排序。
