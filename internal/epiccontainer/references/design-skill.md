---
description: "設計契約 — 根據 story/task 上下文產出 http-api YAML 契約，dry-run 預覽後經人確認才寫入 vault"
argument-hint: "{epic} {story-id|task-id}（一次一個）"
---

# 契約設計

契約是**跨專案邏輯共識**：定義「應該長什麼樣」，不是反映既有 code 長什麼樣。根據 Story Map 中的 story / task 上下文，產出 **http-api 契約 YAML**。先存在才能讓前後端並行開發、讓 `/breakdown` 有 L3 指令可寫、讓 `/submit` 有斷言可跑。

> **名詞**：契約（contract）= 機器可驗證的 API 規格，前後端、task 之間的單一真實來源；`/submit` 之後會直接拿來生成測試。

## 時機

**plan 完成後 → dispatch 之前**在 host side 跑（也就是執行 plan 的同一個環境）。跑完 `/design` 再 `/dispatch`，`/breakdown` 才能拿契約 ID 當 L3 verify 指令。

刻意**不**探目標專案的既有 codebase：契約是邏輯層對齊，不是現有程式碼的鏡子。命名/欄位慣例的對齊錨點取自「同一 epic 內既有契約」，而不是 target project 的 routes。

## 參數

- `$ARGUMENTS`：epic 名稱 + 單一 `story-id` 或 `task-id`（如 `epic-a001 task-a1-be`）。**一次只處理一個**。

## 前置條件

- Piper 已啟動且本機 `http://127.0.0.1:8622` 可達
- 目標 epic 已存在 story map

> 預設情境為 host side（以下指令以 `127.0.0.1:8622` 為例）；若在 DevContainer 內執行，改用 `$PIPER_HOST:8622`。

## 流程

### 1. 讀 Story Map

```bash
curl -sfL "http://127.0.0.1:8622/api/epics/{epic}/story-map"
```

依 `$ARGUMENTS`：
- `task-*` → 定位單一 task 及其 parent story
- `story-*` → 列出該 story 下**所有** tasks（可能跨多 targetProject），讓使用者選擇一個 task 做契約（避免一次 design 多契約炸 context）

從 story 取出**驗收條件、Always / Ask / Never、tags**；從 task 取出 `targetProject`、`description`、`acceptance`。

### 2. 列出既有契約作為風格錨點

```bash
curl -sfL "http://127.0.0.1:8622/api/epics/{epic}/contracts"
```

用 `GET /api/epics/{epic}/contracts/{id}` 讀 1–2 個**同一 epic 內**最相近的契約，當 ID 命名、欄位型別、錯誤碼慣例的對齊基準。**不讀 target project 的既有 routes** — 那是實作細節，契約定義應該先於它。

若發現目標 task 已有明顯對應的契約（ID 或 path 高度相似）→ 暫停，詢問使用者：
- 改這份契約（改 version、走 changelog） → 進入「契約修訂」分支
- 另開新契約（ID 區分） → 進入「新契約」分支

### 3. 起草契約 YAML

依 story 驗收條件**反推**契約需要回什麼（足以讓呼叫端完成下一步），而不是翻譯 target project 現有 endpoint。Phase 1 僅支援 `type: http-api`。

```yaml
id: {epic-slug}-{resource}-{action}           # 如 dungeon-session-list、dungeon-item-get
version: 1.0
status: draft
owners:
  - {task-id}                                 # 對應本次 design 的 task；/breakdown、/submit 以此反查契約
type: http-api

http:
  method: GET
  path: /api/{resource}
  request:
    query:
      {param}: string
  response:
    success:
      status: 200
      body:
        status: string                        # "ok" / "error" / ...
        items: array
    errors:
      - status: 400
        when: "缺少必要欄位"
      - status: 500
        when: "內部錯誤"

generates:
  - project: {target-backend-project}
    format: go-httptest
    path: {path}/routes_{resource}_contract_test.go
  - project: {target-frontend-project}
    format: ts-types
    path: ui/src/types/contracts/{contract-id}.ts

changelog:
  - version: 1.0
    date: {today-YYYY-MM-DD}
    type: initial
    summary: 初始契約（由 /design 產出）
```

