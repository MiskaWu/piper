---
description: "拆分 Task Brief 為解耦的 Sub-tasks — 標注檔案範圍、依賴關係、並行 Wave"
argument-hint: "[task-id] 或不帶參數自動掃描"
---

# Task Sub-task 拆分器

> **名詞定義**：本文件中的「Sub-task」指 Breakdown 拆分出的檔案級執行單位；「Task」指 Story Map 中按專案切分的工作單位。

分析 Task Brief，拆分為檔案級解耦的 Sub-tasks，按依賴關係分組為可並行的 Wave。
目標：讓每個 Sub-task scope 夠小、context 夠明確，Claude Code 一次只專注一件事。

## 參數

- `$ARGUMENTS`：一或多個 task ID（如 `task-a1`），或不帶參數掃描所有待拆分的 briefs

## 啟動步驟

1. 掃描 `.plan/briefs/` 目錄下的 `task-*.md`（排除 `done/`）
2. 如果有提供 task ID → 只處理指定的
3. 如果沒有 → 列出所有尚未包含「Sub-task 拆分」章節的 briefs 供選擇

## 分析步驟

對每個 brief，依序執行：

### 1. 理解 Task 範圍

- 讀取 brief 的「做什麼」和「驗收條件」
- 讀取 brief 的「Story 驗收條件」章節，理解這個 task 在 story 整體中的定位
- 讀取「相關決策」和「技術探針結論」
- 確認 target project 的技術棧（讀 CLAUDE.md 或 go.mod / package.json）

### 1.5. 反查本 task 的契約

呼叫 Piper API 取得所有 owners 含本 task-id 的契約：

```bash
curl -sfL "http://$PIPER_HOST/api/epics/{epic}/contracts?owner={task-id}"
```

- 非空 JSON 陣列 → 每個 ID 都是本 task 該實作並驗證的契約。記下 ID 清單，後續步驟 2 會直接用來填 L3 驗收指令，並在 brief 寫入「契約」章節
- 空陣列 → 可能原因：
  1. `/design` 尚未跑 → **停下來告知使用者補 `/design {epic} {task-id}`**，不繼續拆分
  2. 契約 owners 寫成 story-id（舊流程遺留）→ 改讀全量 `GET .../contracts`，列出候選請使用者挑或確認無契約
  3. 本 task 不涉及跨模組 API（純 UI、純重構）→ 確認 Wave 1 會標注「跳過契約」

### 1.6. Message 契約 Binding Inline 補充

對步驟 1.5 反查到的每個契約 ID，呼叫：

```bash
curl -sfL "http://$PIPER_HOST/api/epics/{epic}/contracts/{id}" | grep '^type:' | awk '{print $2}'
```

取 `type`：
- `http-api` → 略過，method+path 本身就是完整呼叫識別，不需 binding
- `message` → 進下一步驗 binding（結構化 message 契約需靠 manifest + binding 才能定位到具體 transport）
- `ui-props` / `data-schema` → 略過，非 interaction 契約

對每個 message 契約，檢查本專案是否已補 binding：

```bash
PROJECT=$(basename $PWD)
curl -sfL -o /dev/null -w '%{http_code}' "http://$PIPER_HOST/api/bindings/$PROJECT/{contract-id}"
```

**`200` — 已綁定**：讀 binding 的 `contract_version` 比對契約當前 `version`：
- 相同 → OK
- major 差一代（如契約 2.0、binding 1.x）→ 警告「binding 可能過期，/submit 會擋」，列入後面的補綁清單

**`404` — 缺 binding**：不停下，**inline 在本流程中補完**，對每個缺的 message 契約依序：

1. **讀契約語意**：
   ```bash
   curl -sfL "http://$PIPER_HOST/api/epics/{epic}/contracts/{contract-id}"
   ```
   取 `message.resource` / `message.action` / `message.request` / `message.response` / `message.errors`（欄位語意，不是 proto message name）。

2. **搜本地 .proto 找對應 service.method**：
   ```bash
   PROTO_ROOT=$(curl -sfL "http://$PIPER_HOST/api/projects/$PROJECT/manifest" | grep 'proto_root:' | awk '{print $2}')
   grep -rn "service " "$PROTO_ROOT" --include='*.proto'
   ```
   依契約 resource / action 語意比對候選。找不到 → 停下要求使用者先補 .proto 檔再重跑 /breakdown。

3. **讀取該 service 的 RPC 定義**，抓對應 method 的 Request / Response message 名。

