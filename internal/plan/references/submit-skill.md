---
description: "Task 自測提交 — 在 worktree 中跑契約測試 + fix loop，測試綠燈後狀態推進到 in-review 等 /verify 收尾"
argument-hint: "[task-id]"
---

# Task 自測提交（Submit）

實作者角色的最後一哩。在受控的 worktree 中跑 Task 的契約測試，紅燈時啟動受限的 fix subagent 修改產品碼（禁止改測試 / 契約），直到綠燈或達到終止條件。**這個流程故意設計為作弊困難**：測試檔與契約檔在 fix 階段被 diff-gate 鎖定；越線即判失敗。

> **命名邏輯**：`/submit` 做「實作者自測 + 交付」— 狀態推到 `in-review`。最終 `done` 由另一個獨立 session 的 `/verify` skill 把關（C1 evidence + C2 抽驗 + Story 交叉），跟本 skill 分離以避免自做自驗。

## 參數

- `$ARGUMENTS`：單一 task ID（如 `task-a1`）。**一次只處理一個 task**。

## 前置條件

- 在 DevContainer 內執行，`PIPER_HOST` 環境變數存在（Piper 有注入）
- Task 的 brief 已由 `/breakdown` 補完 L3 驗收條件
- Epic 已存在至少一份覆蓋此 task 路徑的契約 YAML（`POST /api/epics/{epic}/contracts/{id}/generate` 能產出測試檔）
- Task 目前狀態必須是 `in-progress`（狀態機 guard）

## 流程

### 0. 入口 Guard：確認 task status == in-progress

讀取當前 story-map 確認 task status。

- `todo` → 停止，告知使用者「請先把 task 轉到 `in-progress` 再跑 `/submit`」
- `in-review` → 停止，告知使用者「task 已在 in-review，請跑 `/verify {task-id}` 做最終驗收；若要重跑 fix loop，請先把狀態退回 `in-progress`」
- `done` → 停止，告知使用者「task 已完成」
- `in-progress` → 繼續

### 1. 讀 Brief，取得元資料

讀取 `.plan/briefs/task-{id}.md`（若不存在，改讀 `.plan/briefs/done/task-{id}.md` 警告使用者「task 已完成」並退出）。從 brief 取得：

- `task-id`、`story-id`、`epic` 名稱
- `targetProject`（要在哪個專案跑測試）
- 該 task 應用到的**契約 ID 清單**，依序嘗試：
  1. 讀 brief 的「契約」章節（由 `/breakdown` 寫入的主路徑）
  2. 若章節缺失 → 呼叫 `GET /api/epics/{epic}/contracts?owner={task-id}` 反查（補丁，適用於 breakdown 尚未更新的舊 brief）
  3. 若反查仍為空 → 呼叫 `GET /api/epics/{epic}/contracts` 全量掃描，交給使用者選擇對應契約（最後一層 fallback；通常代表 owners 寫錯層級或 `/design` 根本沒跑）

### 1.5. Message 契約 Binding Pre-check

對步驟 1 取到的每個契約 ID，先查 type：

```bash
CONTRACT_TYPE=$(curl -sfL "http://$PIPER_HOST/api/epics/{epic}/contracts/{id}" | grep '^type:' | awk '{print $2}')
```

若 `type: message` → 必須確認本專案已補 binding：

```bash
PROJECT=$(basename $PWD)
curl -sfL -o /dev/null -w '%{http_code}' "http://$PIPER_HOST/api/bindings/$PROJECT/{contract-id}"
```

- `200` → OK，繼續
- `404` → **停止**，告知使用者：
  > message 契約 `{contract-id}` 尚未補 binding。/submit 無法產出可編譯的測試骨架。
  > 請先跑 `/bind {contract-id}`，完成後重跑 `/submit {task-id}`。

**不進入 worktree 建立與狀態機轉**——在這階段停止成本最低。狀態機轉後才發現缺 binding 會讓 task 卡在 in-review。

### 2. 狀態機轉 in-review

呼叫 Piper API 把該 task 的 status 推進到 `in-review`（一次到位，不再經過中間態）：

```
curl -fsS -X PUT "http://$PIPER_HOST/api/epics/{epic}/story-map" \
  -H 'Content-Type: application/yaml' \
  --data-binary @<(<修改後的 story-map.yaml>)
```

若 API 回 400（狀態機不允許），停止並提示使用者狀態目前是什麼，以及要先轉成 `in-progress` 才能跑 `/submit`。

### 3. 建立隔離 worktree

取得 targetProject 絕對路徑，執行：

```
bash .claude/scripts/verify-setup-worktree.sh {task-id} {target-project-path}
```

解析輸出的 JSON，記下 `worktree` 路徑、`commitHash`（pinned hash，diff-gate 用）、`baseBranch`。

> worktree 腳本名稱仍是 `verify-*.sh`：它們是「用 worktree 執行驗證測試」的通用工具，不隨 skill 角色改名。

### 4. 生成契約測試

對每份相關契約呼叫：

```
curl -fsS -X POST "http://$PIPER_HOST/api/epics/{epic}/contracts/{contract-id}/generate" \
  -H 'Content-Type: application/json' \
  -d '{"overwrite":false}'
```

回應的 `artifacts[]` 告知每個檔案是否被 write。若某個檔案已存在且 `written:false`，**先讀取現有檔案**判斷是否仍覆蓋本契約；若否，要求使用者確認 `overwrite:true` 重跑。

> 注意：生成器目前寫到主 repo 的 path。Phase 1 在 worktree 中再 copy 一份：
>
> ```
> cp {target-project-path}/{generated-file} {worktree}/{generated-file}
> ```

