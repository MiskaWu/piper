---
description: "Task 最終驗收 — C1 evidence + C2 抽驗 + Story 交叉判定 + 缺口分流，通過才推 done 並搬 brief"
argument-hint: "[task-id...] 或不帶參數自動掃描"
---

# Task 最終驗收（Verify）

獨立驗證者角色。驗收 task 是否滿足技術條件與 task 級驗收標準，全部通過才推進到 `done` 並將 brief 搬到 `done/`；有缺口則**按類型分流**退回合適的狀態（brief / evidence 缺口 → `in-review`；產品碼 / C2 抽驗失敗 → `in-progress`）。

> **命名邏輯**：`/submit` 是實作者自測（推到 `in-review`），`/verify` 是獨立驗證者做最終把關（推到 `done`）。兩個動作故意在不同 session 跑，避免自做自驗。

> **名詞定義**：本文件中的「Task」指 Story Map 中按專案切分的工作單位。本流程在驗收 Task 時同時交叉檢查 Story 驗收條件，確保 task 的產出涵蓋其應負責的 story 驗收子集。

## 參數

- `$ARGUMENTS`：一或多個 task ID（如 `task-a1 task-b1`），或不帶參數自動掃描

## 前置條件

- Task 目前狀態必須是 `in-review`（由 `/submit` 推進而來）
- Task brief 已勾驗收條件 + 附 evidence

## 執行步驟

### 0. 入口 Guard：確認 task status == in-review

對每個要驗收的 task，先讀取當前 story-map 確認 status：

- `todo` / `in-progress` → 停止，告知使用者「請先跑 `/submit {task-id}` 讓測試綠燈後再跑 `/verify`」
- `done` → 跳過（已完成）
- `in-review` → 繼續

### 1. 掃描待驗收的 tasks

如果有提供 task ID → 只處理指定的 tasks。
如果沒有提供 → 掃描 `.plan/briefs/` 目錄下所有 `task-*.md`（排除 `done/` 子目錄），篩出 status == `in-review` 的。

### 2. Checkbox 硬閘門

執行 `bash .claude/scripts/check-briefs.sh [task-ids...]` 檢查所有 brief 的驗收條件 checkbox。
- exit 0 → 全部勾選，繼續下一步
- exit 1 → 有未勾選項目，**停止流程**，輸出 script 的錯誤清單，要求先勾選再重跑 `/verify`
- exit 2 → 找不到 brief，跳過

此步驟為硬性檢查，不可跳過或自行判定通過。

### 3. Breakdown 完整性檢查

> 本步驟**不是**主觀推演「功能如預期運作嗎」——實作正確性交由 `/submit` 契約測試與步驟 4 的 C1/C2 機械驗證。此處只做 **completeness check**：確認 breakdown 產出的 L3 驗收條件能涵蓋 brief「做什麼」章節描述的行為。

1. 讀取 brief 的「做什麼」章節，列出其中描述的使用者/系統行為（bullet list）
2. 對每條行為，檢查「驗收條件」章節是否有對應的 `- [x]` 項目
3. 標記結果：
   - ✅ 每條行為都有對應 L3 驗收條件覆蓋
   - ⚠️ 有行為未被任何驗收條件覆蓋 → 列入缺口清單，類型 `[acceptance_uncovered]`

這個步驟要抓的是：breakdown 漏寫驗收條件、或 scope creep 後沒回補驗收。不做也不能做的是：「agent 讀程式碼判斷實作對不對」——那是 `/submit` 的責任，重做一次只會引入主觀判斷。

### 4. 驗收條件核對

接著逐條技術驗收：

1. 讀取 brief 中的「驗收條件」章節
2. 逐條確認是否已滿足（檢查相關程式碼、測試是否存在且通過）
3. 標記結果：
   - ✅ 全部通過
   - ⚠️ 部分未通過（列出未通過項目）
   - ❌ 找不到 brief → 跳過

#### C1 — Evidence 強制要求

對每一條 `- [x]`，要求下方 5 行內有 `**evidence:**` 標記（log path / command output hash / `/submit` run id）。

- 沒附 evidence → **視為未勾選**，列入缺口清單（類型 `[missing_evidence]`），不進步驟 7
- check-briefs.sh 已做這個檢查；若 script exit 非零，直接採用 script 輸出作為缺口清單

#### C2 — 隨機抽驗

C1 通過後，從 `- [x]` 驗收條件中**隨機抽 1–2 條**（`rand() % n`），讀取該條下的 `**verify:**` 指令並重跑：

- exit 0 → 該條通過抽驗
- exit 非 0 → 視為該條驗收失敗，列入缺口清單（類型 `[sampling_failed]`），**不移動 brief**
- `**verify:**` 不存在 → 列入缺口清單（類型 `[missing_verify_cmd]`），要求先回 `/breakdown` 補指令