4. **A-gate 確認**：貼出起草的 binding YAML：
   ```yaml
   contract_id: {contract-id}
   contract_version: "{契約當前 version}"
   bound_at: {today-YYYY-MM-DD}
   proto:
     service: {偵測/選定的 service}
     method: {選定的 method}
     file: {相對 repo root 的 .proto 路徑}
     request: {Request message 名}
     response: {Response message 名}
   ```
   問使用者：「這樣綁對嗎？（yes / 修 X=Y / 跳過 → 請事後跑 `/bind {id}` 自己補）」

5. **yes** → PUT binding：
   ```bash
   curl -sfL -X PUT "http://$PIPER_HOST/api/bindings/$PROJECT/{contract-id}" \
     -H 'Content-Type: text/yaml' \
     --data-binary @/tmp/piper-bind-{contract-id}.yaml
   ```
   2xx → 進下一個契約；400 → 顯示錯誤回 A-gate 修。

全部 message 契約補齊 binding 後繼續步驟 2。

> 何時 inline 補不處理 vs 交給獨立 `/bind`：
> - 本 task 關聯的 message 契約數 ≤ 2 且 service.method 明確 → inline 補完最順
> - 需要全量 sweep（整個 project 多個 task 都缺 binding）→ 建議使用者中斷，跑 `/bind`（不帶參數）一次補齊再重跑 /breakdown
> - 契約 major bump 後的重綁（binding 已存在但 version 落後）→ 同上，交給 `/bind` 專門處理

若在 A-gate 任何一步使用者選「跳過」→ 記錄這個契約未綁定，繼續 breakdown 其他步驟，**在最後的 A-gate 裡提醒使用者跑 /submit 前必須補完**。

### 2. 識別變動檔案

根據 task 需求，列出所有需要新增或修改的檔案。方法：

- 從驗收條件反推需要哪些功能
- 對照 Story 驗收條件，確認 Task 驗收條件是否涵蓋了本 task 應負責的 story 驗收子集；若有遺漏，**先補充 Task 驗收條件再繼續拆分**
- 補充時將模糊驗收改寫為可測試形式：
  - BAD: `API 回傳正確的資料`
  - GOOD: `GET /api/sessions 回傳 200，body 含 status 欄位`
- **L3 驗收指令（強制）**：每一條 Task 驗收條件都必須附帶**可機器執行**的驗證指令，`/submit` 與 `/verify` 會直接跑。可接受形式：
  - 步驟 1.5 反查到的每個契約 → 對應 `POST /api/epics/{epic}/contracts/{id}/generate` 生成的測試檔（**優先**，能對上就不要自己寫 curl/jq）
  - `curl -sfL http://$PIPER_HOST/api/... | jq -e ...`
  - `go test -run TestX ./pkg/...`
  - `npm test -- --testNamePattern=X`
  - 其他能自動化且 exit code 能判斷成敗的指令
- 若任何一條驗收無法給出 L3 指令，**改寫該條直到可以**；改不動的拆成兩條（一條可驗、一條留給 manual review）
- 驗收條件寫入 brief 時，格式：
  ```markdown
  - [ ] GET /api/items 回傳 200，body 含 items 陣列
    **verify:** `curl -sfL "http://$PIPER_HOST/api/items" | jq -e '.items | length >= 0'`
  ```
- 搜尋 codebase 找到相關的現有檔案（用 Grep/Glob）
- 區分：新增檔案 vs 修改現有檔案

### 3. 拆分 Sub-tasks

每個 Sub-task 遵循以下原則：

**解耦原則（最重要）**
- 同一個 Wave 內的 sub-tasks **不得碰相同檔案**
- 如果兩個 sub-task 都需要改 `routes.go`，它們必須在不同 wave
- 純新增檔案的 sub-task 天然解耦，優先放入早期 wave

**粒度原則**
- 每個 sub-task 改 1-3 個檔案（含測試檔）
- 一個 sub-task 應該在單次 Claude Code 對話中能完成
- 測試檔和實作檔放同一個 sub-task（TDD 循環不該跨 sub-task）

**依賴原則**
- 資料結構/型別定義 → 先做（被其他 sub-task 引用）
- 核心邏輯 → 中間（依賴型別，被 handler 引用）
- 串接層（routes、handler）→ 後做（依賴核心邏輯）
- 前端 → 最後（依賴 API 完成）

### 4. 分組為 Waves

- Wave 內的所有 sub-tasks 可同時並行執行
- Wave 之間有順序依賴：Wave N+1 的 sub-tasks 依賴 Wave N 的某些 sub-tasks
- 每個 sub-task 明確標注依賴哪些前置 sub-tasks

### 強制 Wave 結構

無論 task 大小，拆分結果必須包含以下固定 wave：