### 5. Fix Loop（上限 N=10）

維護狀態：
- `loopCount`（初始 0）
- `fingerprintHistory`（空陣列）
- `lastPassedCount`（初始 -1）

每次 iteration：

1. **在 worktree 跑測試**
   - Go：`cd {worktree} && go test ./...`
   - Node：`cd {worktree} && npm test --silent`
   - 測試指令以 targetProject 的 CLAUDE.md 或 Makefile 為準
2. **全綠 → 跳到步驟 6**（成功）
3. **紅燈**：
   - 擷取失敗清單：測試名稱 + 失敗訊息（取每個 fail 測試的前 20 行）
   - 計算 **fingerprint**：`sha1(sorted(失敗測試名稱) + 主要 error lines)`
   - 追加到 `fingerprintHistory`
   - **終止條件**（命中任一即停止）：
     - `loopCount >= 10`
     - 最近 3 次 fingerprint 相同（修復無進展）
     - 本次通過數 < `lastPassedCount`（回歸，fix subagent 弄壞了）
   - 若命中終止條件 → 跳到步驟 6（失敗分支）
   - 否則：更新 `lastPassedCount` = 本次通過數
4. **啟動 Fix Subagent**
   - 使用 Agent tool 啟動 `general-purpose` subagent，**不使用 isolation: worktree**（外層已在 worktree 中）
   - prompt 模板：

```
你是 Fix Subagent，在 worktree {worktree} 中工作。

## 目標
讓 {worktree} 中的測試全部通過。

## 絕對禁止
- 修改任何以 `_test.go` / `.test.ts` / `.test.tsx` / `.spec.ts` / `.spec.tsx` 結尾的檔案
- 修改 `contracts/` 目錄下的任何 `*.yaml` 檔案
- 刪除上述受保護檔案
違反 → 整個 submit 判失敗，你的工作被丟棄。

## 允許
- 修改生產 / 業務邏輯檔案（handlers、services、types、components 等）
- 新增檔案（除受保護檔案以外）

## 最近一次失敗摘要
{貼上失敗測試名稱 + error tail}

## 契約全文（唯讀參考）
{貼上契約 YAML 原文}

## 指示
1. 讀錯誤，定位產品碼缺陷
2. 只修產品碼讓測試變綠
3. 修完不需要跑測試（外層會跑）
4. 回報你動了哪些檔案（相對 worktree 的路徑）
```
   - 等 subagent 完成，`loopCount++`，回到步驟 1

### 6. Diff Gate + 收尾

**成功分支**：
1. 跑 `bash .claude/scripts/verify-diff-gate.sh {worktree} {commitHash}`
   - exit 1 → diff-gate 命中，視為作弊，**改判失敗**
2. 若通過：
   - `cd {worktree} && git add -A && git commit -m "submit: task-{id} 通過自測"`
   - 回到主 repo，把 worktree 的 commit rebase/cherry-pick 回 baseBranch
     - 簡化做法：`git -C {target} cherry-pick $(git -C {worktree} rev-parse HEAD)`
   - **不改 status**（步驟 2 已經推到 `in-review`，最終到 `done` 由 `/verify` skill 負責）
   - 在 brief 中勾 L3 驗收條件，附上 evidence：
     - 每條驗收下一行：`**evidence:** submit run {timestamp}, {loopCount} iterations`
   - 清理：`bash .claude/scripts/verify-cleanup-worktree.sh {task-id} {target}`
   - 提示使用者：「請跑 `/verify {task-id}` 做最終驗收」

**失敗分支**（loop 耗盡 / fingerprint 停滯 / 回歸 / diff-gate 命中）：
1. 把 status 退回 `in-progress`（要人介入）
2. **保留 worktree 不清理**，讓使用者可以進去 debug
3. 在 brief 末尾追加失敗報告：

```markdown
## Submit 失敗報告 (YYYY-MM-DD HH:MM)

- worktree: /tmp/piper-verify-{task-id}
- loopCount: {N}
- 終止原因: {loop 耗盡 | fingerprint 停滯 | 回歸 | diff-gate 作弊}
- 最後一次失敗測試:
  - {test name}: {brief error line}
{若 diff-gate 命中，列出違規檔案}
```

### 7. 寫入指標

不論成敗，在 targetProject 的 `.plan/metrics.jsonl` 追加一行：

```json
{"task":"task-{id}","timestamp":"ISO","verify_loop_count":N,"verify_passed_first_try":bool,"verify_exhausted":bool,"diff_gate_blocked":bool}
```

欄位（欄位名為相容 retro 報告 schema 保持 `verify_*` 前綴）：
- `verify_passed_first_try`: `loopCount==0 && 成功`
- `verify_exhausted`: 因 loop 上限 / fingerprint / 回歸失敗
- `diff_gate_blocked`: fix 階段動了受保護檔案

## 注意事項

- **一次只處理一個 task**。跨 task 的整合驗證由人類或 retro 判斷。
- PIPER_HOST 不可達 → 立即停止並要求使用者檢查 DevContainer 環境，不進入 loop。
- worktree 路徑固定 `/tmp/piper-verify-{task-id}`，若前次殘留 setup 腳本會自動清除。
- 外層 Claude 不直接寫生產碼。只有 fix subagent 寫，這是保護使用者「外層的思考脈絡不污染 fix 決策」。
- 本 skill **只推進到 `in-review`**，不會自行推到 `done`。最終驗收由獨立 session 的 `/verify` skill 把關，避免自做自驗。
- 使用繁體中文回報狀態與失敗報告。