規則：
- **ID 格式**：`{epic-slug}-{resource}-{action}`，全小寫 hyphen-case；若 epic 已有契約，沿用其風格
- **owners**：填本次 design 的 `task-id`。下游 `/breakdown` 會用 `GET /api/epics/{epic}/contracts?owner={task-id}` 反查契約 ID 當 L3 verify 指令 — owners 填錯層級（如填 story-id）反查就會失敗，卡住 breakdown
- **method / path**：依 REST 慣例命名（複數資源、動詞後綴等），不照抄 target project
- **body 欄位型別**：用 `string`/`number`/`boolean`/`array`/`object`，或自訂 TS 型別名
- **errors**：至少一條 4xx（輸入驗證）+ 一條 5xx（內部錯誤）；有權限/找不到場景須各補一條
- **generates**：後端 `go-httptest` 必填；前端若有 TS client 再加 `ts-types`；`project` 對齊 story map `targetProject` 值；`path` 走 target project 慣例目錄（Go 測試放專案根或同 package 目錄、TS types 放 `ui/src/types/contracts/`）

### 4. 本地 A-gate（寫入前確認）

把起草的 YAML 存到暫存檔 `/tmp/piper-design-{id}.yaml` 並**完整貼到對話中**，向使用者說明：
- 契約 ID、type、method/path、主要回傳欄位、錯誤碼
- `generates[]` 會在哪個 project 產生哪些檔

詢問：**「確認寫入 vault 嗎？（yes / 修 X / 撤回）」**

- 回「yes」→ 進步驟 5
- 回「修 X」→ 修正 YAML 後重貼，重複本步驟
- 回「撤回」/ 任何拒絕 → 停止，不碰 vault

### 5. 寫入 vault + dry-run 預覽

1. **寫入**：

   ```bash
   curl -sfL -X PUT "http://127.0.0.1:8622/api/epics/{epic}/contracts/{id}" \
     -H 'Content-Type: text/yaml' --data-binary @/tmp/piper-design-{id}.yaml
   ```

   - 400 → 把錯誤訊息貼出來（通常是必填欄位漏、type/http 不符），回到步驟 3 修正
   - 2xx → 契約已落地 `.vault/epics/{epic}/contracts/{id}.yaml`

2. **預覽將來 `/submit` 會生成什麼**（不實際落地）：

   ```bash
   curl -sfL -X POST "http://127.0.0.1:8622/api/epics/{epic}/contracts/{id}/generate" \
     -H 'Content-Type: application/json' \
     -d '{"dryRun":true}'
   ```

   回應的 `artifacts[]` 內每筆：
   - `project` / `format` / `path` — 會在哪裡產出
   - `content` — 產出內容（可擷取前 30 行讓使用者確認）

3. **後悔路徑**：若使用者看到 dry-run 內容後想撤：

   ```bash
   curl -sfL -X DELETE "http://127.0.0.1:8622/api/epics/{epic}/contracts/{id}"
   ```

   回到步驟 3 重設計。

### 6. 結束

輸出契約落地位置：`.vault/epics/{epic}/contracts/{id}.yaml`；建議下一步：
- 同一 task 還有其他 story / API → 重跑 `/design` 補
- 全部契約定義完 → `/dispatch` 派發；target project 內 `/breakdown` 會引用契約 ID 當 L3 verify
- 實作完成 → `/submit {task-id}` 拿契約生成測試並跑 fix loop；綠燈後再跑 `/verify` 最終驗收

## 「契約修訂」分支（步驟 2 已存在對應契約）

1. `GET /api/epics/{epic}/contracts/{id}` 讀舊版 YAML
2. 在舊版上做必要修改，**version 規則**：
   - 新增 optional field / endpoint / error case → minor bump（1.0 → 1.1）
   - 刪欄位 / 改型別 / 改 path / method → major bump（1.x → 2.0），`changelog.reason` 必須說明遷移路徑
   - **owners 更新**：若本次 design 的 task-id 不在既有 owners 中，append 進去（不要覆蓋既有 owners — 多個 task 共用一份契約時需要累積，否則原先的 task `/breakdown` 反查會斷）
3. **Changelog 必填新條目**：
   ```yaml
   changelog:
     - version: 1.1
       date: {today}
       type: additive   # 或 breaking
       summary: {一句話摘要}
       reason: {觸發原因，例：verify-retro-feedback / plan-new-scope / breaking migration：...}
       retro_item: {若來自 retro，貼原始觀察}
   ```
4. 其餘步驟同「新契約」流程（本地 A-gate → PUT → dry-run 預覽）

> 伺服器端 `ClassifyChange` 會自動判定 breaking/additive，但目前不強制要求 migration 欄位 — 人 review 時確認 breaking 變更的影響有寫清楚。

## 注意事項

- **一次一個 story/task**。多 task 的契約各自 `/design` 跑一次。
- **不探 target project codebase**。契約是邏輯共識，不是現有 code 的鏡子。
- **不自動跑 generate**：dry-run 只預覽；真正落地測試檔由 `/submit` 負責。
- **寫 vault 前必須 A-gate**。即使伺服器端能驗過，仍由人做最後一眼 review。
- 使用繁體中文回報。