**Wave 1 — 契約先行（必要）**
- 定義共用的 types、interface、API request/response struct
- 這是所有後續 sub-task 的共識基礎，確保各 sub-task 對著同一份契約實作
- 如果 task 涉及 API，包含 route path + method + request/response 型別
- 如果 task 純前端，包含 props interface + shared types

> 如果 task 不涉及跨模組整合（如純 UI 調整、單一模組重構），可在拆分時標注 `Wave 1 — 跳過（單模組不需契約）` 並說明理由。

**最後一個實作 Wave — 含整合驗證**
- 最後一個實作 wave 的 agent 完成自己的 sub-task 後，額外負責：
  1. 跑完整測試套件（不只單元測試，包含 httptest / integration 級別）
  2. 逐條核對 **Task 級別**的驗收條件（brief 的「驗收條件」章節），並對照 **Story 驗收條件**確認本 task 負責的部分已滿足
  3. 如果有遺漏，在此 sub-task 補齊
- **不另開獨立的整合 wave** — `/verify` 負責最終驗收
- 如果有多個 wave 各自改了會互相影響的東西，最後一個 wave 的 agent 需要在自己的 sub-task 完成後跑全量測試確認整合

- **標準化驗證 checklist**（最後實作 wave 的驗收條件必須包含以下適用項）：
  - [ ] **模組初始化 smoke test**：新模組 / 服務能正常啟動，不因缺少依賴而 panic
  - [ ] **DI 接線完整**：所有新增的 struct field / constructor 參數在 caller 端已傳入真實實例
  - [ ] **Handler 回傳欄位完整性**：API 回傳值包含呼叫端所需的所有欄位（對照前端 / 呼叫端的使用方式）
  - [ ] **單次寫入原則**：同一 request 內不對同一筆 record 做多次 persist
  - [ ] **TODO / placeholder 清零**：grep `TODO`、`FIXME`、`PLACEHOLDER`，確認本 task 新增的程式碼中無殘留
  - [ ] **建置通過**：`go build ./...` 或 `npm run build`（視技術棧）零錯誤

### 5. 為每個 Sub-task 撰寫 Context

這是關鍵 — 每個 sub-task 要帶足夠資訊，讓 Claude Code **不需要讀整個 brief** 就能開工：

- 要做什麼（具體到函式/struct 層級）
- 要改哪些檔案
- 參考哪些現有程式碼（給出路徑）
- 這個 sub-task 的驗收條件（從 task 驗收條件拆出來的子集）
- 如果驗收條件可用確定性方式驗證（script、test command、curl、lint check），標注驗證指令

**邊界驗收（依 sub-task 類型自動附加）**

根據 sub-task 的操作性質，除了從 task 驗收條件拆出的子集，還必須附加對應的邊界驗收項：

| Sub-task 類型 | 自動附加的邊界驗收 |
|---|---|
| **狀態累積操作**（連續新增、批次更新、計數器遞增） | ① 連續操作 N 次結果不衝突（ID / 座標 / 索引不重疊） ② 邊界值（0 個、上限個） ③ 併發安全（如適用） |
| **新增依賴注入**（新 struct field、新 constructor 參數） | ① 所有 caller 已更新 ② mock / stub 已同步更新 ③ `nil` 依賴不導致 panic |
| **Handler / API 層變更** | ① 回傳值反映實際結果（非回傳原始輸入） ② 缺少必要欄位 → 4xx ③ 依賴服務失敗 → 5xx ④ 回傳欄位足以讓呼叫端完成下一步 |
| **資料結構變更**（schema 遷移、struct 欄位增刪） | ① 既有資料向後相容 ② 零值 / null 欄位有明確處理 ③ 序列化 / 反序列化一致 |

## 輸出格式

將以下內容**追加到 brief 檔案末尾**：