抽驗的目的是對抗「先自己勾選再說」的作弊；指令存在且可 exit 0 是對「測試確實綠燈過」的抽樣確認。**C2 在獨立 session 重跑，`/submit` 的 worktree 已清理，環境跟實作者分離 — 這是防自做自驗的關鍵防線**。

### 4.5. Story 驗收條件交叉檢查

Task 級驗收通過後，交叉檢查 Story 層級的驗收條件。這層無法用契約測試覆蓋（`/submit` 只驗本 task 的契約，不驗 story 責任劃分），只能靠判斷。為了**把隱性判斷顯性化**，必須逐條產出判定表，不可只報結論。

1. 讀取 brief 中的「Story 驗收條件」章節
2. **逐條**產出判定表：

```
| # | Story 驗收條件 | 本 task 負責？ | 判定依據 |
|---|----------------|---------------|----------|
| 1 | 登入失敗顯示錯誤訊息 | ✅ 是 | 本 task 實作登入 API，錯誤回傳是 API 端責任 |
| 2 | 密碼強度檢查 | ❌ 否 | 由 task-b2 負責前端驗證 |
| 3 | Session 過期自動登出 | ⚠️ 部分 | 本 task 應實作 token TTL，但 UI 跳轉由 task-c1 |
```

3. 對「✅ 是」或「⚠️ 部分」的條目，進一步確認本 task 的 L3 驗收條件（步驟 4）是否已涵蓋：
   - 已涵蓋 → ✅
   - 有遺漏 → 列入缺口清單，類型 `[acceptance_uncovered]`，說明缺什麼

此步驟為判斷性檢查（非硬閘門），但發現遺漏時應列入缺口清單一併處理。目的是防止 task 通過了自身驗收卻縮減了 story 承諾的範圍。判定表會留在 `/verify` 的輸出中，讓人或後續的 retro session 可以 review 責任劃分是否合理——而不是讓 agent 在黑箱裡自己決定「這條不是我的責任」。

### 5. 判定結果

**全部通過 → 進入步驟 6（推狀態）然後步驟 7（移動）**

**有缺口 → 按類型分流退回，不推 done：**

| 類型 | 本質 | 退回狀態 | 原因 |
|---|---|---|---|
| `[missing_evidence]` | brief 勾了但沒附 evidence | `in-review` | 只需補 brief，不用重跑 fix loop |
| `[missing_verify_cmd]` | brief 下沒寫 verify 指令 | `in-review` | 只需補 brief（回 `/breakdown` 補指令）|
| `[acceptance_uncovered]` | 驗收條件未覆蓋 breakdown/story 行為 | `in-review` | 只需補 brief + 補相關實作（視情況） |
| `[sampling_failed]` | C2 抽驗重跑測試失敗 | `in-progress` | 產品碼有問題，要重跑 `/submit` |
| `[missing_test]` | 缺少測試 | `in-progress` | 產品碼邏輯沒測試 |
| `[boundary_case]` | 邊界情況未處理 | `in-progress` | 產品碼未完整 |
| `[integration]` | 跨模組整合問題 | `in-progress` | 產品碼整合問題 |
| `[scope_violation]` | 超出範圍修改 | `in-progress` | 需要修正實作範圍 |
| `[naming_mismatch]` | 命名不一致 | `in-progress` | 產品碼要改 |
| `[uncategorized]` | 無法歸類 | `in-progress`（保守） | 人工判斷 |

把 task status 退回對應狀態：

```
curl -fsS -X PUT "http://$PIPER_HOST/api/epics/{epic}/story-map" ...
```

輸出缺口清單：

```
🔍 task-a1 — 登入 API｜驗收未通過

### Breakdown 完整性缺口
1. ⚠️ [acceptance_uncovered]「session 過期自動登出」
   **現狀**: brief「做什麼」有提到，驗收條件沒有對應項目
   **建議修復**: 回 /breakdown 補驗收 + verify 指令
   **退回狀態**: in-review（補完 brief 直接重跑 /verify）

### 驗收條件缺口
1. ❌ [missing_evidence]「回傳結構化錯誤碼」
   **現狀**: 已勾選但 5 行內無 **evidence:** 標記
   **建議修復**: 補上 submit run id 或 test log 路徑
   **退回狀態**: in-review
2. ❌ [sampling_failed]「POST /api/login 成功回傳 token」
   **現狀**: C2 抽驗重跑指令 exit 非 0
   **建議修復**: 重跑 /submit 找實際失敗原因
   **退回狀態**: in-progress（產品碼需修正）

### Story 驗收條件疑似遺漏
1. ⚠️ [acceptance_uncovered]「登入失敗時顯示明確錯誤訊息」
   **判定依據**: 本 task 實作登入 API，錯誤回傳是 API 端責任（見 4.5 判定表第 1 列）
   **現狀**: 目前只回傳 401，無 JSON body
   **建議修復**: 回傳 JSON body 含 error code + message
   **退回狀態**: in-progress

---
以上缺口修復完成後：
- 全部是 in-review 層（brief/evidence）→ 直接重跑 /verify
- 有 in-progress 層（產品碼）→ 請先跑 /submit，綠燈後再跑 /verify
```