```markdown
## 契約

> 由 `/breakdown` 從 `GET /api/epics/{epic}/contracts?owner={task-id}` 反查產生；`/submit` 會讀這節拿契約 ID 生成測試。

- {contract-id-1}
- {contract-id-2}

（若步驟 1.5 回空陣列，填「無 — 本 task 不涉及跨模組契約」並說明理由）

## Sub-task 拆分

> 拆分時間：{日期}
> 共 {N} 個 sub-tasks，{M} 個 waves

### Wave 1 — 契約先行

#### Sub-task 1: 定義共用型別與介面
- **檔案**: `path/to/types.go`
- **做什麼**: {定義 request/response struct、interface、共用常數}
- **參考**: `path/to/similar_types.go` 的風格
- **驗收**: 型別定義完成，`go vet` 通過

### Wave 2（可並行，依賴 Wave 1）

#### Sub-task 2: {簡短標題}
- **檔案**: `path/to/file.go`, `path/to/file_test.go`
- **依賴**: Sub-task 1
- **做什麼**: {具體描述，到函式/struct 層級}
- **參考**: {參考路徑}
- **驗收**: {驗收條件}

（同 wave 可有多個 sub-task，格式同上，確保檔案不重疊）

### Wave N（最後實作 wave）

#### Sub-task N: {實作內容} + 整合驗證
- **檔案**: {此 sub-task 需要的檔案}
- **依賴**: {前置 sub-tasks}
- **做什麼**: {此 sub-task 的實作} + 串接所有前面 wave 的產出；跑完整測試套件
- **驗收**: 本 sub-task 驗收 + 所有 sub-task 串接正確 + 完整測試通過 + Task 驗收條件全數滿足 + Story 驗收條件（本 task 範圍）已覆蓋 → 先 `/submit` 再 `/verify`
```

## 完整性檢查

拆分完成後自動執行以下檢查，有問題必須調整後才能產出：

### 結構衝突

1. **同 wave 檔案衝突** → 必須拆到不同 wave
2. **單 sub-task 超過 3 個檔案** → 建議再拆

### TODO / Placeholder 覆蓋

掃描所有 sub-task 的「做什麼」描述，找出含有以下模式的產出：
- 明確標記 `TODO`、`FIXME`、`PLACEHOLDER` 的程式碼
- 空字串 / 零值佔位（如 `type := ""`、`id := 0`、`return nil`）
- 「先回傳固定值，後續替換」的描述

每個 placeholder **必須有對應的 sub-task 負責填入真實值**。如果找到無主的 placeholder：
→ 在同 wave 或後續 wave 補上收尾 sub-task，或合併到現有 sub-task 的做什麼與驗收條件中

### 依賴注入接線

如果任何 sub-task 涉及以下操作：
- 新增 struct field（尤其是 interface 型別的依賴）
- 新增 constructor 參數
- 修改 `New*()` 函式簽名

→ 必須有對應的 sub-task（通常在整合 wave）負責：
- 在 caller 端傳入新依賴
- 更新組裝點（如 `main.go`、`wire.go`、DI 容器）
- 更新既有測試中的 constructor 呼叫

### 驗收條件覆蓋

- Task 驗收條件的每一條都必須映射到至少一個 sub-task 的驗收
- 不得有驗收條件沒有對應的 sub-task

### 語意自檢（結構檢查通過後 Claude 自行驗證）

7. **獨立可理解**：逐一檢查每個 sub-task 的「做什麼」描述 — 不讀其他 sub-task 也能理解這個 sub-task 要做什麼。如果描述中引用了「同上」、「跟 sub-task N 一樣」等相對描述 → 改寫為自足描述。

8. **驗收全覆蓋**：即上方「驗收條件覆蓋」檢查，確認 Task 驗收條件的每一條都映射到至少一個 sub-task。

9. **DI 接線完整**：即上方「依賴注入接線」檢查，確認新增的 interface / struct field 有對應的 wiring sub-task。

## 自動放行流程

拆分完成後依序執行完整性檢查 + 語意自檢：

1. **結構檢查**（同 wave 檔案衝突、單 sub-task 檔案數、TODO 覆蓋、DI 接線、驗收覆蓋）
2. **語意自檢**（獨立可理解、驗收全覆蓋、DI 接線完整）
3. **L3 驗收指令檢查**：掃描 Task 驗收條件章節，確保每一條 `- [ ]` 下都有 `**verify:**` 行；否則提示補足
4. **契約章節檢查**：brief 有「契約」章節，且內容與步驟 1.5 的反查結果一致（非空時每個契約 ID 至少被某條 `verify:` 引用一次）

- 全部通過 → **暫停並提示使用者 review**（A 閘門）：
  - 輸出類似「拆分完成，請 review Task 驗收條件 + L3 verify 指令，確認無誤後執行實作，完成後跑 `/submit`」
  - 不自動進入實作階段，等待使用者明確指示
- 有未通過項 → 自動調整拆分並重新檢查（最多重試 2 次）
- 重試仍未通過 → 停止，列出所有未通過項目，等待使用者確認

## 注意事項

- 使用繁體中文撰寫
- 拆分結果追加到 brief 檔案，不另建檔案
- 如果 brief 已有「Sub-task 拆分」章節，詢問是否覆蓋
- 拆分時要實際搜尋 codebase，不能憑空猜測檔案路徑
- 參考路徑必須是真實存在的檔案