缺口分流規則：一個 task 內若同時有「brief 缺口」和「產品碼缺口」，取**最嚴重者**（in-progress 優先於 in-review）作為退回狀態。

### 6. 推狀態 → done（僅全部通過時）

對通過的 task，呼叫 API 把 status 從 `in-review` 推進到 `done`：

```
curl -fsS -X PUT "http://$PIPER_HOST/api/epics/{epic}/story-map" ...
```

若 API 回 400（狀態機拒絕，例如 status 不是 in-review），停止並提示使用者手動檢查狀態。

### 7. 批次移動 brief（狀態推進成功後）

```
📋 驗收通過預覽

✅ 已通過：
  task-a1 — 登入 API
    Breakdown 完整性：✅ 做什麼的每條行為都有 L3 驗收覆蓋
    驗收條件：3/3 通過（含 C1 evidence + C2 抽驗 1 條綠燈）
    Story 交叉檢查：✅ 判定表已輸出，本 task 範圍覆蓋充分
  task-b1 — 密碼重設 API
    Breakdown 完整性：✅
    驗收條件：2/2 通過（含 C1 evidence + C2 抽驗 1 條綠燈）
    Story 交叉檢查：✅

確認移至 done/？
```

確認後：

```bash
mkdir -p .plan/briefs/done
mv .plan/briefs/task-a1.md .plan/briefs/done/
mv .plan/briefs/task-b1.md .plan/briefs/done/
```

### 7.5. 記錄指標

驗收通過並移動到 done/ 後，將指標追加到 `.plan/metrics.jsonl`（一行一筆 JSON）：

```json
{"task":"task-a1","timestamp":"2026-04-22T10:00:00Z","passed_first_try":true,"gap_count":0,"gap_types":[],"gap_details":[],"rework_count":0,"done_sampling_failed":false}
```

欄位說明：
- `task`: task ID
- `timestamp`: 完成時間（ISO 8601）
- `passed_first_try`: 本次 `/verify` 是否第一次就全過（沒有經歷缺口修復循環）
- `gap_count`: 驗收過程中發現的缺口總數（含修復循環中新發現的）
- `gap_types`: 缺口分類陣列（使用上方定義的標記，去掉方括號）
- `gap_details`: 未分類缺口（`uncategorized`）的自由文字描述
- `rework_count`: `/verify` 修復循環次數（0 = 一次通過）
- `done_sampling_failed`: C2 抽驗過程中是否曾經失敗（本次 `/verify` 整個 session 內只要有一次抽驗 fail 即 true）

如果 `.plan/metrics.jsonl` 不存在，建立它。每個 task 一行，追加到檔案末尾。

### 8. 產出摘要

```
✅ 已完成 2 個 tasks，移至 .plan/briefs/done/
   - task-a1 — 登入 API
   - task-b1 — 密碼重設 API
```

## 缺口分類全表

| 標記 | 意義 | 退回狀態 |
|------|------|----------|
| `[missing_test]` | 缺少測試 | `in-progress` |
| `[missing_evidence]` | 驗收勾了但沒附 evidence（C1 缺口） | `in-review` |
| `[missing_verify_cmd]` | 驗收條件下沒寫 verify 指令（C2 缺口） | `in-review` |
| `[sampling_failed]` | C2 抽驗重跑失敗 | `in-progress` |
| `[boundary_case]` | 邊界情況未處理 | `in-progress` |
| `[integration]` | 跨模組整合問題 | `in-progress` |
| `[acceptance_uncovered]` | 驗收條件未覆蓋 | `in-review` |
| `[scope_violation]` | 超出範圍修改 | `in-progress` |
| `[naming_mismatch]` | 命名不一致 | `in-progress` |
| `[uncategorized]` | 無法歸類（必須在描述中補充說明） | `in-progress`（保守）|

## 注意事項

- 不帶參數時自動掃描所有 status=`in-review` 的 brief，方便一次處理多個
- 缺口必須修復，不提供「仍要標記完成」的選項 — 使用者若要強制完成，需明確說「跳過驗收」
- 修復後重跑 `/verify`（若退到 in-progress，先跑 `/submit`）形成**檢查 → 修復 → 再檢查**循環，直到全部通過
- 移動後不要刪除 brief，`done/` 目錄的檔案是上游 retro 的信號
- **C1 + C2 + 獨立 session 是防自做自驗的核心**：`/submit` 寫的 evidence 在 `/verify` 被獨立重跑抽驗，作弊會在 C2 被抓
